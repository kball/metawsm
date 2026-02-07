# Changelog

## 2026-02-07

- Initial workspace created


## 2026-02-07

Step 1: Implemented initial metawsm MVP (CLI, policy, HSM, SQLite store, orchestration service, tests, and diary updates).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Exposes command surface for operators
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Implements run lifecycle
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Persists durable orchestrator state


## 2026-02-07

Step 2: Committed MVP snapshot (e4f6c66) and implemented initial TUI monitor command with active-run dashboard support.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — TUI command implementation
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — ActiveRuns + tmux helper refinements
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — ListRuns query for monitor

