# AGENT.md for `metawsm`

## Purpose
`metawsm` is an orchestrator for running multiple agent threads across multiple code workspaces.

It should compose three existing systems:
- `wsm` (workspace-manager): creates and manages multi-repo workspaces via git worktrees.
- `docmgr`: manages ticket/project documentation workspaces under `ttmp/`.
- `tmux`: hosts long-running agent sessions and pane/window orchestration.

This repo currently has no implementation, so start by building thin, reliable orchestration over these tools before adding new internal abstractions.

## Source-of-Truth Tools
Treat these as authoritative and avoid re-implementing their internals in `metawsm`.

### `wsm` (workspace-manager)
What it owns:
- Repository discovery and registry.
- Workspace lifecycle (create/fork/merge/delete/add/remove).
- Branch coordination across repos.
- `go.work` generation for Go workspaces.
- Workspace metadata and setup script execution.
- Tmux session attach/create helper.

Key commands:
- `wsm discover <paths...> [--recursive]`
- `wsm create <name> --repos repo1,repo2 [--branch ...] [--base-branch ...] [--agent-source ...] [--dry-run]`
- `wsm fork <new-name> [source-name] [--branch ...] [--dry-run]`
- `wsm merge [workspace-name] [--dry-run] [--keep-workspace]`
- `wsm status [workspace-name]`
- `wsm list`, `wsm info`, `wsm delete`, `wsm tmux`

Important persisted state and files:
- Repository registry: `~/.config/workspace-manager/registry.json`
- Workspace configs: `~/.config/workspace-manager/workspaces/<name>.json`
- Default workspace root: `~/workspaces/YYYY-MM-DD/`
- Per-workspace metadata: `<workspace>/.wsm/wsm.json`
- Optional setup hooks:
  - `<workspace>/.wsm/setup.sh`
  - `<workspace>/.wsm/setup.d/*.sh`
  - `<workspace>/<repo>/.wsm/setup.sh`
  - `<workspace>/<repo>/.wsm/setup.d/*.sh`

Notable behavior to rely on:
- `--agent-source` copies an `AGENT.md` into the created/forked workspace.
- Fork derives `baseBranch` from source workspace current branch.
- Workspace metadata includes environment variables such as:
  - `WSM_WORKSPACE_NAME`
  - `WSM_WORKSPACE_PATH`
  - `WSM_WORKSPACE_BRANCH`
  - `WSM_WORKSPACE_BASE_BRANCH` (for forks)
  - `WSM_WORKSPACE_REPOS`

### `docmgr`
What it owns:
- Ticket/workspace document lifecycle.
- Frontmatter metadata, topics, vocab, tasks, changelog.
- Relationships between docs and code files.
- Workspace health/status checks and search/indexing.

Key commands:
- `docmgr configure --root ttmp ...` (writes `.ttmp.yaml`)
- `docmgr init` / `docmgr workspace init`
- `docmgr ticket create-ticket --ticket <ID> --title <Title> --topics ...`
- `docmgr ticket rename-ticket --ticket <OLD> --new-ticket <NEW>`
- `docmgr ticket close --ticket <ID> [...]`
- `docmgr doc add --ticket <ID> --doc-type <type> --title <title>`
- `docmgr doc relate --ticket <ID> --doc <path> --file-note path:note`
- `docmgr search --query <text>`
- `docmgr list tickets`, `docmgr list docs --ticket <ID>`
- `docmgr status`, `docmgr doctor`

Root resolution (important when orchestrating from arbitrary directories):
1. `--root`
2. nearest `.ttmp.yaml` (walking up)
3. git root + `/ttmp`
4. current directory + `/ttmp`

Ticket workspace default layout:
- Root path template defaults to `{{YYYY}}/{{MM}}/{{DD}}/{{TICKET}}--{{SLUG}}`
- New ticket folders include `design/`, `reference/`, `playbooks/`, `scripts/`, `sources/`, `.meta/`, `various/`, `archive/`

## `metawsm` Orchestration Model
Build `metawsm` as a coordinator, not a replacement:
- It should call `wsm`, `docmgr`, and `tmux` in well-defined sequences.
- It should keep its own orchestration state minimal and reference external IDs/paths.
- It should be safe to rerun (idempotent where possible) and support dry-run planning.

Suggested high-value flows:
1. `ticket start` flow:
   - Ensure docs root/config exists (`docmgr configure/init` as needed).
   - Create ticket workspace (`docmgr ticket create-ticket`).
   - Create or fork code workspace (`wsm create` or `wsm fork`).
   - Seed/copy `AGENT.md` into workspace (`wsm --agent-source`).
   - Start/attach tmux session (`wsm tmux`).
2. `ticket status` flow:
   - Aggregate `docmgr status` + `wsm status`.
   - Present one combined view for agent/operator decisions.
3. `ticket close` flow:
   - Validate clean/ready branches (`wsm status`).
   - Merge workspace (`wsm merge`).
   - Close/update docs (`docmgr ticket close`, changelog, relations).

## Engineering Guidelines
- Keep integrations explicit and observable (log exact command invocations and outputs).
- Prefer structured outputs from underlying tools when available; parse conservatively.
- Fail with actionable diagnostics (which tool/command failed, cwd, exit code, stderr).
- Do not duplicate git-worktree logic or doc frontmatter business rules in `metawsm`.
- Add abstractions only after at least two real call sites need them.

## Development Commands
- Build: `go build ./...`
- Test: `go test ./...`
- Run binary (once added): `go run ./cmd/metawsm`

## Scope Reminder
Early `metawsm` should optimize for:
- reliable orchestration across many workspaces,
- strong operator ergonomics for multi-agent workflows,
- clear traceability between ticket docs (`docmgr`) and code workspaces (`wsm`).

It should not initially optimize for:
- replacing `docmgr`/`wsm` internals,
- building new VCS/document engines,
- deep custom behavior that already exists in upstream tools.
