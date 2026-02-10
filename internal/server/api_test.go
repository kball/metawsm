package server

import (
	"bufio"
	"bytes"
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

func TestHandleForumStreamSendsFrame(t *testing.T) {
	calls := 0
	core := &mockCore{
		forumWatchEventsFn: func(_ string, _ int64, _ int) ([]model.ForumEvent, error) {
			calls++
			return []model.ForumEvent{
				{
					Sequence: int64(calls),
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
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer conn.Close()

	request := strings.Join([]string{
		"GET /api/v1/forum/stream?ticket=METAWSM-011&poll_ms=50 HTTP/1.1",
		"Host: " + parsed.Host,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Version: 13",
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
		"",
		"",
	}, "\r\n")
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write handshake request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("expected websocket upgrade status, got %q", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read header line: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	framePayload, err := readWebSocketFrame(reader)
	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	if !bytes.Contains(framePayload, []byte(`"type":"forum.events"`)) && !bytes.Contains(framePayload, []byte(`"type":"heartbeat"`)) {
		t.Fatalf("unexpected websocket payload: %s", string(framePayload))
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

func newTestRuntime(core serviceapi.Core) *Runtime {
	return &Runtime{
		service:   core,
		worker:    NewForumWorker(core, time.Second, 10, time.Minute, nil),
		startedAt: time.Now().UTC(),
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
