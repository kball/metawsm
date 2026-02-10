# metawsm

`metawsm` orchestrates agent work across multiple tickets and workspaces by composing:
- `docmgr` for ticket/document lifecycle,
- `wsm` for workspace lifecycle,
- `tmux` for per-agent runtime sessions.

## Current MVP

Implemented command surface:
- `metawsm run`
- `metawsm bootstrap`
- `metawsm status`
- `metawsm auth check`
- `metawsm review sync`
- `metawsm watch`
- `metawsm operator`
- `metawsm guide`
- `metawsm resume`
- `metawsm stop`
- `metawsm restart`
- `metawsm cleanup`
- `metawsm commit`
- `metawsm pr`
- `metawsm merge`
- `metawsm iterate`
- `metawsm close`
- `metawsm policy-init`
- `metawsm tui`
- `metawsm docs`
- `metawsm web`

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
  --doc-home-repo metawsm \
  --doc-seed-mode copy_from_repo_on_start \
  --agent agent \
  --dry-run
```

Start a bootstrap run with interactive intake:

```bash
go run ./cmd/metawsm bootstrap \
  --ticket METAWSM-002 \
  --repos metawsm \
  --doc-home-repo metawsm \
  --base-branch main
```

Inspect status:

```bash
go run ./cmd/metawsm status --ticket METAWSM-003
```

Answer pending guidance from an agent:

```bash
go run ./cmd/metawsm guide --ticket METAWSM-003 --answer "Proceed with the sentinel JSON contract."
```

Review/merge workflow for completed runs:

```bash
go run ./cmd/metawsm merge --ticket METAWSM-003 --dry-run
go run ./cmd/metawsm merge --ticket METAWSM-003
```

Send operator feedback and kick off another agent iteration:

```bash
go run ./cmd/metawsm iterate --ticket METAWSM-003 --feedback "Address the diff comments and add regression tests."
```

Sync PR review comments and optionally dispatch them into iterate workflow:

```bash
# sync only (preview)
go run ./cmd/metawsm review sync --ticket METAWSM-003 --dry-run

# sync + dispatch queued review feedback to agents
go run ./cmd/metawsm review sync --ticket METAWSM-003 --dispatch
```

Restart the latest run for a ticket:

```bash
go run ./cmd/metawsm restart --ticket METAWSM-003
```

Run the operator supervision loop:

```bash
# supervise all active runs
go run ./cmd/metawsm operator --all --interval 15

# run in assist mode (default from policy)
go run ./cmd/metawsm operator --all --llm-mode assist

# run in auto mode
go run ./cmd/metawsm operator --all --llm-mode auto

# observe only (no actions)
go run ./cmd/metawsm operator --all --dry-run
```

Clean up the latest run for a ticket (kills agent tmux sessions and deletes workspaces):

```bash
go run ./cmd/metawsm cleanup --ticket METAWSM-003
```

Keep workspaces during cleanup:

```bash
go run ./cmd/metawsm cleanup --ticket METAWSM-003 --keep-workspaces
```

Run initial TUI monitor:

```bash
# Monitor one run
go run ./cmd/metawsm tui --run-id RUN_ID

# Monitor all active runs
go run ./cmd/metawsm tui
```

Doc federation view and optional index refresh:

```bash
go run ./cmd/metawsm docs --policy .metawsm/policy.json
go run ./cmd/metawsm docs --policy .metawsm/policy.json --refresh
```

Run the web dashboard backend (serves API + embedded/static SPA):

```bash
go run ./cmd/metawsm web --addr :3001
```

For local frontend development with Vite HMR:

```bash
npm --prefix ui install
make dev-backend
make dev-frontend
```

## Policy

Default policy file: `.metawsm/policy.json`.
Reference example: `examples/policy.example.json`.

Important fields:
- `workspace.default_strategy` (`create|fork|reuse`)
- `tmux.session_pattern` (supports `{agent}` and `{workspace}`)
- `workspace.base_branch` (branch used as workspace start-point; default `main`)
- `health.idle_seconds`
- `health.activity_stalled_seconds`
- `health.progress_stalled_seconds`
- `operator.unhealthy_confirmations`
- `operator.restart_budget`
- `operator.restart_cooldown_seconds`
- `operator.stale_run_age_seconds`
- `operator.llm.mode` (`off|assist|auto`)
- `operator.llm.command` (V1 default: `codex`)
- `operator.llm.timeout_seconds`
- `operator.llm.max_tokens`
- `git_pr.mode` (`off|assist|auto`)
- `git_pr.require_all` (require all configured checks to pass)
- `git_pr.required_checks` (`tests|forbidden_files|clean_tree`)
- `git_pr.test_commands[]` (shell commands run in each target repo)
- `git_pr.forbidden_file_patterns[]` (glob patterns blocked in changed files)
- `git_pr.allowed_repos[]` (optional allow-list for commit/PR workflows)
- `git_pr.default_labels[]` and `git_pr.default_reviewers[]`
- `git_pr.review_feedback.enabled` (enable PR review feedback sync)
- `git_pr.review_feedback.mode` (`assist|auto`)
- `git_pr.review_feedback.include_review_comments` (V1 must be `true`)
- `git_pr.review_feedback.ignore_authors[]` (optional commenter ignore list)
- `git_pr.review_feedback.max_items_per_sync` (ingest cap per sync pass)
- `git_pr.review_feedback.auto_dispatch_cap_per_interval` (operator auto cap)
- `close.require_clean_git`
- `docs.authority_mode` (`workspace_active`)
- `docs.seed_mode` (`none|copy_from_repo_on_start`)
- `docs.api.workspace_endpoints[]` (workspace-scoped docmgr API endpoints)
- `docs.api.repo_endpoints[]` (repo fallback docmgr API endpoints)
- `docs.api.request_timeout_seconds`
- `agent_profiles[].runner` (currently `codex` or `shell`)
- `agent_profiles[].base_prompt`
- `agent_profiles[].skills`
- `agents[].profile` (maps each agent to an `agent_profiles` entry)

Kickoff doc-home selection:
- `--doc-home-repo` selects which workspace repo hosts `ttmp/` for docmgr operations.
- `--doc-repo` remains as a legacy alias for compatibility.
- Default behavior picks the first `--repos` entry.

## Bootstrap Signals

For bootstrap runs, agents communicate through workspace files:
- Guidance request: `<workspace>/.metawsm/guidance-request.json`
- Guidance response (written by `metawsm guide`): `<workspace>/.metawsm/guidance-response.json`
- Completion marker: `<workspace>/.metawsm/implementation-complete.json`
- Validation gate (required before close for bootstrap runs):
  `<workspace>/.metawsm/validation-result.json` with `status="passed"` and `done_criteria` matching the run brief.

## Operator Escalation Summaries

When `metawsm operator` escalates in environments using `docs.authority_mode=workspace_active`, it appends escalation summaries to workspace ticket docs:

- `<workspace>/<doc_home_repo>/ttmp/.../<ticket>/changelog.md`

Entries include run id, escalation intent, summary/evidence, and requested operator decision.

## Build & Test

```bash
npm --prefix ui install
go generate ./internal/web
go test ./... -count=1
go build ./...
go build -tags embed ./...
```
