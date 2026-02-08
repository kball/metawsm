# Tasks

## TODO


- [ ] Define versioned forum command/event envelopes with correlation and causation IDs
- [ ] Add SQLite schema for forum command-side state, read projections, and forum_events audit table
- [ ] Add Watermill + Redis configuration and topic registry for forum commands/events
- [ ] Implement forum command handlers (open/add-post/assign/change-state/set-priority/close) with invariant validation
- [ ] Integrate operator loop forum consumers for escalation handling, triage actions, and low-autonomy answer flow
- [ ] Implement idempotent projection consumers for forum_thread_views and forum_thread_stats
- [ ] Add metawsm forum CLI commands that publish command messages to Watermill topics
- [ ] Implement docs-sync subscriber default-on with policy override for answered/closed thread summaries
- [ ] Add end-to-end tests for Redis outage, duplicate delivery idempotency, projection lag, and replay recovery
- [ ] Implement forum query/watch APIs over projections and event cursor for CLI/TUI consumers
