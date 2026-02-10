# Changelog

## 2026-02-10

- Initial workspace created


## 2026-02-10

Created REWORK_CLI ticket, analyzed current metawsm CLI flag architecture, explored glazed command/layer APIs, and authored a phased migration plan plus task checklist.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Analyzed current CLI flag architecture and command dispatch
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/design-doc/01-plan-migrate-metawsm-cli-to-glazed.md — Documented migration plan and design decisions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/tasks.md — Added phased implementation checklist


## 2026-02-10

Step 1: Established CLI baseline matrix and usage guardrails (commit de9f9d6eaa5427c8db4a0b7afc83818e53816da6)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Introduced usageCommandLines and usageText for stable baseline
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Added usage matrix regression test
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/reference/01-cli-baseline-matrix.md — Captured baseline command families and parser counts


## 2026-02-10

Step 2-3: Added glazed root scaffolding and migrated low-risk commands policy-init/docs/serve (commit 16bf5a39d8a53faa5d63342b2b74d5f1928bf115)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/glazed_low_risk_commands.go — Glazed command implementations for policy-init/docs/serve
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Entrypoint switched to executeCLI
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Root command registration test
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/root_command.go — Root command registration via cobra and glazed


## 2026-02-10

Step 4: Migrated run-selector family to glazed with shared selector layer (commit d94292e2b5b17d9255a8c339b8b89190e2f49b04)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/glazed_run_selector_commands.go — Implemented status/resume/stop/restart/cleanup/merge/commit/pr/iterate/close as glazed commands
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/root_command.go — Registered run-selector glazed commands in root


## 2026-02-10

Step 5: Migrated grouped command trees for auth/review/forum hierarchy (commit 313863de313c267dab6325aa47e5602327bc5bc0)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/glazed_grouped_commands.go — Implemented auth check and review sync as glazed grouped commands plus forum subcommand tree wrappers
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/root_command.go — Registered grouped trees and removed legacy top-level grouped passthrough


## 2026-02-10

Step 7-8: Confirmed legacy switch removal and passed full regression validation (go test ./...)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Entrypoint uses executeCLI without legacy command switch
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/tasks.md — All ticket tasks complete


## 2026-02-10

Step 6: Migrated watch/operator/tui command registration to glazed wrappers (commit 0b8ccdbae922888d1e88d11b44fce0443f9542be)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/glazed_loop_commands.go — Added glazed wrappers for watch/operator/tui with compatibility forwarding
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/root_command.go — Removed legacy passthrough and registered migrated loop commands

