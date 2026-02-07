# Tasks

## TODO


- [x] Scaffold metawsm CLI commands: run, status, resume, stop, close
- [x] Define v1 declarative policy schema (workspace strategy, health thresholds, close gates, tmux layout)
- [x] Implement policy loader + validation with clear error messages
- [x] Define HSM state model for run, step, and agent lifecycles with legal transitions
- [x] Design SQLite schema and migration strategy for runs, steps, agents, events, transitions
- [x] Implement SQLite store + migration bootstrap at .metawsm/metawsm.db
- [x] Implement RunSpec + plan compiler supporting multi-ticket runs in parallel
- [x] Set default workspace strategy to wsm create with explicit opt-in for fork
- [x] Implement command executor for docmgr/wsm/tmux steps with structured logs and retries
- [x] Implement tmux adapter with one session namespace per agent/workspace pair
- [x] Persist transition/event history for auditability and deterministic resume
- [x] Implement agent health evaluator (liveness + activity heartbeat + progress heartbeat)
- [x] Implement metawsm status output aggregating run state and health state
- [x] Implement resume and stop flows as HSM transitions
- [x] Implement close flow with clean-git preflight gate before any merge
- [x] Integrate close flow with wsm merge and docmgr ticket close after gate passes
- [x] Create initial TUI run monitor for active runs and per-agent state
- [x] Add tests for HSM transition legality and recovery paths
- [x] Add tests for SQLite persistence and migration compatibility
- [x] Add integration tests for multi-ticket dry-run and execution planning
- [x] Document policy examples and operator playbook for run/resume/close workflows
