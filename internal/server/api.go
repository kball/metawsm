package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/serviceapi"
)

func (r *Runtime) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", r.handleHealth)
	mux.HandleFunc("/api/v1/runs", r.handleRuns)
	mux.HandleFunc("/api/v1/runs/", r.handleRunByID)
	mux.HandleFunc("/api/v1/forum/threads", r.handleForumThreads)
	mux.HandleFunc("/api/v1/forum/threads/", r.handleForumThreadAction)
	mux.HandleFunc("/api/v1/forum/control/signal", r.handleForumControlSignal)
	mux.HandleFunc("/api/v1/forum/events", r.handleForumEvents)
	mux.HandleFunc("/api/v1/forum/stats", r.handleForumStats)
	mux.HandleFunc("/api/v1/forum/debug", r.handleForumDebug)
	mux.HandleFunc("/api/v1/forum/stream", r.handleForumStream)
}

func (r *Runtime) handleRuns(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	ticket := strings.TrimSpace(req.URL.Query().Get("ticket"))
	snapshots, err := r.service.ListRunSnapshots(req.Context(), ticket)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_runs_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": snapshots})
}

func (r *Runtime) handleRunByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	runID := strings.TrimSpace(strings.TrimPrefix(req.URL.Path, "/api/v1/runs/"))
	if runID == "" || strings.Contains(runID, "/") {
		writeAPIError(w, http.StatusBadRequest, "invalid_run_id", "run id is required")
		return
	}
	snapshot, err := r.service.RunSnapshot(req.Context(), runID)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "run_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": snapshot})
}

func (r *Runtime) handleForumThreads(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		filter, err := parseForumThreadFilter(req)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_filter", err.Error())
			return
		}
		threads, err := r.service.ForumListThreads(filter)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "forum_list_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
	case http.MethodPost:
		var payload forumOpenThreadRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumOpenThread(req.Context(), serviceapi.ForumOpenThreadOptions{
			ThreadID:      strings.TrimSpace(payload.ThreadID),
			Ticket:        strings.TrimSpace(payload.Ticket),
			RunID:         strings.TrimSpace(payload.RunID),
			AgentName:     strings.TrimSpace(payload.AgentName),
			Title:         strings.TrimSpace(payload.Title),
			Body:          strings.TrimSpace(payload.Body),
			Priority:      model.ForumPriority(strings.TrimSpace(payload.Priority)),
			ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:     strings.TrimSpace(payload.ActorName),
			CorrelationID: strings.TrimSpace(payload.CorrelationID),
			CausationID:   strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_open_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
	}
}

func (r *Runtime) handleForumThreadAction(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/forum/threads/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_thread_path", "thread path is required")
		return
	}
	segments := strings.Split(path, "/")
	threadID := strings.TrimSpace(segments[0])
	if threadID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_thread_id", "thread id is required")
		return
	}
	if len(segments) == 1 {
		if req.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
			return
		}
		detail, err := r.service.ForumGetThread(threadID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "forum_thread_failed", err.Error())
			return
		}
		if detail == nil {
			writeAPIError(w, http.StatusNotFound, "forum_thread_not_found", "forum thread not found")
			return
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}
	if req.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}

	action := strings.TrimSpace(strings.ToLower(segments[1]))
	switch action {
	case "posts":
		var payload forumAddPostRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumAddPost(req.Context(), serviceapi.ForumAddPostOptions{
			ThreadID:      threadID,
			Body:          strings.TrimSpace(payload.Body),
			ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:     strings.TrimSpace(payload.ActorName),
			CorrelationID: strings.TrimSpace(payload.CorrelationID),
			CausationID:   strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_add_post_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	case "assign":
		var payload forumAssignThreadRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumAssignThread(req.Context(), serviceapi.ForumAssignThreadOptions{
			ThreadID:       threadID,
			AssigneeType:   model.ForumActorType(strings.TrimSpace(payload.AssigneeType)),
			AssigneeName:   strings.TrimSpace(payload.AssigneeName),
			AssignmentNote: strings.TrimSpace(payload.AssignmentNote),
			ActorType:      model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:      strings.TrimSpace(payload.ActorName),
			CorrelationID:  strings.TrimSpace(payload.CorrelationID),
			CausationID:    strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_assign_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	case "state":
		var payload forumChangeStateRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumChangeState(req.Context(), serviceapi.ForumChangeStateOptions{
			ThreadID:      threadID,
			ToState:       model.ForumThreadState(strings.TrimSpace(payload.ToState)),
			ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:     strings.TrimSpace(payload.ActorName),
			CorrelationID: strings.TrimSpace(payload.CorrelationID),
			CausationID:   strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_state_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	case "priority":
		var payload forumSetPriorityRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumSetPriority(req.Context(), serviceapi.ForumSetPriorityOptions{
			ThreadID:      threadID,
			Priority:      model.ForumPriority(strings.TrimSpace(payload.Priority)),
			ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:     strings.TrimSpace(payload.ActorName),
			CorrelationID: strings.TrimSpace(payload.CorrelationID),
			CausationID:   strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_priority_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	case "close":
		var payload forumCloseThreadRequest
		if err := decodeJSON(req, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		thread, err := r.service.ForumCloseThread(req.Context(), serviceapi.ForumChangeStateOptions{
			ThreadID:      threadID,
			ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
			ActorName:     strings.TrimSpace(payload.ActorName),
			CorrelationID: strings.TrimSpace(payload.CorrelationID),
			CausationID:   strings.TrimSpace(payload.CausationID),
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "forum_close_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
	default:
		writeAPIError(w, http.StatusNotFound, "unknown_action", "unsupported forum thread action")
	}
}

func (r *Runtime) handleForumControlSignal(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}
	var payload forumControlSignalRequest
	if err := decodeJSON(req, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	thread, err := r.service.ForumAppendControlSignal(req.Context(), serviceapi.ForumControlSignalOptions{
		RunID:         strings.TrimSpace(payload.RunID),
		Ticket:        strings.TrimSpace(payload.Ticket),
		AgentName:     strings.TrimSpace(payload.AgentName),
		ActorType:     model.ForumActorType(strings.TrimSpace(payload.ActorType)),
		ActorName:     strings.TrimSpace(payload.ActorName),
		CorrelationID: strings.TrimSpace(payload.CorrelationID),
		CausationID:   strings.TrimSpace(payload.CausationID),
		Payload:       payload.Payload,
	})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "forum_control_signal_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread})
}

func (r *Runtime) handleForumEvents(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	query := req.URL.Query()
	ticket := strings.TrimSpace(query.Get("ticket"))
	cursor, err := parseInt64Query(query.Get("cursor"), 0)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	limit, err := parseIntQuery(query.Get("limit"), 100)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	events, err := r.service.ForumWatchEvents(ticket, cursor, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "forum_watch_failed", err.Error())
		return
	}
	nextCursor := cursor
	if len(events) > 0 {
		nextCursor = events[len(events)-1].Sequence
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      events,
		"next_cursor": nextCursor,
	})
}

func (r *Runtime) handleForumStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	ticket := strings.TrimSpace(req.URL.Query().Get("ticket"))
	runID := strings.TrimSpace(req.URL.Query().Get("run_id"))
	stats, err := r.service.ForumListStats(ticket, runID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "forum_stats_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stats": stats,
	})
}

func (r *Runtime) handleForumDebug(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	query := req.URL.Query()
	limit, err := parseIntQuery(query.Get("limit"), 50)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	debugSnapshot, err := r.service.ForumStreamDebugSnapshot(req.Context(), serviceapi.ForumDebugOptions{
		Ticket: strings.TrimSpace(query.Get("ticket")),
		RunID:  strings.TrimSpace(query.Get("run_id")),
		Limit:  limit,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "forum_debug_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"debug": debugSnapshot,
	})
}

func parseForumThreadFilter(req *http.Request) (model.ForumThreadFilter, error) {
	query := req.URL.Query()
	limit, err := parseIntQuery(query.Get("limit"), 50)
	if err != nil {
		return model.ForumThreadFilter{}, err
	}
	return model.ForumThreadFilter{
		Ticket:   strings.TrimSpace(query.Get("ticket")),
		RunID:    strings.TrimSpace(query.Get("run_id")),
		State:    model.ForumThreadState(strings.TrimSpace(query.Get("state"))),
		Priority: model.ForumPriority(strings.TrimSpace(query.Get("priority"))),
		Assignee: strings.TrimSpace(query.Get("assignee")),
		Limit:    limit,
	}, nil
}

func parseIntQuery(raw string, fallback int) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	return n, nil
}

func parseInt64Query(raw string, fallback int64) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	return n, nil
}

func decodeJSON(req *http.Request, out any) error {
	if req.Body == nil {
		return fmt.Errorf("request body is required")
	}
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	return nil
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": apiError{
			Code:    strings.TrimSpace(code),
			Message: strings.TrimSpace(message),
		},
	})
}

type forumOpenThreadRequest struct {
	ThreadID      string `json:"thread_id"`
	Ticket        string `json:"ticket"`
	RunID         string `json:"run_id"`
	AgentName     string `json:"agent_name"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Priority      string `json:"priority"`
	ActorType     string `json:"actor_type"`
	ActorName     string `json:"actor_name"`
	CorrelationID string `json:"correlation_id"`
	CausationID   string `json:"causation_id"`
}

type forumAddPostRequest struct {
	Body          string `json:"body"`
	ActorType     string `json:"actor_type"`
	ActorName     string `json:"actor_name"`
	CorrelationID string `json:"correlation_id"`
	CausationID   string `json:"causation_id"`
}

type forumAssignThreadRequest struct {
	AssigneeType   string `json:"assignee_type"`
	AssigneeName   string `json:"assignee_name"`
	AssignmentNote string `json:"assignment_note"`
	ActorType      string `json:"actor_type"`
	ActorName      string `json:"actor_name"`
	CorrelationID  string `json:"correlation_id"`
	CausationID    string `json:"causation_id"`
}

type forumChangeStateRequest struct {
	ToState       string `json:"to_state"`
	ActorType     string `json:"actor_type"`
	ActorName     string `json:"actor_name"`
	CorrelationID string `json:"correlation_id"`
	CausationID   string `json:"causation_id"`
}

type forumSetPriorityRequest struct {
	Priority      string `json:"priority"`
	ActorType     string `json:"actor_type"`
	ActorName     string `json:"actor_name"`
	CorrelationID string `json:"correlation_id"`
	CausationID   string `json:"causation_id"`
}

type forumCloseThreadRequest struct {
	ActorType     string `json:"actor_type"`
	ActorName     string `json:"actor_name"`
	CorrelationID string `json:"correlation_id"`
	CausationID   string `json:"causation_id"`
}

type forumControlSignalRequest struct {
	RunID         string                      `json:"run_id"`
	Ticket        string                      `json:"ticket"`
	AgentName     string                      `json:"agent_name"`
	ActorType     string                      `json:"actor_type"`
	ActorName     string                      `json:"actor_name"`
	CorrelationID string                      `json:"correlation_id"`
	CausationID   string                      `json:"causation_id"`
	Payload       model.ForumControlPayloadV1 `json:"payload"`
}

func contextWithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
