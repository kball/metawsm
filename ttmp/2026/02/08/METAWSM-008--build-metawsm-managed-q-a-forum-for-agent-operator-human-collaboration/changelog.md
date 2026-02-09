# Changelog

## 2026-02-08

- Initial workspace created


## 2026-02-08

Reworked Q&A forum design to full Watermill-based event architecture (command topics, domain events, projections, and phased rollout).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md — Defines the Watermill event topology


## 2026-02-08

Incorporated open-question decisions: Watermill from day one, Redis transport, SQLite state persistence, provisional 30-minute SLA defaults; clarified remaining policy questions.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md — Captures architecture decisions and clarifies open policy questions


## 2026-02-08

Resolved remaining forum policy questions: cross-run thread scope, docs-sync default-on, and lower operator autonomy with mandatory human review gates.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md — Captures final policy defaults for thread scope


## 2026-02-08

Aligned implementation phases with resolved defaults: docs-sync default-on (with policy override) and SLA default marked as resolved at 30 minutes.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md — Keeps phase details consistent with resolved architecture policy decisions


## 2026-02-08

Expanded tasks.md into a concrete execution backlog for Watermill+Redis forum implementation, projections, operator integration, docs sync defaults, and resilience testing.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/tasks.md — Tracks phase-aligned implementation tasks for execution


## 2026-02-08

Step 1: Implemented forum domain envelopes, SQLite command/projection/event schema, orchestrator handlers, and metawsm forum CLI with query/watch support (commit e5fb6e4433362077cd8215b40736f2fdf4d8aff2).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — New forum CLI command group and operator-visible interaction surface
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/forum.go — Versioned forum envelope and command/event contracts
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum.go — Service APIs with forum invariant validation and transition controls
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_forum.go — Core forum command/event persistence and projection/query logic

