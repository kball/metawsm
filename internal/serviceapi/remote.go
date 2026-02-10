package serviceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"metawsm/internal/model"
)

type RemoteCore struct {
	baseURL string
	client  *http.Client
}

func NewRemoteCore(baseURL string, timeout time.Duration) *RemoteCore {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &RemoteCore{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (r *RemoteCore) Shutdown() {}

func (r *RemoteCore) ProcessForumBusOnce(_ context.Context, _ int) (int, error) {
	return 0, fmt.Errorf("remote core does not support ProcessForumBusOnce")
}

func (r *RemoteCore) ForumBusHealth() error {
	var response struct {
		Status string `json:"status"`
	}
	if err := r.doJSON(context.Background(), http.MethodGet, "/api/v1/health", nil, nil, &response); err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(response.Status)) != "ok" {
		return fmt.Errorf("forum bus health is degraded")
	}
	return nil
}

func (r *RemoteCore) ForumOutboxStats() (model.ForumOutboxStats, error) {
	var response struct {
		Outbox model.ForumOutboxStats `json:"outbox"`
	}
	if err := r.doJSON(context.Background(), http.MethodGet, "/api/v1/health", nil, nil, &response); err != nil {
		return model.ForumOutboxStats{}, err
	}
	return response.Outbox, nil
}

func (r *RemoteCore) ForumStreamDebugSnapshot(ctx context.Context, options ForumDebugOptions) (model.ForumStreamDebugSnapshot, error) {
	query := map[string]string{}
	if ticket := strings.TrimSpace(options.Ticket); ticket != "" {
		query["ticket"] = ticket
	}
	if runID := strings.TrimSpace(options.RunID); runID != "" {
		query["run_id"] = runID
	}
	if options.Limit > 0 {
		query["limit"] = strconv.Itoa(options.Limit)
	}
	var response struct {
		Debug model.ForumStreamDebugSnapshot `json:"debug"`
	}
	if err := r.doJSON(ctx, http.MethodGet, "/api/v1/forum/debug", query, nil, &response); err != nil {
		return model.ForumStreamDebugSnapshot{}, err
	}
	return response.Debug, nil
}

func (r *RemoteCore) RunSnapshot(ctx context.Context, runID string) (RunSnapshot, error) {
	var response struct {
		Run RunSnapshot `json:"run"`
	}
	if err := r.doJSON(ctx, http.MethodGet, "/api/v1/runs/"+url.PathEscape(strings.TrimSpace(runID)), nil, nil, &response); err != nil {
		return RunSnapshot{}, err
	}
	return response.Run, nil
}

func (r *RemoteCore) ListRunSnapshots(ctx context.Context, ticket string) ([]RunSnapshot, error) {
	query := map[string]string{}
	if strings.TrimSpace(ticket) != "" {
		query["ticket"] = strings.TrimSpace(ticket)
	}
	var response struct {
		Runs []RunSnapshot `json:"runs"`
	}
	if err := r.doJSON(ctx, http.MethodGet, "/api/v1/runs", query, nil, &response); err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (r *RemoteCore) ForumOpenThread(ctx context.Context, options ForumOpenThreadOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"thread_id":      strings.TrimSpace(options.ThreadID),
		"ticket":         strings.TrimSpace(options.Ticket),
		"run_id":         strings.TrimSpace(options.RunID),
		"agent_name":     strings.TrimSpace(options.AgentName),
		"title":          strings.TrimSpace(options.Title),
		"body":           strings.TrimSpace(options.Body),
		"priority":       strings.TrimSpace(string(options.Priority)),
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	if err := r.doJSON(ctx, http.MethodPost, "/api/v1/forum/threads", nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumAddPost(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"body":           strings.TrimSpace(options.Body),
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(options.ThreadID)) + "/posts"
	if err := r.doJSON(ctx, http.MethodPost, path, nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumAnswerThread(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	return r.ForumAddPost(ctx, options)
}

func (r *RemoteCore) ForumAssignThread(ctx context.Context, options ForumAssignThreadOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"assignee_type":   strings.TrimSpace(string(options.AssigneeType)),
		"assignee_name":   strings.TrimSpace(options.AssigneeName),
		"assignment_note": strings.TrimSpace(options.AssignmentNote),
		"actor_type":      strings.TrimSpace(string(options.ActorType)),
		"actor_name":      strings.TrimSpace(options.ActorName),
		"correlation_id":  strings.TrimSpace(options.CorrelationID),
		"causation_id":    strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(options.ThreadID)) + "/assign"
	if err := r.doJSON(ctx, http.MethodPost, path, nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumChangeState(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"to_state":       strings.TrimSpace(string(options.ToState)),
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(options.ThreadID)) + "/state"
	if err := r.doJSON(ctx, http.MethodPost, path, nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumSetPriority(ctx context.Context, options ForumSetPriorityOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"priority":       strings.TrimSpace(string(options.Priority)),
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(options.ThreadID)) + "/priority"
	if err := r.doJSON(ctx, http.MethodPost, path, nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumCloseThread(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(options.ThreadID)) + "/close"
	if err := r.doJSON(ctx, http.MethodPost, path, nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumAppendControlSignal(ctx context.Context, options ForumControlSignalOptions) (model.ForumThreadView, error) {
	payload := map[string]any{
		"run_id":         strings.TrimSpace(options.RunID),
		"ticket":         strings.TrimSpace(options.Ticket),
		"agent_name":     strings.TrimSpace(options.AgentName),
		"actor_type":     strings.TrimSpace(string(options.ActorType)),
		"actor_name":     strings.TrimSpace(options.ActorName),
		"correlation_id": strings.TrimSpace(options.CorrelationID),
		"causation_id":   strings.TrimSpace(options.CausationID),
		"payload":        options.Payload,
	}
	var response struct {
		Thread model.ForumThreadView `json:"thread"`
	}
	if err := r.doJSON(ctx, http.MethodPost, "/api/v1/forum/control/signal", nil, payload, &response); err != nil {
		return model.ForumThreadView{}, err
	}
	return response.Thread, nil
}

func (r *RemoteCore) ForumListThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error) {
	query := map[string]string{}
	if strings.TrimSpace(filter.Ticket) != "" {
		query["ticket"] = strings.TrimSpace(filter.Ticket)
	}
	if strings.TrimSpace(filter.RunID) != "" {
		query["run_id"] = strings.TrimSpace(filter.RunID)
	}
	if strings.TrimSpace(string(filter.State)) != "" {
		query["state"] = strings.TrimSpace(string(filter.State))
	}
	if strings.TrimSpace(string(filter.Priority)) != "" {
		query["priority"] = strings.TrimSpace(string(filter.Priority))
	}
	if strings.TrimSpace(filter.Assignee) != "" {
		query["assignee"] = strings.TrimSpace(filter.Assignee)
	}
	if filter.Limit > 0 {
		query["limit"] = strconv.Itoa(filter.Limit)
	}
	var response struct {
		Threads []model.ForumThreadView `json:"threads"`
	}
	if err := r.doJSON(context.Background(), http.MethodGet, "/api/v1/forum/threads", query, nil, &response); err != nil {
		return nil, err
	}
	return response.Threads, nil
}

func (r *RemoteCore) ForumGetThread(threadID string) (*ForumThreadDetail, error) {
	var detail ForumThreadDetail
	path := "/api/v1/forum/threads/" + url.PathEscape(strings.TrimSpace(threadID))
	if err := r.doJSON(context.Background(), http.MethodGet, path, nil, nil, &detail); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not_found") {
			return nil, nil
		}
		return nil, err
	}
	return &detail, nil
}

func (r *RemoteCore) ForumListStats(ticket string, runID string) ([]model.ForumThreadStats, error) {
	query := map[string]string{}
	if strings.TrimSpace(ticket) != "" {
		query["ticket"] = strings.TrimSpace(ticket)
	}
	if strings.TrimSpace(runID) != "" {
		query["run_id"] = strings.TrimSpace(runID)
	}
	var response struct {
		Stats []model.ForumThreadStats `json:"stats"`
	}
	if err := r.doJSON(context.Background(), http.MethodGet, "/api/v1/forum/stats", query, nil, &response); err != nil {
		return nil, err
	}
	return response.Stats, nil
}

func (r *RemoteCore) ForumWatchEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
	query := map[string]string{
		"cursor": strconv.FormatInt(cursor, 10),
	}
	if strings.TrimSpace(ticket) != "" {
		query["ticket"] = strings.TrimSpace(ticket)
	}
	if limit > 0 {
		query["limit"] = strconv.Itoa(limit)
	}
	var response struct {
		Events []model.ForumEvent `json:"events"`
	}
	if err := r.doJSON(context.Background(), http.MethodGet, "/api/v1/forum/events", query, nil, &response); err != nil {
		return nil, err
	}
	return response.Events, nil
}

func (r *RemoteCore) doJSON(ctx context.Context, method string, path string, query map[string]string, body any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	fullURL := r.baseURL + path
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		values := parsed.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		parsed.RawQuery = values.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return decodeRemoteError(response.StatusCode, payload)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func decodeRemoteError(status int, payload []byte) error {
	var wrapper struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &wrapper); err == nil && strings.TrimSpace(wrapper.Error.Code) != "" {
		return fmt.Errorf("%s (http %d): %s", wrapper.Error.Code, status, strings.TrimSpace(wrapper.Error.Message))
	}
	return fmt.Errorf("http %d: %s", status, strings.TrimSpace(string(payload)))
}
