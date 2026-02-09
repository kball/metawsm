# Tasks

## TODO


- [x] Define versioned forum command/event envelopes with correlation and causation IDs
- [x] Add SQLite schema for forum command-side state, read projections, and forum_events audit table
- [x] Add Watermill + Redis configuration and topic registry for forum commands/events
- [x] Implement forum command handlers (open/add-post/assign/change-state/set-priority/close) with invariant validation
- [x] Integrate operator loop forum consumers for escalation handling, triage actions, and low-autonomy answer flow
- [x] Implement idempotent projection consumers for forum_thread_views and forum_thread_stats
- [x] Add metawsm forum CLI commands that publish command messages to Watermill topics
- [x] Implement docs-sync subscriber default-on with policy override for answered/closed thread summaries
- [ ] Add end-to-end tests for Redis outage, duplicate delivery idempotency, projection lag, and replay recovery
- [x] Implement forum query/watch APIs over projections and event cursor for CLI/TUI consumers
- [x] T1: Add forum command/event envelope structs (with event_id, version, correlation_id, causation_id)
- [x] T2: Add SQLite forum tables (threads/posts/transitions/assignments/events/views/stats)
- [x] T3: Add forum topic registry + Redis transport config surface in policy/service
- [x] T4: Implement forum command handlers with invariant validation
- [x] T5: Add forum query/watch service APIs over projection tables
- [x] T6: Add metawsm forum CLI command group and subcommands
- [x] T7: Add tests for command handling, projection idempotency, and watch cursor
