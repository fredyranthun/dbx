# dbx — AWS SSM Port-Forwarding Session Manager

`dbx` is a configuration-oriented CLI that helps you **start, manage, and stop multiple AWS SSM port-forwarding sessions** (jumpbox ➜ remote host) from a **single terminal**.

It’s **engine-agnostic**: dbx only provides `localhost:<port>` endpoints so you can connect using **DBeaver**, your app, or any client you prefer.

> Sessions are managed by `dbx` and (by default) are stopped when `dbx` exits.

---

## Features (MVP)

- ✅ Start multiple SSM port-forward sessions concurrently
- ✅ Keep everything manageable from one terminal
- ✅ `connect`, `ls`, `logs`, `stop` commands
- ✅ Configuration-oriented (YAML/JSON)
- ✅ Safe defaults (binds to `127.0.0.1`)

Planned / optional:

- ⏳ Interactive TUI (`dbx ui`)
- ⏳ `dbx doctor` (dependency + AWS auth checks)

---

## Requirements

- **AWS CLI v2** installed and available in `PATH`
- **Session Manager Plugin** installed (required for `aws ssm start-session`)
- Access to the target EC2 instance via SSM and permission to start sessions
- Linux / WSL recommended (macOS likely works too)

Quick checks:

```bash
aws --version
aws ssm start-session --help
aws sts get-caller-identity
```

---

## Installation

### Build from source

```bash
git clone <REPO_URL>
cd dbx
go build -o dbx ./cmd/dbx
./dbx --help
```

(Optional) Put it in your PATH:

```bash
sudo mv ./dbx /usr/local/bin/dbx
```

---

## Configuration

dbx loads config in this order:

1. `--config <path>`
2. `$DBX_CONFIG`
3. `~/.dbx/config.yml` (also supports `.yaml` or `.json`)

### Example config (YAML)

Create `~/.dbx/config.yml`:

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
        remote_host: "mydb.xxxxxx.sa-east-1.rds.amazonaws.com"
        remote_port: 5432
      stg:
        target_instance_id: "i-0abcdef1234567890"
        remote_host: "mydb-stg.xxxxxx.sa-east-1.rds.amazonaws.com"
        remote_port: 5432

  - name: service2
    envs:
      dev:
        target_instance_id: "i-0fedcba9876543210"
        remote_host: "10.0.12.34"
        remote_port: 3306
```

### Notes

- `target_instance_id`: the **jumpbox EC2 instance** that has network access to the remote DB host
- `remote_host`: **reachable from the jumpbox** (RDS endpoint, private DNS name, or IP)
- `remote_port`: DB port (e.g., 5432 for Postgres, 3306 for MySQL)
- dbx does **not** store DB credentials (use your DB client for auth)

---

## Usage

### Start a tunnel

```bash
dbx connect service1 dev
```

Output includes a machine-friendly endpoint line:

```txt
service=service1 env=dev
remote=mydb...:5432
ENDPOINT=127.0.0.1:5512
```

You can then connect using DBeaver (or any client) to:

- Host: `127.0.0.1`
- Port: `5512`

### List running sessions

```bash
dbx ls
```

Shows active sessions with:

- service/env
- local endpoint (bind:port)
- remote host:port
- status
- uptime
- PID

### Follow logs

```bash
dbx logs service1/dev --follow
```

Or print last N lines:

```bash
dbx logs service1/dev --lines 200
```

### Stop a session

```bash
dbx stop service1/dev
# or
dbx stop service1 dev
```

Stop all:

```bash
dbx stop --all
```

---

## Overrides (flags)

You can override defaults per command:

```bash
dbx connect service1 dev --region us-east-1 --profile myprofile --bind 127.0.0.1 --port 55432
```

Common overrides:

- `--profile` AWS profile
- `--region` AWS region
- `--bind` local bind interface (default `127.0.0.1`)
- `--port` force a specific local port

---

## How it works (high level)

dbx starts a child process like:

```bash
aws ssm start-session \
  --target i-0123... \
  --document-name AWS-StartPortForwardingSessionToRemoteHost \
  --parameters host=["remote_host"],portNumber=["remote_port"],localPortNumber=["local_port"] \
  --region sa-east-1 \
  --profile corp
```

dbx then:

- captures stdout/stderr (logs)
- waits until `bind:local_port` is listening (readiness)
- keeps process running until you stop it (or dbx exits)

---

## Security

- Default bind is `127.0.0.1` so tunnels are only accessible locally.
- Avoid using `0.0.0.0` unless you understand the implications (it exposes the local port on your network).

---

## Development

### Project structure

```
dbx/
  cmd/dbx/main.go
  internal/config/...
  internal/session/...
  README.md
  PRD.md
```

### Running locally

```bash
go run ./cmd/dbx --help
go run ./cmd/dbx connect service1 dev
```

---

## Troubleshooting

### `aws: command not found`

Install AWS CLI v2 and ensure it’s in PATH.

### `SessionManagerPlugin is not found`

Install the AWS Session Manager Plugin for your OS/WSL environment.

### `connect` hangs or times out

- Verify you can start SSM sessions:

  ```bash
  aws ssm start-session --target <instance-id>
  ```

- Confirm jumpbox can reach `remote_host:remote_port`
- Increase `startup_timeout_seconds` in config

### Port range exhausted

Increase `port_range` in config or stop unused sessions.

---

## License

TBD
