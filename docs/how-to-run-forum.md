# Forum Runtime Runbook

This document explains what must be configured and running for `metawsm` forum workflows to work reliably.

Scope:
- forum threads/posts (`metawsm forum ...`)
- forum control signals (`metawsm forum signal ...`)
- run lifecycle gates that now depend on forum control state

## Runtime Model (What Is Actually Running)

Forum handling is embedded inside each `metawsm` command process, and transport is backed by Redis Streams.

There is currently no separate always-on forum daemon in this repo. Instead:
1. command handlers enqueue messages into `forum_outbox` (SQLite)
2. in-process bus workers publish outbox messages to Redis stream topics
3. resulting forum events are published to `forum.events.*`
4. in-process subscribers consume Redis stream messages and run topic handlers
5. projection handlers update `forum_thread_views` and `forum_thread_stats`

This means forum behavior is active whenever a `metawsm` command that constructs the service is running.

## Required Prerequisites

Tools/binaries:
- `go`
- `sqlite3`
- `redis-server` (required)
- `git`
- `tmux` (required for agent session orchestration)
- `docmgr` and `wsm` (required for full run/bootstrap workflows)

Files/config:
- `.metawsm/policy.json` must exist or defaults must be acceptable
- `.metawsm/metawsm.db` will be created automatically by service initialization

Policy fields that must be valid for forum startup:
- `forum.enabled`
- `forum.topics.command_prefix`
- `forum.topics.event_prefix`
- `forum.topics.integration_prefix`
- `forum.redis.url`
- `forum.redis.stream`
- `forum.redis.group`
- `forum.redis.consumer`

Important current behavior:
- `forum.redis.*` is validated and used by runtime transport.
- Redis Streams are required for command/event delivery.
- SQLite remains the durable outbox staging store.

## What Must Be Running

Always required:
- a `metawsm` command process (for whichever action you are performing)
- reachable Redis server matching `forum.redis.url`

Required for active run execution:
- per-agent `tmux` sessions (started by `run`/`bootstrap`/`resume`)

Optional but common:
- `metawsm operator --all` (continuous supervision/escalation)
- `metawsm watch ...` (alert-focused monitoring)

Not required as a separate process right now:
- dedicated forum worker daemon (workers run in-process in `metawsm` commands)

## Bring-Up Checklist

1. Start Redis (local default):

```bash
redis-server --port 6379
```

2. Initialize policy if needed:

```bash
go run ./cmd/metawsm policy-init
```

3. Confirm forum config is present and non-empty:

```bash
rg -n '"forum"|"command_prefix"|"event_prefix"|"integration_prefix"|"url"|"stream"|"group"|"consumer"' .metawsm/policy.json
```

4. Start a run or bootstrap flow:

```bash
go run ./cmd/metawsm bootstrap \
  --ticket METAWSM-002 \
  --repos metawsm \
  --doc-home-repo metawsm
```

5. Create/read forum traffic:

```bash
go run ./cmd/metawsm forum ask --run-id RUN_ID --ticket METAWSM-002 --title "Question" --body "Need guidance"
go run ./cmd/metawsm forum list --run-id RUN_ID
go run ./cmd/metawsm forum thread --thread-id THREAD_ID
```

6. Post control signals (forum-first lifecycle path):

```bash
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type guidance_request --question "Need decision"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type guidance_answer --answer "Proceed"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type completion --summary "Implementation complete"
go run ./cmd/metawsm forum signal --run-id RUN_ID --ticket METAWSM-002 --agent-name agent --type validation --status passed --done-criteria "tests pass"
```

7. Verify run state reflects forum control state:

```bash
go run ./cmd/metawsm status --run-id RUN_ID
```

## Operational Verification

Use these checks when diagnosing forum issues:

- app-level check:

```bash
go run ./cmd/metawsm status --run-id RUN_ID
```

- focused test suites:

```bash
go test ./internal/forumbus ./internal/store ./internal/orchestrator -count=1
```

- Redis stream inspection:

```bash
redis-cli XINFO STREAM forum.commands.open_thread
redis-cli XINFO GROUPS metawsm-forum
```

- inspect outbox and events directly:

```bash
sqlite3 .metawsm/metawsm.db "select status,count(*) from forum_outbox group by status;"
sqlite3 .metawsm/metawsm.db "select sequence,event_type,thread_id,occurred_at from forum_events order by sequence desc limit 20;"
sqlite3 .metawsm/metawsm.db "select projection_name,count(*) from forum_projection_events group by projection_name;"
```

## Failure Modes and Recovery

`forum redis url is empty`
- Cause: `forum.redis.url` is blank in policy.
- Fix: set non-empty `forum.redis.url` (and other required `forum.redis.*` fields), then rerun command.

`forum redis ping failed`
- Cause: Redis is not running or unreachable at configured URL.
- Fix: start Redis, verify host/port/db in `forum.redis.url`, and retry.

`no handler for topic ...` in outbox failures
- Cause: topic prefixes/config mismatch or handler registration regression.
- Fix: verify `forum.topics.*` values and code defaults; run tests above; re-run a forum command to drive outbox processing again.

SQLite lock/contention (`database is locked`)
- Cause: many concurrent writers/readers against `.metawsm/metawsm.db`.
- Fix: reduce concurrent command loops; retry command; avoid multiple heavy loops on same DB.

Run stuck in `awaiting_guidance`
- Cause: pending `guidance_request` without matching `guidance_answer` in the control thread.
- Fix: post a `guidance_answer` with `metawsm forum signal ... --type guidance_answer ...`.

Close blocked for bootstrap runs
- Cause: missing `completion` or `validation` control signals, or validation criteria mismatch.
- Fix: post `completion` and `validation` signals for each agent; ensure validation `done_criteria` matches run brief expectation.

## Durable Operator Guidance

- Use forum commands and control signals as the only lifecycle signaling path.
- Do not rely on legacy `.metawsm/*.json` file signaling.
- Treat `status` output as human-readable; automation paths should prefer typed service APIs in code (`RunSnapshot`).
