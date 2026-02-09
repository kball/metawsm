# Forum Runtime Runbook

This runbook covers required setup for forum workflows after daemon cutover.

Scope:
- forum threads/posts (`metawsm forum ...`)
- forum control signals (`metawsm forum signal ...`)
- daemon API/WebSocket verification and recovery

## Runtime Model

Forum handling is daemonized behind `metawsm serve`:
1. CLI forum commands call daemon HTTP endpoints (`--server`, default `http://127.0.0.1:3001`).
2. The daemon writes forum commands/events through shared service APIs.
3. A durable worker loop processes the SQLite outbox and Redis stream transport continuously.
4. WebSocket clients subscribe to `/api/v1/forum/stream` for live updates.
5. Projections update `forum_thread_views` and `forum_thread_stats`.

Forum command workflows are now mandatory daemon mode: if `metawsm serve` is not running, forum commands fail fast.

## Required Prerequisites

Tools/binaries:
- `go`
- `sqlite3`
- `redis-server` (required)
- `git`
- `tmux` (required for run/agent orchestration)
- `docmgr` and `wsm` (required for full run/bootstrap workflows)

Files/config:
- `.metawsm/policy.json` must exist or defaults must be acceptable
- `.metawsm/metawsm.db` is created automatically

Policy fields required for forum runtime:
- `forum.enabled`
- `forum.topics.command_prefix`
- `forum.topics.event_prefix`
- `forum.topics.integration_prefix`
- `forum.redis.url`
- `forum.redis.stream`
- `forum.redis.group`
- `forum.redis.consumer`

## What Must Be Running

Always required:
- reachable Redis matching `forum.redis.url`
- one `metawsm serve` process for the target DB

Required for active run execution:
- per-agent `tmux` sessions started by `run`/`bootstrap`/`resume`

Optional but common:
- `metawsm operator --all` for continuous supervision
- Vite dev server for UI work (`make dev-frontend`)

## Bring-Up Checklist

1. Start Redis:

```bash
redis-server --port 6379
```

2. Initialize policy if needed:

```bash
go run ./cmd/metawsm policy-init
```

3. Start daemon:

```bash
go run ./cmd/metawsm serve --addr :3001 --db .metawsm/metawsm.db
```

4. Verify health/API:

```bash
curl -s http://127.0.0.1:3001/api/v1/health | jq
curl -s http://127.0.0.1:3001/api/v1/forum/stats | jq
```

5. Start run/bootstrap and generate forum traffic:

```bash
go run ./cmd/metawsm bootstrap \
  --ticket METAWSM-002 \
  --repos metawsm \
  --doc-home-repo metawsm

go run ./cmd/metawsm forum ask --run-id RUN_ID --ticket METAWSM-002 --title "Question" --body "Need guidance"
go run ./cmd/metawsm forum list --run-id RUN_ID
go run ./cmd/metawsm forum thread --thread-id THREAD_ID
```

6. Post control signals:

```bash
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type guidance_request --question "Need decision"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type guidance_answer --answer "Proceed"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type completion --summary "Implementation complete"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type validation --status passed --done-criteria "tests pass"
```

7. Optional live stream check:

```bash
websocat ws://127.0.0.1:3001/api/v1/forum/stream
```

## Operational Verification

Use these checks for diagnosis:

- daemon health + worker lag:

```bash
curl -s http://127.0.0.1:3001/api/v1/health | jq
```

- run status:

```bash
go run ./cmd/metawsm status --run-id RUN_ID
```

- focused tests:

```bash
go test ./internal/server ./internal/serviceapi ./internal/orchestrator ./internal/store -count=1
```

- Redis stream inspection:

```bash
redis-cli XINFO STREAM forum.commands.open_thread
redis-cli XINFO GROUPS metawsm-forum
```

- SQLite outbox/events:

```bash
sqlite3 .metawsm/metawsm.db "select status,count(*) from forum_outbox group by status;"
sqlite3 .metawsm/metawsm.db "select sequence,event_type,thread_id,occurred_at from forum_events order by sequence desc limit 20;"
sqlite3 .metawsm/metawsm.db "select projection_name,count(*) from forum_projection_events group by projection_name;"
```

## Failure Modes and Recovery

`connection refused` for forum commands
- Cause: `metawsm serve` is not running or wrong `--server`.
- Fix: start daemon, confirm address, retry command with `--server http://127.0.0.1:3001`.

`forum redis url is empty`
- Cause: `forum.redis.url` is blank in policy.
- Fix: set required `forum.redis.*` fields and restart daemon.

`forum redis ping failed`
- Cause: Redis is unavailable at configured URL.
- Fix: start Redis, verify URL/port/db, then restart daemon.

`database is locked`
- Cause: heavy concurrent SQLite access.
- Fix: reduce competing processes on one DB, retry operation.

Run stuck in `awaiting_guidance`
- Cause: pending `guidance_request` lacks `guidance_answer`.
- Fix: post `guidance_answer` with `metawsm forum signal`.

Close blocked for bootstrap runs
- Cause: missing `completion`/`validation` control signals, or mismatched done criteria.
- Fix: post both signals per agent and ensure done criteria match bootstrap brief.

## Logging Guidance

- Default logging is stdout from `metawsm serve`.
- For optional file retention, redirect stdout/stderr to a log file in your supervisor/service wrapper.
- Structured sync state remains queryable from SQLite (`doc_sync_states`) and status/API outputs.

## Operator Guidance

- Forum/control signals are the only lifecycle signaling path.
- Do not rely on legacy `.metawsm/*.json` signal files.
- For automation, prefer typed service/API payloads over parsing CLI text output.
