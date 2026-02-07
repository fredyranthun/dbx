# AGENTS.md — dbx (Code Assistant Execution Guide)

This document is the implementation playbook for a code assistant to build `dbx` from scratch with minimal ambiguity. Follow the steps in order. Keep changes small, compile often, and add tests where it’s cheap.

Project goal: a Go CLI that manages multiple AWS SSM port-forwarding sessions (jumpbox ➜ remote host) using a YAML/JSON config.

---

## Working Agreement (must follow)

- Always run `make check` before finishing a task.
- Prefer small commits and incremental changes; keep the repo building at each step.
- Do not invoke `aws` via a shell string. Always exec `aws` directly with an args slice.
- Do not implement the TUI until MVP CLI is working.

---

## 0) Definition of Done (MVP)

You are done when all are true:

1. `dbx connect service1 dev` starts a port-forward session and prints `ENDPOINT=<bind>:<port>`.
2. Multiple sessions run concurrently (>=5) without port collisions.
3. `dbx ls` lists running sessions with correct ports and uptime.
4. `dbx logs service1/dev --follow` tails logs from the session process.
5. `dbx stop service1/dev` stops the session process.
6. On Ctrl+C, dbx stops all sessions by default (unless `--no-cleanup`).

---

## 1) Constraints and Principles

- Engine-agnostic: no DB credentials, no DB client integration required.
- Port-forward only: use `AWS-StartPortForwardingSessionToRemoteHost`.
- In-memory sessions: persistence across restarts is not required.
- Avoid parsing AWS output as a readiness signal. Use local port readiness (TCP check).
- Default bind must be `127.0.0.1`.

---

## 2) Tooling / Dependencies (Go)

Use Go 1.22+.

Suggested dependencies:

- CLI: `github.com/spf13/cobra`
- Config: `github.com/spf13/viper`
- Validation: `github.com/go-playground/validator/v10`
- Logging: `github.com/rs/zerolog` (or zap)
- Table output: simple formatting or `github.com/olekukonko/tablewriter` (optional)

---

## 3) Repository Structure (create exactly)

```

.
├── cmd/
│ └── dbx/
│ └── main.go
├── internal/
│ ├── config/
│ │ ├── config.go
│ │ ├── loader.go
│ │ └── validate.go
│ └── session/
│ ├── aws_command.go
│ ├── logs.go
│ ├── manager.go
│ ├── ports.go
│ ├── readiness.go
│ └── types.go
├── .codex/
│ └── config.toml
├── AGENTS.md
├── Makefile
├── PRD.md
├── README.md
└── go.mod

```

Notes:

- `.codex/config.toml` must exist to provide Codex “golden path” commands.
- `go.mod` should be created in Step 1.

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

Root flags:

- `--config PATH`
- `--verbose`
- `--no-cleanup` (optional but recommended)

### Output requirements

- `connect` must print a final line: `ENDPOINT=<bind>:<port>`
- `ls` prints a readable table.
- Errors must include the session key and be actionable.

---

## 6) Implementation Steps (Execute in Order)

### Step 1 — Initialize module & main entry

Files:

- `go.mod`
- `cmd/dbx/main.go`

Tasks:

- `go mod init github.com/fredyranthun/db` (ALREADY DONE)
- Create Cobra root command with persistent flags (`--config`, `--verbose`, optional `--no-cleanup`)
- Add stub subcommands: connect, ls, logs, stop
- Ensure `make check` succeeds (add targets if missing)

Acceptance:

- `dbx --help` shows commands.
- `make check` passes.

---

### Step 2 — Implement config structs

Files:

- `internal/config/config.go`

Tasks:

- Define strongly typed structs: `Config`, `Defaults`, `Service`, `EnvConfig`
- Add helpers for defaults merging if needed.

Acceptance:

- Package compiles.

---

### Step 3 — Implement config loader

Files:

- `internal/config/loader.go`

Tasks:

- Implement `LoadConfig(pathOverride string) (*Config, string, error)`
- Support YAML/JSON with Viper
- Default path logic: search `~/.dbx/config.(yml|yaml|json)` (pick first existing)
- Missing config file should return a clear error.

Acceptance:

- `LoadConfig("")` loads `~/.dbx/config.yml` successfully (when present).

---

### Step 4 — Implement config validation

Files:

- `internal/config/validate.go`

Tasks:

- Implement `Validate(cfg *Config) error`
- Errors must mention failing path (e.g., `services[service1].envs[dev].remote_port`)

Acceptance:

- Invalid config fails fast with clear error.

---

### Step 5 — Implement session types and logging buffer

Files:

- `internal/session/types.go`
- `internal/session/logs.go`

Tasks:

- Define:
  - `SessionState`: `starting`, `running`, `stopping`, `stopped`, `error`
  - `SessionKey`: `service/env`
  - `Session` struct with:
    - Key, Service, Env
    - Bind, LocalPort
    - RemoteHost, RemotePort
    - TargetInstanceID, Region, Profile
    - PID
    - State, StartTime, LastError
    - cmd \*exec.Cmd, cancel context.CancelFunc
    - logBuf \*RingBuffer
    - subscribers / pubsub for follow logs

- Implement `RingBuffer` (default 500 lines):
  - `Append(line string)`
  - `Last(n int) []string`

Acceptance:

- Unit test for RingBuffer (optional but recommended).

---

### Step 6 — Implement port allocator

Files:

- `internal/session/ports.go`

Tasks:

- `FindFreePort(bind string, min int, max int) (int, error)` using `net.Listen`
- `ValidatePortAvailable(bind string, port int) error`

Acceptance:

- Picks a free port within range.

---

### Step 7 — Implement AWS CLI command builder

Files:

- `internal/session/aws_command.go`

Tasks:

- `BuildSSMPortForwardArgs(...) []string`
- Must use:
  - document: `AWS-StartPortForwardingSessionToRemoteHost`
  - parameters format:
    - `host=["..."],portNumber=["..."],localPortNumber=["..."]`

- Include `--region` and `--profile` only if non-empty.

Acceptance:

- Args are valid for manual copy/paste.

---

### Step 8 — Implement readiness check

Files:

- `internal/session/readiness.go`

Tasks:

- `WaitForPort(bind string, port int, timeout time.Duration) error`
- Poll `net.DialTimeout` until success or timeout.

Acceptance:

- Works in a test with a temp listener.

---

### Step 9 — Implement session manager (core)

Files:

- `internal/session/manager.go`

Tasks:

- `Manager` with mutex + sessions map
- Implement:
  - `Start(...)`
  - `Stop(key)`
  - `StopAll()`
  - `List() []SessionSummary`
  - `Get(key)`

- `Start` must:
  - allocate port
  - start `exec.CommandContext(ctx, "aws", args...)`
  - capture stdout/stderr to ring buffer + subscribers
  - readiness via `WaitForPort`

- `Stop` must:
  - send interrupt then kill after timeout
  - update state transitions

Acceptance:

- Start/stop works for one session.

---

### Step 10 — Wire CLI commands

Files:

- `cmd/dbx/main.go` (or split)

Tasks:

- `connect` prints summary + `ENDPOINT=...`
- `ls` prints table
- `logs` prints last N; follow streams until Ctrl+C
- `stop` stops one or all

Acceptance:

- Full CLI flow works.

---

### Step 11 — Cleanup on exit

Files:

- `cmd/dbx/main.go`

Tasks:

- Trap SIGINT/SIGTERM
- If `--no-cleanup` is false: `StopAll()` and exit
- Ensure `logs --follow` Ctrl+C doesn’t break global cleanup

Acceptance:

- Ctrl+C stops all sessions.

---

## 7) Testing Guidance (Lightweight)

Recommended tests:

- RingBuffer
- Port allocator
- Readiness check

Avoid tests requiring AWS access.

---

## 8) Error Handling

- Always include the session key in errors:
  - `service1/dev: failed to start session: ...`

- If aws exits early, include last ~20 log lines in error output.

---

## 9) TUI Implementation Steps (Post-MVP Only)

Only start these steps after Step 11 acceptance criteria pass.

### Step 12 — Add TUI command scaffold

Files:

- `cmd/dbx/main.go`
- `internal/ui/model.go`
- `internal/ui/view.go`

Tasks:

- Add `dbx ui` command.
- Initialize Bubble Tea program from command handler.
- Create placeholder model/view with startup help and quit handling (`q`, `ctrl+c`).

Acceptance:

- `dbx ui` launches and exits cleanly.
- `make check` passes.

---

### Step 13 — Build TUI state model from app/session manager

Files:

- `internal/ui/model.go`

Tasks:

- Define UI state:
  - configured targets (`service/env`)
  - running sessions snapshot
  - selected item index
  - focused pane (`targets`, `sessions`, `logs`)
  - last error/status line
- Add periodic refresh (`tea.Tick`) to pull `List()` snapshots from manager.

Acceptance:

- TUI reflects session start/stop changes without restart.

---

### Step 14 — Implement layout, keybindings, and actions

Files:

- `internal/ui/view.go`
- `internal/ui/model.go`

Tasks:

- Three-pane layout:
  - left: all configured `service/env`
  - middle: running sessions
  - right/bottom: logs for selected session
- Keybindings:
  - `j/k` or arrows: move selection
  - `tab`: cycle focus pane
  - `c`: connect selected target
  - `s`: stop selected running session
  - `S`: stop all sessions
  - `l`: toggle follow logs
  - `q` or `ctrl+c`: quit
- Show endpoint for running items (`bind:port`) and session state.

Acceptance:

- Connect/stop actions work from TUI and match CLI behavior.

---

### Step 15 — Wire logs stream integration

Files:

- `internal/ui/model.go`
- `internal/session/logs.go` (only if extra helpers are required)

Tasks:

- On selection/focus change, subscribe to selected session logs.
- Render buffered history first, then streamed lines when follow is enabled.
- Cleanly unsubscribe on session switch and on quit.

Acceptance:

- No goroutine/channel leaks when switching sessions repeatedly.
- Logs pane shows last lines and live updates.

---

### Step 16 — TUI robustness and tests

Files:

- `internal/ui/model_test.go`
- `internal/ui/view_test.go` (optional snapshot-style assertions)

Tasks:

- Add tests for:
  - key handling transitions
  - focus/selection behavior
  - connect/stop intent dispatch
  - follow toggle and subscription lifecycle
- Ensure graceful shutdown:
  - `dbx ui` quit triggers default cleanup behavior unless `--no-cleanup`

Acceptance:

- `make check` passes.
- TUI can be used for a full flow: connect, view logs, stop, quit.
