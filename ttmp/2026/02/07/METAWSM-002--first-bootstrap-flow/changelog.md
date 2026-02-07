# Changelog

## 2026-02-07

- Initial workspace created


## 2026-02-07

Researched METAWSM-001 baseline and authored minimum bootstrap flow design with implementation tasks

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/design-doc/01-minimum-bootstrap-flow.md — Primary design artifact for this ticket
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/tasks.md — Implementation backlog derived from the design


## 2026-02-07

Locked bootstrap decisions: guidance via workspace sentinel file, auto-create missing tickets, and mandatory --repos for v1

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/design-doc/01-minimum-bootstrap-flow.md — Updated MVP decisions and implementation plan
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/tasks.md — Updated backlog to reflect mandatory repos and guidance sentinel implementation


## 2026-02-07

Implemented bootstrap and guide flow: mandatory repos, auto ticket create, run-brief persistence, guidance sentinel lifecycle, and tests

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Bootstrap and guide CLI implementation
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Bootstrap mode run lifecycle and guidance handling
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Guide workflow regression coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Run brief and guidance request persistence


## 2026-02-07

Added bootstrap close validation gates and published operator playbook with test procedures

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Intake prompt and non-interactive completeness tests
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Bootstrap close checks for validation-result and pending-guidance blocking
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Coverage for bootstrap close blocked/passing validation scenarios
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/playbook/01-bootstrap-operator-playbook.md — Operator playbook for manual validation


## 2026-02-07

Fixed bootstrap ticket auto-create detection to parse structured ticket list output

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Robust ticket existence detection using docmgr JSON output
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Coverage for debug-prefixed JSON array extraction


## 2026-02-07

Fixed .gitignore pattern to ensure cmd/metawsm source is tracked and committed

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/.gitignore — Narrowed ignore from metawsm to /metawsm to avoid masking cmd/metawsm
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — CLI entrypoint now tracked in git history


## 2026-02-07

Fixed workspace naming collision so repeated bootstrap runs generate distinct workspace names

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — workspaceNameFor now uses normalized run token instead of first 8 chars
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Regression coverage for distinct workspace names across run ids


## 2026-02-07

Added first-class `restart` and `cleanup` commands with ticket-based run lookup and operator docs

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added `metawsm restart` and `metawsm cleanup` command handlers and usage text
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added ticket/run-id resolver for restart/cleanup APIs
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added dry-run coverage for restart/cleanup by ticket
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Added latest-run lookup by ticket
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md — Documented new restart/cleanup commands
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/playbook/01-bootstrap-operator-playbook.md — Added operator recovery commands


## 2026-02-07

Fixed bootstrap workspace base-branch behavior and tmux agent startup durability

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Aligns newly-created workspace branches to a configurable base branch and wraps tmux agent command to keep session alive
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added `--base-branch` support for `run` and `bootstrap`
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — Added `workspace.base_branch` policy field with default `main`
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added regression tests for base-branch reset and tmux wrapper behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json — Included `workspace.base_branch`


## 2026-02-07

Fixed ticket-based cleanup/restart run selection to skip dry-run runs and made workspace deletion idempotent

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Ticket resolver now prefers non-dry-run runs; cleanup ignores missing workspace deletes
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Added run-id listing API for ticket-based selection
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added regression tests for non-dry-run selection and missing-workspace output matching


## 2026-02-07

Fixed false-healthy bootstrap agent reporting and hardened tmux startup retries

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added pane-based agent exit-code detection, startup wait checks, restart verification, and bootstrap run failure transition on failed agents
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added tests for exit-code parsing, codex command normalization, and failed-agent run transition
