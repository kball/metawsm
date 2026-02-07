# Tasks

## TODO

- [x] Add `metawsm bootstrap --ticket --repos` command with `--repos` mandatory validation and intake completeness gates.
- [x] Persist Run Brief and intake Q/A transcript in SQLite, keyed by run id.
- [x] Auto-create the docs ticket workspace when `--ticket` is missing.
- [x] Generate/update a ticket reference document containing the run brief and resolved scope.
- [x] Extend orchestration lifecycle with explicit guidance-needed pause state and resume path.
- [x] Implement sentinel guidance signal at `<workspace>/.metawsm/guidance-request.json`.
- [x] Add `metawsm guide --run-id --answer` command to feed user guidance back into execution.
- [ ] Enforce completion checks from Run Brief before close/merge.
- [ ] Add integration tests for intake, guidance loop, and merge-ready completion.
- [ ] Add operator playbook documenting bootstrap workflow and failure recovery.
