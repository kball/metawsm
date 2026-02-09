# Tasks

## TODO

- [x] T1: Define and implement forum control payload schemas (`guidance_request`, `guidance_answer`, `completion`, `validation`) with explicit versioning.
- [x] T2: Enforce exactly one control thread per `(run_id, agent_name)` at service and store layers (validation + unique constraint/index + migration guard).
- [x] T3: Add Watermill + Redis runtime package (`internal/forumbus`) with router lifecycle, publisher/subscriber wiring, and health checks.
- [x] T4: Add durable outbox table and worker for command/event publish reliability (state+event+outbox atomic write, retry, replay).
- [x] T5: Refactor forum command entrypoints to dispatcher abstraction and switch to bus-backed command publishing.
- [x] T6: Implement async Watermill command consumers for open/add-post/assign/state/priority/close command topics.
- [ ] T7: Implement projection consumers for `forum.events.*` and idempotent projection application via `forum_projection_events`.
- [x] T8: Refactor `Guide` flow to forum-answer command path only; remove legacy `.metawsm/guidance-response.json` writes.
- [x] T9: Refactor `syncBootstrapSignals()` to forum-only control-state derivation; remove legacy file-ingestion control path.
- [x] T10: Refactor `ensureBootstrapCloseChecks()` to forum completion/validation semantics only; remove file-based close gates.
- [ ] T11: Add typed run snapshot API and migrate `watch`/`operator` off status-text parsing.
- [x] T12: Remove `metawsm guide` command from CLI surface and replace with forum command guidance in help/hints.
- [x] T13: Remove all legacy file-signal readers/writers (`guidance-request/response`, `implementation-complete`, `validation-result`) from runtime code.
- [ ] T14: Update docs (`README.md`, `docs/system-guide.md`, ticket docs/playbooks) to forum-first-only control flow and commands.
- [ ] T15: Add integration tests for Redis unavailable (startup/mid-run), duplicate delivery idempotency, projection lag catch-up, and outbox replay recovery.
- [ ] T16: Add end-to-end tests for forum-only lifecycle (ask -> answer -> resume -> completion -> validation -> close) with one-thread-per-agent enforcement.
- [ ] T17: Execute production cutover checklist: enable forum-first path globally, verify metrics/alerts, and remove any remaining runtime migration toggles.
