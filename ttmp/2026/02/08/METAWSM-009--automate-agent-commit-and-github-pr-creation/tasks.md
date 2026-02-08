# Tasks

## TODO

- [x] Define policy and safety gates for automated commit/PR flow
- [x] Design and add run pull request persistence schema
- [x] Implement service primitives for branch prep and commit creation
- [x] Implement service primitive for GitHub PR creation via gh CLI
- [x] Add metawsm commit command with dry-run previews
- [x] Add metawsm pr command with dry-run previews
- [x] Integrate commit/pr readiness signals into operator loop
- [ ] Add tests for commit/preflight rejection paths
- [x] Add tests for PR creation and persisted metadata
- [ ] Write operator and agent playbook for commit/PR workflow
- [x] Proposal A V1: add auth preflight check using gh auth status and git credential availability
- [x] Proposal A V1: add metawsm auth check command to report push/PR readiness
- [x] Proposal A V1: record credential mode and actor identity in run events for commit/pr actions
- [ ] Validation framework: define extensible check interface and require_all policy semantics
- [ ] Validation V1: enforce all configured test commands must pass before commit/pr
- [ ] Validation V1: add forbidden-file and clean-tree checks as required gates
- [ ] Implement per repo/ticket branch+PR fanout orchestration for multi-repo runs
- [ ] Enforce human-only merge policy in operator and CLI surfaces (no auto-merge path)
- [ ] Add playbook section for Proposal A setup (gh login, git identity, troubleshooting)
- [ ] Add end-to-end test for successful local-auth commit push and PR creation
