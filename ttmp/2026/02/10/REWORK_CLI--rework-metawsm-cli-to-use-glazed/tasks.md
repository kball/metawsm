# Tasks

## TODO

- [x] Establish CLI baseline matrix and ensure parser/usage tests cover current behavior
- [x] Add glazed dependency and create root cobra/glazed command scaffolding
- [x] Migrate low-risk standalone commands (policy-init, serve, docs) to glazed
- [x] Migrate run-selector command family (status/resume/stop/restart/cleanup/merge/commit/pr/iterate/close) with shared selector layer
- [ ] Migrate grouped command trees (auth check, review sync, forum subcommands)
- [ ] Migrate watch/operator/tui loop commands with equivalent signal and runtime behavior
- [ ] Remove legacy flag.NewFlagSet switch path and keep compatibility aliases where needed
- [ ] Run full regression validation (go test ./..., targeted CLI smoke checks, docs/help updates)
