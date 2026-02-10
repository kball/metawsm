package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/serviceapi"
)

func TestHandleRuns(t *testing.T) {
	core := &mockCore{
		listRunSnapshotsFn: func(_ context.Context, ticket string) ([]serviceapi.RunSnapshot, error) {
			if ticket != "METAWSM-011" {
				t.Fatalf("expected ticket filter METAWSM-011, got %q", ticket)
			}
			return []serviceapi.RunSnapshot{
				{RunID: "run-2", Tickets: []string{"METAWSM-011"}, Status: model.RunStatusRunning},
				{RunID: "run-1", Tickets: []string{"METAWSM-010"}, Status: model.RunStatusPaused},
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/runs?ticket=METAWSM-011", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Runs []serviceapi.RunSnapshot `json:"runs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal runs: %v", err)
	}
	if len(payload.Runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(payload.Runs))
	}
}

func TestHandleForumOpenThread(t *testing.T) {
	core := &mockCore{
		forumOpenThreadFn: func(_ context.Context, options serviceapi.ForumOpenThreadOptions) (model.ForumThreadView, error) {
			if options.Ticket != "METAWSM-011" {
				t.Fatalf("unexpected ticket %q", options.Ticket)
			}
			return model.ForumThreadView{
				ThreadID:   "fthr-1",
				Ticket:     "METAWSM-011",
				Title:      options.Title,
				State:      model.ForumThreadStateNew,
				Priority:   model.ForumPriorityNormal,
				PostsCount: 1,
				UpdatedAt:  time.Now().UTC(),
				OpenedAt:   time.Now().UTC(),
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	body := `{"ticket":"METAWSM-011","title":"Need decision","body":"Should we proceed?","priority":"normal","actor_type":"agent","actor_name":"agent-a"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal thread: %v", err)
	}
	if payload.Thread.ThreadID != "fthr-1" {
		t.Fatalf("unexpected thread id %q", payload.Thread.ThreadID)
	}
}

func TestHandleForumEvents(t *testing.T) {
	core := &mockCore{
		forumWatchEventsFn: func(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
			if ticket != "METAWSM-011" {
				t.Fatalf("unexpected ticket %q", ticket)
			}
			if cursor != 2 {
				t.Fatalf("unexpected cursor %d", cursor)
			}
			if limit != 10 {
				t.Fatalf("unexpected limit %d", limit)
			}
			return []model.ForumEvent{
				{
					Sequence: 3,
					Envelope: model.ForumEnvelope{
						EventID:      "evt-3",
						EventType:    "forum.post.added",
						EventVersion: 1,
						ThreadID:     "fthr-1",
						Ticket:       "METAWSM-011",
						OccurredAt:   time.Now().UTC(),
					},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/forum/events?ticket=METAWSM-011&cursor=2&limit=10", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		NextCursor int64              `json:"next_cursor"`
		Events     []model.ForumEvent `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal forum events: %v", err)
	}
	if payload.NextCursor != 3 {
		t.Fatalf("expected next cursor 3, got %d", payload.NextCursor)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(payload.Events))
	}
}

func TestHandleForumDebug(t *testing.T) {
	core := &mockCore{
		forumStreamDebugSnapshotFn: func(_ context.Context, options serviceapi.ForumDebugOptions) (model.ForumStreamDebugSnapshot, error) {
			if options.Ticket != "METAWSM-011" {
				t.Fatalf("unexpected ticket %q", options.Ticket)
			}
			if options.RunID != "run-123" {
				t.Fatalf("unexpected run id %q", options.RunID)
			}
			if options.Limit != 15 {
				t.Fatalf("unexpected limit %d", options.Limit)
			}
			return model.ForumStreamDebugSnapshot{
				GeneratedAt: time.Now().UTC(),
				Ticket:      options.Ticket,
				RunID:       options.RunID,
				Outbox: model.ForumOutboxStats{
					PendingCount:    3,
					ProcessingCount: 1,
					FailedCount:     2,
				},
				Bus: model.ForumBusDebug{
					Running:       true,
					Healthy:       true,
					StreamName:    "metawsm-forum.abc",
					ConsumerGroup: "metawsm-forum.abc",
					ConsumerName:  "operator-abc",
				},
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/forum/debug?ticket=METAWSM-011&run_id=run-123&limit=15", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Debug model.ForumStreamDebugSnapshot `json:"debug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal forum debug: %v", err)
	}
	if payload.Debug.Ticket != "METAWSM-011" {
		t.Fatalf("unexpected payload ticket %q", payload.Debug.Ticket)
	}
	if payload.Debug.Bus.StreamName != "metawsm-forum.abc" {
		t.Fatalf("unexpected stream name %q", payload.Debug.Bus.StreamName)
	}
}

func TestHandleForumSearch(t *testing.T) {
	core := &mockCore{
		forumSearchThreadsFn: func(options serviceapi.ForumSearchThreadsOptions) ([]model.ForumThreadView, error) {
			if options.Query != "validation mismatch" {
				t.Fatalf("unexpected query %q", options.Query)
			}
			if options.Ticket != "METAWSM-011" {
				t.Fatalf("unexpected ticket %q", options.Ticket)
			}
			if options.RunID != "run-123" {
				t.Fatalf("unexpected run_id %q", options.RunID)
			}
			if options.State != model.ForumThreadStateWaitingHuman {
				t.Fatalf("unexpected state %q", options.State)
			}
			if options.Priority != model.ForumPriorityHigh {
				t.Fatalf("unexpected priority %q", options.Priority)
			}
			if options.Assignee != "kball" {
				t.Fatalf("unexpected assignee %q", options.Assignee)
			}
			if options.ViewerType != model.ForumViewerHuman {
				t.Fatalf("unexpected viewer type %q", options.ViewerType)
			}
			if options.ViewerID != "human:kball" {
				t.Fatalf("unexpected viewer id %q", options.ViewerID)
			}
			if options.Limit != 15 {
				t.Fatalf("unexpected limit %d", options.Limit)
			}
			if options.Cursor != 22 {
				t.Fatalf("unexpected cursor %d", options.Cursor)
			}
			return []model.ForumThreadView{{
				ThreadID:     "fthr-search-1",
				Ticket:       "METAWSM-011",
				Title:        "Validation mismatch in parser",
				State:        model.ForumThreadStateWaitingHuman,
				Priority:     model.ForumPriorityHigh,
				IsUnseen:     true,
				IsUnanswered: true,
				OpenedAt:     time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			}}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/forum/search?query=validation+mismatch&ticket=METAWSM-011&run_id=run-123&state=waiting_human&priority=high&assignee=kball&viewer_type=human&viewer_id=human:kball&limit=15&cursor=22", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Threads []model.ForumThreadView `json:"threads"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal search response: %v", err)
	}
	if len(payload.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(payload.Threads))
	}
	if !payload.Threads[0].IsUnseen {
		t.Fatalf("expected unseen badge in response")
	}
}

func TestHandleForumQueues(t *testing.T) {
	core := &mockCore{
		forumListQueueFn: func(options serviceapi.ForumQueueOptions) ([]model.ForumThreadView, error) {
			if options.QueueType != model.ForumQueueUnseen {
				t.Fatalf("unexpected queue type %q", options.QueueType)
			}
			if options.ViewerType != model.ForumViewerHuman {
				t.Fatalf("unexpected viewer type %q", options.ViewerType)
			}
			if options.ViewerID != "human:kball" {
				t.Fatalf("unexpected viewer id %q", options.ViewerID)
			}
			if options.Limit != 25 {
				t.Fatalf("unexpected limit %d", options.Limit)
			}
			return []model.ForumThreadView{{
				ThreadID:     "fthr-queue-1",
				Ticket:       "METAWSM-011",
				Title:        "Queue item",
				State:        model.ForumThreadStateWaitingHuman,
				Priority:     model.ForumPriorityNormal,
				IsUnseen:     true,
				IsUnanswered: true,
				OpenedAt:     time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			}}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/forum/queues?type=unseen&ticket=METAWSM-011&viewer_type=human&viewer_id=human:kball&limit=25", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Threads []model.ForumThreadView `json:"threads"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal queue response: %v", err)
	}
	if len(payload.Threads) != 1 {
		t.Fatalf("expected 1 queue thread, got %d", len(payload.Threads))
	}
}

func TestHandleForumMarkSeen(t *testing.T) {
	core := &mockCore{
		forumMarkThreadSeenFn: func(_ context.Context, options serviceapi.ForumMarkThreadSeenOptions) (model.ForumThreadSeen, error) {
			if options.ThreadID != "fthr-1" {
				t.Fatalf("unexpected thread id %q", options.ThreadID)
			}
			if options.ViewerType != model.ForumViewerHuman {
				t.Fatalf("unexpected viewer type %q", options.ViewerType)
			}
			if options.ViewerID != "human:kball" {
				t.Fatalf("unexpected viewer id %q", options.ViewerID)
			}
			if options.LastSeenEventSequence != 12 {
				t.Fatalf("unexpected seen sequence %d", options.LastSeenEventSequence)
			}
			return model.ForumThreadSeen{
				ThreadID:              options.ThreadID,
				ViewerType:            options.ViewerType,
				ViewerID:              options.ViewerID,
				LastSeenEventSequence: options.LastSeenEventSequence,
				UpdatedAt:             time.Now().UTC(),
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads/fthr-1/seen", strings.NewReader(`{"viewer_type":"human","viewer_id":"human:kball","last_seen_event_sequence":12}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Seen model.ForumThreadSeen `json:"seen"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal seen response: %v", err)
	}
	if payload.Seen.ThreadID != "fthr-1" {
		t.Fatalf("unexpected seen thread id %q", payload.Seen.ThreadID)
	}
}

func TestHandleForumStreamRejectsInvalidUpgrade(t *testing.T) {
	core := &mockCore{}
	runtime := newTestRuntime(core)
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/forum/stream?ticket=METAWSM-011", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestHandleForumStreamSendsCatchUpFrame(t *testing.T) {
	core := &mockCore{
		forumWatchEventsFn: func(_ string, _ int64, _ int) ([]model.ForumEvent, error) {
			return []model.ForumEvent{
				{
					Sequence: 7,
					Envelope: model.ForumEnvelope{
						EventID:      "evt-stream-1",
						EventType:    "forum.post.added",
						EventVersion: 1,
						ThreadID:     "fthr-stream-1",
						Ticket:       "METAWSM-011",
						OccurredAt:   time.Now().UTC(),
					},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(core)
	conn, reader := openTestWebSocket(t, runtime, "/api/v1/forum/stream?ticket=METAWSM-011")
	defer conn.Close()

	frame := readWebSocketJSONFrame(t, conn, reader, 500*time.Millisecond)
	if frame["type"] != "forum.events" {
		t.Fatalf("expected forum.events frame, got %#v", frame["type"])
	}
	if frame["next_cursor"] == nil {
		t.Fatalf("expected next_cursor in frame payload")
	}
}

func TestHandleForumStreamSendsHeartbeatWhenIdle(t *testing.T) {
	calls := 0
	core := &mockCore{
		forumWatchEventsFn: func(_ string, _ int64, _ int) ([]model.ForumEvent, error) {
			calls++
			return nil, nil
		},
	}
	runtime := newTestRuntime(core)
	conn, reader := openTestWebSocket(t, runtime, "/api/v1/forum/stream?ticket=METAWSM-011")
	defer conn.Close()

	frame := readWebSocketJSONFrame(t, conn, reader, 500*time.Millisecond)
	if frame["type"] != "heartbeat" {
		t.Fatalf("expected heartbeat frame, got %#v", frame["type"])
	}
	if calls != 1 {
		t.Fatalf("expected exactly one catch-up watch call, got %d", calls)
	}
}

func TestHandleForumStreamSendsLiveBrokerEventFrame(t *testing.T) {
	calls := 0
	core := &mockCore{
		forumWatchEventsFn: func(_ string, _ int64, _ int) ([]model.ForumEvent, error) {
			calls++
			return nil, nil
		},
	}
	runtime := newTestRuntime(core)
	conn, reader := openTestWebSocket(t, runtime, "/api/v1/forum/stream?ticket=METAWSM-011")
	defer conn.Close()

	go func() {
		time.Sleep(25 * time.Millisecond)
		runtime.eventBroker.Publish(model.ForumEvent{
			Sequence: 9,
			Envelope: model.ForumEnvelope{
				EventID:      "evt-live-9",
				EventType:    "forum.post.added",
				EventVersion: 1,
				ThreadID:     "fthr-live-1",
				Ticket:       "METAWSM-011",
				OccurredAt:   time.Now().UTC(),
			},
		})
	}()

	frame := readWebSocketJSONFrame(t, conn, reader, 500*time.Millisecond)
	if frame["type"] != "forum.events" {
		t.Fatalf("expected forum.events frame, got %#v", frame["type"])
	}
	if calls != 1 {
		t.Fatalf("expected exactly one catch-up watch call, got %d", calls)
	}
	events, ok := frame["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("expected non-empty events payload, got %#v", frame["events"])
	}
}

func readWebSocketFrame(reader *bufio.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	payloadLen := int(header[1] & 0x7f)
	switch payloadLen {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		payloadLen = int(extended[0])<<8 | int(extended[1])
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		payloadLen = int(extended[7])
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func openTestWebSocket(t *testing.T, runtime *Runtime, path string) (net.Conn, *bufio.Reader) {
	t.Helper()
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	request := strings.Join([]string{
		"GET " + path + " HTTP/1.1",
		"Host: " + parsed.Host,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Version: 13",
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
		"",
		"",
	}, "\r\n")
	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		t.Fatalf("write handshake request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read status line: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		t.Fatalf("expected websocket upgrade status, got %q", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read header line: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}
	return conn, reader
}

func readWebSocketJSONFrame(t *testing.T, conn net.Conn, reader *bufio.Reader, timeout time.Duration) map[string]any {
	t.Helper()
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	payload, err := readWebSocketFrame(reader)
	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	var frame map[string]any
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("unmarshal websocket frame: %v payload=%s", err, string(payload))
	}
	return frame
}

func newTestRuntime(core serviceapi.Core) *Runtime {
	return &Runtime{
		service:     core,
		worker:      NewForumWorker(core, time.Second, 10, time.Minute, nil),
		startedAt:   time.Now().UTC(),
		eventBroker: NewForumEventBroker(32),
		streamBeat:  50 * time.Millisecond,
	}
}

type mockCore struct {
	listRunSnapshotsFn func(context.Context, string) ([]serviceapi.RunSnapshot, error)
	runSnapshotFn      func(context.Context, string) (serviceapi.RunSnapshot, error)

	forumOpenThreadFn          func(context.Context, serviceapi.ForumOpenThreadOptions) (model.ForumThreadView, error)
	forumAddPostFn             func(context.Context, serviceapi.ForumAddPostOptions) (model.ForumThreadView, error)
	forumAnswerThreadFn        func(context.Context, serviceapi.ForumAddPostOptions) (model.ForumThreadView, error)
	forumAssignThreadFn        func(context.Context, serviceapi.ForumAssignThreadOptions) (model.ForumThreadView, error)
	forumChangeStateFn         func(context.Context, serviceapi.ForumChangeStateOptions) (model.ForumThreadView, error)
	forumSetPriorityFn         func(context.Context, serviceapi.ForumSetPriorityOptions) (model.ForumThreadView, error)
	forumCloseThreadFn         func(context.Context, serviceapi.ForumChangeStateOptions) (model.ForumThreadView, error)
	forumControlSignalFn       func(context.Context, serviceapi.ForumControlSignalOptions) (model.ForumThreadView, error)
	forumStreamDebugSnapshotFn func(context.Context, serviceapi.ForumDebugOptions) (model.ForumStreamDebugSnapshot, error)
	forumListThreadsFn         func(model.ForumThreadFilter) ([]model.ForumThreadView, error)
	forumGetThreadFn           func(string) (*serviceapi.ForumThreadDetail, error)
	forumListStatsFn           func(string, string) ([]model.ForumThreadStats, error)
	forumWatchEventsFn         func(string, int64, int) ([]model.ForumEvent, error)
	forumSearchThreadsFn       func(serviceapi.ForumSearchThreadsOptions) ([]model.ForumThreadView, error)
	forumListQueueFn           func(serviceapi.ForumQueueOptions) ([]model.ForumThreadView, error)
	forumMarkThreadSeenFn      func(context.Context, serviceapi.ForumMarkThreadSeenOptions) (model.ForumThreadSeen, error)
}

func (m *mockCore) Shutdown() {}

func (m *mockCore) ProcessForumBusOnce(_ context.Context, _ int) (int, error) { return 0, nil }
func (m *mockCore) ForumBusHealth() error                                     { return nil }
func (m *mockCore) ForumOutboxStats() (model.ForumOutboxStats, error) {
	return model.ForumOutboxStats{}, nil
}
func (m *mockCore) ForumStreamDebugSnapshot(ctx context.Context, options serviceapi.ForumDebugOptions) (model.ForumStreamDebugSnapshot, error) {
	if m.forumStreamDebugSnapshotFn == nil {
		return model.ForumStreamDebugSnapshot{}, nil
	}
	return m.forumStreamDebugSnapshotFn(ctx, options)
}
func (m *mockCore) RunSnapshot(ctx context.Context, runID string) (serviceapi.RunSnapshot, error) {
	if m.runSnapshotFn == nil {
		return serviceapi.RunSnapshot{}, fmt.Errorf("run snapshot not implemented")
	}
	return m.runSnapshotFn(ctx, runID)
}
func (m *mockCore) ListRunSnapshots(ctx context.Context, ticket string) ([]serviceapi.RunSnapshot, error) {
	if m.listRunSnapshotsFn == nil {
		return []serviceapi.RunSnapshot{}, nil
	}
	return m.listRunSnapshotsFn(ctx, ticket)
}
func (m *mockCore) ForumOpenThread(ctx context.Context, options serviceapi.ForumOpenThreadOptions) (model.ForumThreadView, error) {
	if m.forumOpenThreadFn == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum open not implemented")
	}
	return m.forumOpenThreadFn(ctx, options)
}
func (m *mockCore) ForumAddPost(ctx context.Context, options serviceapi.ForumAddPostOptions) (model.ForumThreadView, error) {
	if m.forumAddPostFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumAddPostFn(ctx, options)
}
func (m *mockCore) ForumAnswerThread(ctx context.Context, options serviceapi.ForumAddPostOptions) (model.ForumThreadView, error) {
	if m.forumAnswerThreadFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumAnswerThreadFn(ctx, options)
}
func (m *mockCore) ForumAssignThread(ctx context.Context, options serviceapi.ForumAssignThreadOptions) (model.ForumThreadView, error) {
	if m.forumAssignThreadFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumAssignThreadFn(ctx, options)
}
func (m *mockCore) ForumChangeState(ctx context.Context, options serviceapi.ForumChangeStateOptions) (model.ForumThreadView, error) {
	if m.forumChangeStateFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumChangeStateFn(ctx, options)
}
func (m *mockCore) ForumSetPriority(ctx context.Context, options serviceapi.ForumSetPriorityOptions) (model.ForumThreadView, error) {
	if m.forumSetPriorityFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumSetPriorityFn(ctx, options)
}
func (m *mockCore) ForumCloseThread(ctx context.Context, options serviceapi.ForumChangeStateOptions) (model.ForumThreadView, error) {
	if m.forumCloseThreadFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumCloseThreadFn(ctx, options)
}
func (m *mockCore) ForumAppendControlSignal(ctx context.Context, options serviceapi.ForumControlSignalOptions) (model.ForumThreadView, error) {
	if m.forumControlSignalFn == nil {
		return model.ForumThreadView{}, nil
	}
	return m.forumControlSignalFn(ctx, options)
}
func (m *mockCore) ForumListThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error) {
	if m.forumListThreadsFn == nil {
		return []model.ForumThreadView{}, nil
	}
	return m.forumListThreadsFn(filter)
}
func (m *mockCore) ForumGetThread(threadID string) (*serviceapi.ForumThreadDetail, error) {
	if m.forumGetThreadFn == nil {
		return nil, nil
	}
	return m.forumGetThreadFn(threadID)
}
func (m *mockCore) ForumListStats(ticket string, runID string) ([]model.ForumThreadStats, error) {
	if m.forumListStatsFn == nil {
		return []model.ForumThreadStats{}, nil
	}
	return m.forumListStatsFn(ticket, runID)
}
func (m *mockCore) ForumWatchEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
	if m.forumWatchEventsFn == nil {
		return []model.ForumEvent{}, nil
	}
	return m.forumWatchEventsFn(ticket, cursor, limit)
}

func (m *mockCore) ForumSearchThreads(options serviceapi.ForumSearchThreadsOptions) ([]model.ForumThreadView, error) {
	if m.forumSearchThreadsFn == nil {
		return []model.ForumThreadView{}, nil
	}
	return m.forumSearchThreadsFn(options)
}

func (m *mockCore) ForumListQueue(options serviceapi.ForumQueueOptions) ([]model.ForumThreadView, error) {
	if m.forumListQueueFn == nil {
		return []model.ForumThreadView{}, nil
	}
	return m.forumListQueueFn(options)
}

func (m *mockCore) ForumMarkThreadSeen(ctx context.Context, options serviceapi.ForumMarkThreadSeenOptions) (model.ForumThreadSeen, error) {
	if m.forumMarkThreadSeenFn == nil {
		return model.ForumThreadSeen{}, nil
	}
	return m.forumMarkThreadSeenFn(ctx, options)
}
