# metawsm

`metawsm` orchestrates agent work across multiple tickets and workspaces by composing:
- `docmgr` for ticket/document lifecycle,
- `wsm` for workspace lifecycle,
- `tmux` for per-agent runtime sessions.

## Current MVP

Implemented command surface:
- `metawsm run`
- `metawsm status`
- `metawsm resume`
- `metawsm stop`
- `metawsm close`
- `metawsm policy-init`
- `metawsm tui`

Key implementation decisions:
- HSM-driven lifecycle transitions for run/step/agent states.
- SQLite durable state in `.metawsm/metawsm.db`.
- Declarative policy file at `.metawsm/policy.json`.
- Tmux session topology is per `agent/workspace` pair.
- Close flow enforces clean git state before merge.

## Quick Start

Initialize policy:

```bash
go run ./cmd/metawsm policy-init
```

Plan a run (no side effects):

```bash
go run ./cmd/metawsm run \
  --ticket METAWSM-001 \
  --repos metawsm \
  --agent agent \
  --dry-run
```

Inspect status:

```bash
go run ./cmd/metawsm status --run-id RUN_ID
```

Run initial TUI monitor:

```bash
# Monitor one run
go run ./cmd/metawsm tui --run-id RUN_ID

# Monitor all active runs
go run ./cmd/metawsm tui
```

## Policy

Default policy file: `.metawsm/policy.json`.
Reference example: `examples/policy.example.json`.

Important fields:
- `workspace.default_strategy` (`create|fork|reuse`)
- `tmux.session_pattern` (supports `{agent}` and `{workspace}`)
- `health.idle_seconds`
- `health.activity_stalled_seconds`
- `health.progress_stalled_seconds`
- `close.require_clean_git`

## Build & Test

```bash
go test ./...
go build ./...
```
