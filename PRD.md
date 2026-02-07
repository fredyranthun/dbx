# PRD — dbx (SSM Port-Forwarding Session Manager)

## 1) Summary

**dbx** is a configuration-oriented CLI (and optional TUI) that manages multiple **AWS SSM port-forwarding sessions** to remote database hosts via jumpbox EC2 instances. It runs entirely in one terminal, keeps multiple tunnels alive concurrently, shows status, streams logs, and stops sessions on demand. Sessions may terminate when dbx exits.

## 2) Goals

- Start/stop/manage multiple SSM port-forwarding sessions concurrently from a single terminal.
- Use a **YAML/JSON config** to define services and environments.
- Be **engine-agnostic**: dbx only provides `localhost:<port>` endpoints for tools like DBeaver.
- Provide:
  - List active sessions
  - Tail logs per session
  - Stop specific sessions or all
  - (Optional) Interactive TUI for managing sessions

## 3) Non-Goals

- DB authentication/credentials management.
- Creating or modifying AWS resources.
- Persisting sessions across tool exits (acceptable that sessions die when dbx exits).
- Handling interactive shell sessions (only port forwarding).

## 4) Users & Use Cases

- Developer working in WSL/Linux needing to connect to many services (dev/stg/prod).
- Wants to:
  - Quickly start a tunnel: `dbx connect service1 dev`
  - See what’s active: `dbx ls`
  - Tail logs: `dbx logs service1/dev -f`
  - Stop tunnels: `dbx stop service1/dev`, `dbx stop --all`
  - Manage everything from one terminal (TUI).

## 5) Requirements

### 5.1 Functional Requirements (MVP)

**FR1 — Load config**

- dbx loads YAML/JSON from (in priority order):
  1. `--config <path>`
  2. `$DBX_CONFIG`
  3. `~/.dbx/config.yml` (or `.yaml` / `.json`)

- Validate config schema; on error print actionable message.

**FR2 — Connect**

- `dbx connect <service> <env> [flags]`
- If session already running for `<service>/<env>`:
  - Print current local endpoint and exit 0.

- If not running:
  - Allocate local port (unless `--port` specified).
  - Start SSM port-forwarding session (AWS CLI process).
  - Wait until local port is listening (readiness).
  - Print:
    - `service/env`
    - `127.0.0.1:<localPort>` (or configured bind)
    - remote target `<host>:<port>`
    - session id (internal)

**FR3 — List**

- `dbx ls`
- Show active sessions with columns:
  - ID (service/env)
  - Local endpoint (bind:port)
  - Remote host:port
  - Status (starting/running/stopped/error)
  - Uptime
  - PID

**FR4 — Stop**

- `dbx stop <service>/<env>` OR `dbx stop <service> <env>`
- Graceful stop:
  - send interrupt/terminate to AWS CLI process
  - wait up to N seconds, then force kill

- `dbx stop --all`

**FR5 — Logs**

- `dbx logs <service>/<env> [--follow|-f] [--lines N]`
- Print last N lines from ring buffer; if `--follow` stream new lines until Ctrl+C.

**FR6 — Concurrency**

- Multiple sessions can be started and run concurrently.
- No local port collisions.

**FR7 — Exit behavior**

- On dbx exit, stop all managed sessions (default). Provide `--no-cleanup` to leave them running (optional).
  - Since user said “sessions die when tool exits is acceptable”, default cleanup is fine.

### 5.2 Functional Requirements (TUI — v1, optional after MVP)

**FR8 — TUI**

- `dbx ui` opens a terminal UI:
  - list all configs (service/env)
  - list running sessions
  - view logs for selected session
  - connect/stop via hotkeys

### 5.3 Non-Functional Requirements

**NFR1 — Cross-platform**

- Works on Linux and WSL. (macOS optional)
- Assumes `aws` CLI is available in PATH.

**NFR2 — Observability**

- dbx has its own logs with `--verbose` for debugging.
- Session logs are captured (stdout/stderr of aws process).

**NFR3 — Safety**

- Default bind is `127.0.0.1` (never expose tunnel by default).
- Do not print secrets.

**NFR4 — Performance**

- Support at least 20 simultaneous sessions (within AWS limits).

## 6) Configuration Specification

### 6.1 YAML schema (canonical)

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
        remote_host: "db.internal" # reachable from target instance
        remote_port: 5432
        local_port: 55432 # optional fixed local port for this env
      stg:
        target_instance_id: "i-0abcdef1234567890"
        remote_host: "db-stg.internal"
        remote_port: 5432
```

### 6.2 Notes

- `remote_host` is the host the jumpbox can reach (RDS endpoint, private DNS, IP).
- `target_instance_id` is the EC2 jumpbox instance ID.
- `local_port` is optional; when present, dbx tries to use that local port for the env.
- JSON config is allowed with same fields.

### 6.3 Overrides

Command flags override config defaults:

- `--profile`, `--region`, `--bind`, `--port`, `--timeout`

## 7) CLI Specification

### 7.1 Root

`dbx [--config PATH] [--verbose]`

### 7.2 Commands

- `dbx connect <service> <env> [--port N] [--bind IP] [--profile P] [--region R]`
- `dbx ls`
- `dbx stop <service>/<env> | <service> <env> | --all`
- `dbx logs <service>/<env> [--follow|-f] [--lines N]`
- (Optional) `dbx ui`
- (Optional) `dbx doctor` (check aws presence, credentials, plugin)

### 7.3 Output conventions

- Human-readable tables for `ls`.
- For `connect`, print endpoint in a machine-friendly final line:
  - `ENDPOINT=127.0.0.1:55321`

## 8) Technical Design

## 8.1 Stack

- Go 1.22+
- cobra (CLI)
- viper (config)
- go-playground/validator (schema)
- bubbletea (TUI, optional)
- zerolog or zap (tool logging)

## 8.2 Architecture Overview

- `ConfigLoader` loads + validates config.
- `SessionManager` manages lifecycle of sessions in-memory.
- Each session is an `exec.Cmd` running `aws ssm start-session ...`
- `LogPump` reads stdout/stderr and stores lines in a ring buffer.
- `PortAllocator` picks a free port from configured range.

## 8.3 Session model

Session key: `service/env` (unique)

Session state:

- `Starting`
- `Running`
- `Stopping`
- `Stopped`
- `Error`

Session fields:

- Key (service/env)
- LocalBind, LocalPort
- RemoteHost, RemotePort
- TargetInstanceId
- Region, Profile
- Cmd (process handle)
- StartTime
- LastError
- LogBuffer (ring)

## 8.4 Starting a session (algorithm)

1. Determine config entry for service/env.
2. Determine effective region/profile/bind.
3. Allocate local port:
   - if `--port` given use it (fail if in use)
   - else if env `local_port` given use it (fail if in use)
   - else scan `port_range` for first free port

4. Build args for AWS CLI:
   - `aws ssm start-session`
   - `--target <instance>`
   - `--document-name AWS-StartPortForwardingSessionToRemoteHost`
   - `--parameters host=["<remote_host>"],portNumber=["<remote_port>"],localPortNumber=["<local_port>"]`
   - `--region`, `--profile` if present

5. Start process with context.
6. Readiness: poll TCP connect to `<bind>:<localPort>` until success or timeout.
7. Mark running or error.

## 8.5 Stopping a session (algorithm)

1. If session exists and running/starting:
2. Send SIGINT (or os.Interrupt) to process
3. Wait up to `stop_timeout_seconds`
4. If still alive: kill
5. Mark stopped, release port

## 8.6 Logs

- Capture stdout and stderr of the aws process.
- Store last N lines (default 500) in memory ring buffer.
- `logs --follow` tails new lines via a channel subscription.

## 8.7 Thread safety

- SessionManager uses mutex for session map.
- Each session has internal channels for log lines, stop signals.

## 9) Repository Structure (initial)

```
dbx/
  go.mod
  cmd/
    dbx/
      main.go
  internal/
    app/
      app.go              # wires config + session manager + commands
    config/
      config.go           # structs, defaults
      loader.go           # viper load
      validate.go         # validator rules
    session/
      manager.go          # start/stop/list/get
      session.go          # session struct + state
      aws_command.go      # builds aws args
      ports.go            # port allocator
      logs.go             # ring buffer + subscriptions
      readiness.go        # wait for port open
    ui/                   # optional later
      model.go
      view.go
  README.md
  PRD.md
```

## 10) Acceptance Criteria (MVP)

- AC1: With a valid config, `dbx connect service1 dev` starts a tunnel and prints `ENDPOINT=127.0.0.1:<port>`.
- AC2: Starting 5 sessions concurrently works without port collisions.
- AC3: `dbx ls` lists all running sessions with correct ports and uptime.
- AC4: `dbx logs service1/dev -f` streams logs from the corresponding aws process.
- AC5: `dbx stop service1/dev` stops the process and removes it from `ls`.
- AC6: On Ctrl+C of dbx, all sessions are stopped (unless `--no-cleanup` is enabled).

## 11) Edge Cases

- AWS credentials expired → session fails; show aws stderr in `connect` error output and `logs`.
- Port range exhausted → friendly error.
- `remote_host` not reachable from jumpbox → SSM may still start but tunnel unusable; dbx only checks local port listening (document limitation).
- Multiple dbx instances running → may cause port conflicts; MVP does not coordinate across processes.

## 12) Risks & Mitigations

- SSM output formats vary → avoid parsing; rely on local port readiness + process liveness.
- Windows signals in WSL → use `exec.CommandContext` and ensure kill fallback.

## 13) TUI v1 Implementation Plan (Post-MVP)

Run this plan only after MVP acceptance criteria (AC1..AC6) are complete.

### Step T1 — Command and program bootstrap

- Add `dbx ui` command in Cobra.
- Start Bubble Tea program and provide a minimal model/view.
- Support quit keys: `q`, `ctrl+c`.

Acceptance:

- `dbx ui` opens and exits cleanly.

### Step T2 — State model and refresh loop

- Model tracks:
  - all configured targets (`service/env`)
  - running sessions snapshot
  - selected row and focused pane
  - follow-logs mode
  - status/error line
- Add periodic refresh tick to sync with SessionManager.

Acceptance:

- Session list reflects runtime changes without restarting UI.

### Step T3 — Interactive controls and actions

- Layout:
  - targets pane
  - running sessions pane
  - logs pane
- Keymap:
  - navigation: arrows or `j/k`
  - pane switch: `tab`
  - connect: `c`
  - stop selected: `s`
  - stop all: `S`
  - follow toggle: `l`
  - quit: `q`, `ctrl+c`

Acceptance:

- Connect/stop from TUI delegates to same manager logic used by CLI.

### Step T4 — Logs integration

- Show recent buffered logs on selection.
- Support live follow stream via session log subscriptions.
- Unsubscribe on selection change and quit.

Acceptance:

- Logs update live when follow is enabled.
- Repeated selection switches do not leak subscriptions.

### Step T5 — Hardening and tests

- Add focused model tests for key handling, focus changes, and action dispatch.
- Verify cleanup behavior on quit matches CLI (`--no-cleanup` respected).

Acceptance:

- `make check` passes with TUI code enabled.
- End-to-end flow works: connect, observe logs, stop, quit.

---

# Code Assistant Implementation Guide (initial tasks)

## Task 1 — Bootstrap project

- Create `go.mod`
- Add cobra root command + subcommands (stubs)
- Add config structs and loader

## Task 2 — Config load + validation

- Implement `ConfigLoader.Load(path)` returning effective config
- Validate required fields:
  - defaults.port_range length=2 and min<max
  - each env has target_instance_id, remote_host, remote_port

- Provide clear errors indicating service/env path

## Task 3 — Session manager MVP

- Implement in-memory manager:
  - StartSession(key, cfg, overrides) -> Session
  - StopSession(key)
  - ListSessions() []SessionSummary
  - GetSession(key)

- Implement port allocator scanning range using `net.Listen("tcp", bind:port)` test.

## Task 4 — aws process runner

- Build aws args for StartPortForwardingSessionToRemoteHost
- Start child process
- Capture stdout/stderr line-by-line into ring buffer
- Readiness poll

## Task 5 — Wire commands

- `connect` calls manager start and prints endpoint
- `ls` prints table
- `stop` stops session(s)
- `logs` prints last N + follow

## Task 6 — Cleanup on exit

- Root command catches SIGINT/SIGTERM and stops all sessions unless `--no-cleanup`

## Task 7 — TUI scaffold (after MVP)

- Add `dbx ui` command.
- Start Bubble Tea app with base model/view and quit handling.

## Task 8 — TUI state and actions

- Build three-pane UI (targets, sessions, logs).
- Wire connect/stop actions to SessionManager.
- Add periodic refresh to reflect state changes.

## Task 9 — TUI logs and cleanup

- Subscribe/unsubscribe to session logs for follow behavior.
- Ensure graceful exit and cleanup parity with CLI.
- Add tests for key interactions and subscription lifecycle.
