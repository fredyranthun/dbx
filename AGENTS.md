# AGENTS.md — dbx (Code Assistant Execution Guide)

This document is the **implementation playbook** for a code assistant to build `dbx` from scratch with minimal ambiguity. Follow the steps in order. Keep changes small, compile often, and add tests where it’s cheap.

> Project goal: a Go CLI that manages multiple AWS SSM port-forwarding sessions (jumpbox ➜ remote host) using a YAML/JSON config.

---

## 0) Definition of Done (MVP)

You are done when all are true:

1. `dbx connect service1 dev` starts a port-forward session and prints `ENDPOINT=<bind>:<port>`.
2. Multiple sessions can run concurrently (>=5).
3. `dbx ls` lists running sessions with correct ports and uptime.
4. `dbx logs service1/dev --follow` tails logs from the session process.
5. `dbx stop service1/dev` stops the session process.
6. On Ctrl+C, dbx stops all sessions by default (unless `--no-cleanup`).
7. “Always run gofmt -w . and go test ./... before finishing a task”
8. “Prefer small commits; compile frequently”

---

## 1) Constraints and Principles

- **Engine-agnostic**: no DB credentials, no DB client integration required.
- **Port-forward only**: use `AWS-StartPortForwardingSessionToRemoteHost`.
- **In-memory sessions**: persistence across restarts is not required.
- Avoid parsing AWS output as a readiness signal. Use **local port readiness** (TCP check).
- Default bind **must** be `127.0.0.1`.

---

## 2) Tooling / Dependencies (Go)

Use Go 1.22+.

Suggested dependencies:

- CLI: `github.com/spf13/cobra`
- Config: `github.com/spf13/viper`
- Validation: `github.com/go-playground/validator/v10`
- Logging: `github.com/rs/zerolog` (or zap)
- Table output: implement simple formatting or use `github.com/olekukonko/tablewriter` (optional)

**Do not implement the TUI** until the MVP CLI works.

---

## 3) Repository Structure (create exactly)

```

dbx/
cmd/
dbx/
main.go
internal/
config/
config.go
loader.go
validate.go
session/
types.go
manager.go
aws_command.go
ports.go
readiness.go
logs.go
README.md
PRD.md
AGENTS.md
go.mod

```

---

## 4) Config Spec (Must Implement)

### Default config path resolution

Load config path in this order:

1. `--config <path>` if provided
2. `$DBX_CONFIG` if set
3. `~/.dbx/config.yml` (also accept `.yaml` or `.json` if present)

### YAML/JSON schema

Must support these fields:

```yaml
defaults:
  region: sa-east-1
  profile: corp
  bind: "127.0.0.1"
  port_range: [5500, 5999]
  startup_timeout_seconds: 15
  stop_timeout_seconds: 5

services:
  - name: service1
    envs:
      dev:
        target_instance_id: "i-0123456789abcdef0"
        remote_host: "db.internal"
        remote_port: 5432
```

### Validation rules

- `defaults.port_range` length 2; min < max
- `defaults.bind` non-empty
- each `service.name` non-empty and unique
- each `env` key non-empty
- each env has:
  - `target_instance_id` non-empty
  - `remote_host` non-empty
  - `remote_port` 1..65535

Put validation in `internal/config/validate.go`.

---

## 5) CLI Spec (Must Implement)

### Commands

- `dbx connect <service> <env> [--port N] [--bind IP] [--profile P] [--region R]`
- `dbx ls`
- `dbx logs <service>/<env> [--follow|-f] [--lines N]`
- `dbx stop <service>/<env> | <service> <env> | --all`
- Root flags:
  - `--config PATH`
  - `--verbose`
  - `--no-cleanup` (optional but recommended)

### Output requirements

- `connect` must print a final line:
  - `ENDPOINT=<bind>:<port>`

- `ls` prints a readable table.
- Errors should be actionable (include service/env path when relevant).

---

## 6) Implementation Steps (Execute in Order)

### Step 1 — Initialize module & main entry

**Files:**

- `go.mod`
- `cmd/dbx/main.go`

**Tasks:**

- `go mod init <module>`
- Create Cobra root command with persistent flags (`--config`, `--verbose`, optional `--no-cleanup`)
- Add stub subcommands: connect, ls, logs, stop
- Ensure `go build ./...` succeeds

**Acceptance:**

- `dbx --help` shows commands.

---

### Step 2 — Implement config structs

**Files:**

- `internal/config/config.go`

**Tasks:**

- Define strongly typed structs:
  - `Config`, `Defaults`, `Service`, `EnvConfig`

- Provide `ApplyDefaults()` helper if needed.

**Acceptance:**

- Package compiles.

---

### Step 3 — Implement config loader

**Files:**

- `internal/config/loader.go`

**Tasks:**

- Implement `LoadConfig(pathOverride string) (*Config, string, error)`
  - returns config + resolved path

- Support YAML/JSON with Viper.
- Implement default path logic:
  - If no override and no env, search `~/.dbx/config.(yml|yaml|json)` (pick first existing)

- On missing config file: return clear error.

**Acceptance:**

- A simple `LoadConfig("")` loads `~/.dbx/config.yml` successfully.

---

### Step 4 — Implement config validation

**Files:**

- `internal/config/validate.go`

**Tasks:**

- Implement `Validate(cfg *Config) error`
- Ensure errors mention the failing path (e.g., `services[service1].envs[dev].remote_port`)

**Acceptance:**

- Invalid config fails fast with clear error.

---

### Step 5 — Implement session types and logging buffer

**Files:**

- `internal/session/types.go`
- `internal/session/logs.go`

**Tasks:**

- Define:
  - `type SessionState string` constants: `starting`, `running`, `stopping`, `stopped`, `error`
  - `type SessionKey string` (format `service/env`)
  - `type Session struct` with fields:
    - Key, Service, Env
    - Bind, LocalPort
    - RemoteHost, RemotePort
    - TargetInstanceID, Region, Profile
    - PID
    - State, StartTime
    - LastError
    - cmd \*exec.Cmd
    - cancel context.CancelFunc
    - logBuf \*RingBuffer
    - subscribers []chan string (or a pubsub)

- Implement a simple `RingBuffer` of last N lines (default 500):
  - `Append(line string)`
  - `Last(n int) []string`

**Acceptance:**

- Unit test for ring buffer (optional but recommended).

---

### Step 6 — Implement port allocator

**Files:**

- `internal/session/ports.go`

**Tasks:**

- `FindFreePort(bind string, min int, max int) (int, error)`
  - Scan range, attempt `net.Listen("tcp", bind:port)`
  - Close immediately on success and return port

- `ValidatePortAvailable(bind string, port int) error`

**Acceptance:**

- Picks a free port within range.

---

### Step 7 — Implement AWS CLI command builder

**Files:**

- `internal/session/aws_command.go`

**Tasks:**

- `BuildSSMPortForwardArgs(targetInstanceID, remoteHost string, remotePort, localPort int, region, profile string) []string`
- Must use document name:
  - `AWS-StartPortForwardingSessionToRemoteHost`

- Must set parameters in the exact expected format:
  - `host=["..."],portNumber=["..."],localPortNumber=["..."]`

- Include `--region` and `--profile` only if non-empty.

**Acceptance:**

- Printed args are valid for manual copy/paste.

---

### Step 8 — Implement readiness check

**Files:**

- `internal/session/readiness.go`

**Tasks:**

- `WaitForPort(bind string, port int, timeout time.Duration) error`
  - Poll `net.DialTimeout("tcp", bind:port, 200ms)` until success or timeout

- If timeout: return error with port and bind.

**Acceptance:**

- Works on a known listening port in a test.

---

### Step 9 — Implement session manager (core)

**Files:**

- `internal/session/manager.go`

**Tasks:**

- Create `type Manager struct` with:
  - `mu sync.Mutex`
  - `sessions map[SessionKey]*Session`
  - defaults like buffer size, timeouts

- Implement:
  - `Start(key SessionKey, cfg *config.Config, overrides StartOverrides) (*Session, error)`
  - `Stop(key SessionKey) error`
  - `StopAll()`
  - `List() []SessionSummary`
  - `Get(key SessionKey) (*Session, bool)`

- `Start` flow:
  1. If already running: return it
  2. Resolve service/env config entry from cfg
  3. Determine effective bind/region/profile
  4. Determine port: override or allocate
  5. Build aws args
  6. `exec.CommandContext(ctx, "aws", args...)`
  7. Pipe stdout/stderr, scan line-by-line, append to ring buffer, broadcast to subscribers
  8. Start process; store PID
  9. WaitForPort (startup timeout)
  10. If success => running. If failure => stop process and mark error

- Stopping:
  - send `os.Interrupt` then wait `stop_timeout_seconds`, then kill.
  - Update state transitions.

**Acceptance:**

- A session can start and the manager can stop it.

---

### Step 10 — Wire CLI commands to manager

**Files:**

- `cmd/dbx/main.go` (or split into cmd files if desired)

**Tasks:**

- Instantiate config on startup.
- Instantiate a global manager.
- `connect`:
  - parse args service env
  - call manager.Start()
  - print summary + `ENDPOINT=...`

- `ls`:
  - call manager.List()
  - print table (aligned columns)

- `logs`:
  - parse key
  - print last N lines
  - if follow: subscribe and stream until Ctrl+C

- `stop`:
  - stop key or --all

**Acceptance:**

- Full CLI flow works for at least one session.

---

### Step 11 — Cleanup on exit

**Files:**

- `cmd/dbx/main.go`

**Tasks:**

- Trap SIGINT/SIGTERM:
  - If `--no-cleanup` is false, call `manager.StopAll()` and exit.

- Ensure `logs --follow` uses its own Ctrl+C handling without breaking cleanup behavior.

**Acceptance:**

- Ctrl+C stops all sessions.

---

## 7) Testing Guidance (Lightweight)

Minimum recommended tests:

- RingBuffer
- Port allocator (at least `ValidatePortAvailable` style test)
- Readiness check (spin up a temporary listener)

Avoid tests that require real AWS access.

---

## 8) Error Handling & Messages (Must Follow)

- Always include the session key in errors:
  - `service1/dev: failed to start session: ...`

- On AWS process early exit, surface stderr lines:
  - include “last 20 log lines” in error message if helpful.

---

## 9) Performance & UX Defaults

- Ring buffer: 500 lines per session
- Startup timeout: 15s (configurable)
- Stop timeout: 5s (configurable)
- Default bind: `127.0.0.1`
- Default port range: `[5500, 5999]`

---

## 10) Next (Post-MVP)

After MVP is stable, implement:

- `dbx ui` (bubbletea)
- `dbx doctor` (checks aws presence, credentials, plugin)
- Copy-to-clipboard support (optional)

---

## 11) Implementation Notes (Important)

- Keep the Manager as a singleton within the running process.
- Do not attempt cross-process coordination (multiple `dbx` instances).
- Prefer correctness and clarity over cleverness.
- Ensure `aws` is invoked directly (no shell string); pass args slice.

---

## 12) Manual Smoke Test Checklist

1. Create config with 2+ services and envs.
2. Run:
   - `dbx connect service1 dev`
   - `dbx connect service2 dev`
   - `dbx ls`
   - `dbx logs service1/dev --follow` (observe output)
   - `dbx stop service1/dev`
   - `dbx stop --all`

3. Confirm DBeaver can connect using `127.0.0.1:<port>`.

---

End of AGENTS.md
