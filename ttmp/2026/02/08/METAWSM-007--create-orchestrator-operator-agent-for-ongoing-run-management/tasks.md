# Tasks

## TODO

- [x] Phase 1: add operator config schema (thresholds + llm block) in policy and validation
- [x] Phase 1: default llm runtime to Codex CLI with llm-mode=assist
- [ ] Phase 2: add SQLite persistence for operator restart attempts and cooldown timestamps
- [ ] Phase 2: add store tests proving operator retry/cooldown state survives service restart
- [ ] Phase 3: add metawsm operator command/flags and loop scaffold (selector, interval, notify, llm-mode)
- [ ] Phase 3: implement deterministic stale-run evidence checks via tmux session/activity/log signals
- [ ] Phase 4: implement Codex CLI adapter with strict JSON response schema validation
- [ ] Phase 4: implement deterministic policy gate that allowlists llm intents before execution
- [ ] Phase 5: wire restart/stop actions through orchestrator service APIs with decision_source tagging
- [ ] Phase 5: write escalation summaries into workspace-active ticket docs under doc_home_repo ttmp path
- [ ] Phase 6: add tests for assist-vs-auto execution safety and malformed llm output fallback
- [ ] Phase 6: add tests for stale-run stop/skip behavior when runtime evidence indicates active work
- [ ] Phase 7: update README/operator docs with workspace-active escalation-summary behavior
