# Tasks

## TODO

- [x] Add server-side forum event broker for websocket fanout (ticket-filtered subscriptions)
- [ ] Rework `/api/v1/forum/stream` to use catch-up + live event push instead of timer polling loop
- [ ] Update websocket tests in `internal/server/api_test.go` for event/heartbeat behavior
- [ ] Update UI websocket handler to ignore heartbeat frames and debounce refresh on event frames
- [ ] Reduce automatic debug panel refresh frequency (no refresh per websocket frame)
- [ ] Add "Ask Question" composer in forum UI for human-originated thread creation
- [ ] Wire ask submit flow to `POST /api/v1/forum/threads` with viewer-backed actor identity
- [ ] Add UI tests for ask validation and successful thread creation path
- [ ] Run manual validation: idle UI no longer generates continuous refresh traffic
- [ ] Run manual validation: human can create a new thread and see it appear live
