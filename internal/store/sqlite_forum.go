package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"metawsm/internal/model"
)

func (s *SQLiteStore) ForumOpenThread(cmd model.ForumOpenThreadCommand) (*model.ForumThreadView, error) {
	if ok, err := s.forumEventExists(cmd.Envelope.EventID); err != nil {
		return nil, err
	} else if ok {
		return s.GetForumThread(cmd.Envelope.ThreadID)
	}

	now := cmd.Envelope.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	priority := cmd.Priority
	if priority == "" {
		priority = model.ForumPriorityNormal
	}
	payload, err := json.Marshal(map[string]any{
		"title":    cmd.Title,
		"body":     cmd.Body,
		"priority": priority,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal forum open payload: %w", err)
	}

	postID := cmd.Envelope.EventID + ".post"
	nowRFC3339 := now.Format(time.RFC3339)
	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
INSERT INTO forum_threads
  (thread_id, ticket, run_id, agent_name, title, state, priority, assignee_type, assignee_name, opened_by_type, opened_by_name, opened_at, updated_at, closed_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, '', '', %s, %s, %s, %s, '');
INSERT INTO forum_posts
  (post_id, thread_id, event_id, author_type, author_name, body_text, created_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s);
INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
INSERT INTO forum_thread_views
  (thread_id, ticket, run_id, agent_name, title, state, priority, assignee_type, assignee_name, opened_by_type, opened_by_name, posts_count, last_post_at, last_post_by_type, last_post_by_name, opened_at, updated_at, closed_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, '', '', %s, %s, 1, %s, %s, %s, %s, %s, '');
COMMIT;`,
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.Ticket),
		quote(cmd.Envelope.RunID),
		quote(cmd.Envelope.AgentName),
		quote(cmd.Title),
		quote(string(model.ForumThreadStateNew)),
		quote(string(priority)),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(nowRFC3339),
		quote(nowRFC3339),
		quote(postID),
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.EventID),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Body),
		quote(nowRFC3339),
		quote(cmd.Envelope.EventID),
		quote(cmd.Envelope.EventType),
		cmd.Envelope.EventVersion,
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.RunID),
		quote(cmd.Envelope.Ticket),
		quote(cmd.Envelope.AgentName),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Envelope.CorrelationID),
		quote(cmd.Envelope.CausationID),
		quote(string(payload)),
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.Ticket),
		quote(cmd.Envelope.RunID),
		quote(cmd.Envelope.AgentName),
		quote(cmd.Title),
		quote(string(model.ForumThreadStateNew)),
		quote(string(priority)),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(nowRFC3339),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(nowRFC3339),
		quote(nowRFC3339),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadStats(cmd.Envelope.Ticket); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadQueueView(cmd.Envelope.ThreadID); err != nil {
		return nil, err
	}
	return s.GetForumThread(cmd.Envelope.ThreadID)
}

func (s *SQLiteStore) ForumAddPost(cmd model.ForumAddPostCommand) (*model.ForumThreadView, error) {
	if ok, err := s.forumEventExists(cmd.Envelope.EventID); err != nil {
		return nil, err
	} else if ok {
		return s.GetForumThread(cmd.Envelope.ThreadID)
	}
	thread, err := s.GetForumThread(cmd.Envelope.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("forum thread %s not found", cmd.Envelope.ThreadID)
	}
	now := cmd.Envelope.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	payload, err := json.Marshal(map[string]any{"body": cmd.Body})
	if err != nil {
		return nil, fmt.Errorf("marshal forum post payload: %w", err)
	}

	postID := cmd.Envelope.EventID + ".post"
	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
INSERT INTO forum_posts
  (post_id, thread_id, event_id, author_type, author_name, body_text, created_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s);
INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
UPDATE forum_threads
SET updated_at=%s
WHERE thread_id=%s;
UPDATE forum_thread_views
SET posts_count=posts_count+1,
    last_post_at=%s,
    last_post_by_type=%s,
    last_post_by_name=%s,
    updated_at=%s
WHERE thread_id=%s;
COMMIT;`,
		quote(postID),
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.EventID),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Body),
		quote(nowRFC3339),
		quote(cmd.Envelope.EventID),
		quote(cmd.Envelope.EventType),
		cmd.Envelope.EventVersion,
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.RunID),
		quote(thread.Ticket),
		quote(thread.AgentName),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Envelope.CorrelationID),
		quote(cmd.Envelope.CausationID),
		quote(string(payload)),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(nowRFC3339),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadQueueView(cmd.Envelope.ThreadID); err != nil {
		return nil, err
	}
	return s.GetForumThread(cmd.Envelope.ThreadID)
}

func (s *SQLiteStore) ForumAssignThread(cmd model.ForumAssignThreadCommand) (*model.ForumThreadView, error) {
	if ok, err := s.forumEventExists(cmd.Envelope.EventID); err != nil {
		return nil, err
	} else if ok {
		return s.GetForumThread(cmd.Envelope.ThreadID)
	}
	thread, err := s.GetForumThread(cmd.Envelope.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("forum thread %s not found", cmd.Envelope.ThreadID)
	}
	now := cmd.Envelope.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	payload, err := json.Marshal(map[string]any{
		"from_assignee_type": thread.AssigneeType,
		"from_assignee_name": thread.AssigneeName,
		"to_assignee_type":   cmd.AssigneeType,
		"to_assignee_name":   cmd.AssigneeName,
		"note":               cmd.AssignmentNote,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal forum assign payload: %w", err)
	}

	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
INSERT INTO forum_assignments
  (thread_id, event_id, from_assignee_type, from_assignee_name, to_assignee_type, to_assignee_name, changed_by_type, changed_by_name, changed_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, %s, %s);
INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
UPDATE forum_threads
SET assignee_type=%s,
    assignee_name=%s,
    updated_at=%s
WHERE thread_id=%s;
UPDATE forum_thread_views
SET assignee_type=%s,
    assignee_name=%s,
    updated_at=%s
WHERE thread_id=%s;
COMMIT;`,
		quote(cmd.Envelope.ThreadID),
		quote(cmd.Envelope.EventID),
		quote(string(thread.AssigneeType)),
		quote(thread.AssigneeName),
		quote(string(cmd.AssigneeType)),
		quote(cmd.AssigneeName),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(nowRFC3339),
		quote(cmd.Envelope.EventID),
		quote(cmd.Envelope.EventType),
		cmd.Envelope.EventVersion,
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(thread.RunID),
		quote(thread.Ticket),
		quote(thread.AgentName),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Envelope.CorrelationID),
		quote(cmd.Envelope.CausationID),
		quote(string(payload)),
		quote(string(cmd.AssigneeType)),
		quote(cmd.AssigneeName),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(string(cmd.AssigneeType)),
		quote(cmd.AssigneeName),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadQueueView(cmd.Envelope.ThreadID); err != nil {
		return nil, err
	}
	return s.GetForumThread(cmd.Envelope.ThreadID)
}

func (s *SQLiteStore) ForumChangeState(cmd model.ForumChangeStateCommand) (*model.ForumThreadView, error) {
	return s.forumChangeState(cmd.Envelope, cmd.ToState, false)
}

func (s *SQLiteStore) ForumCloseThread(cmd model.ForumCloseThreadCommand) (*model.ForumThreadView, error) {
	return s.forumChangeState(cmd.Envelope, model.ForumThreadStateClosed, true)
}

func (s *SQLiteStore) forumChangeState(envelope model.ForumEnvelope, toState model.ForumThreadState, closeEvent bool) (*model.ForumThreadView, error) {
	if ok, err := s.forumEventExists(envelope.EventID); err != nil {
		return nil, err
	} else if ok {
		return s.GetForumThread(envelope.ThreadID)
	}
	thread, err := s.GetForumThread(envelope.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("forum thread %s not found", envelope.ThreadID)
	}
	now := envelope.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	closedAt := ""
	if toState == model.ForumThreadStateClosed {
		closedAt = nowRFC3339
	}
	eventType := envelope.EventType
	if strings.TrimSpace(eventType) == "" {
		eventType = "forum.state.changed"
	}
	if closeEvent {
		eventType = "forum.thread.closed"
	}
	payload, err := json.Marshal(map[string]any{
		"from_state": thread.State,
		"to_state":   toState,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal forum state payload: %w", err)
	}

	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
INSERT INTO forum_state_transitions
  (thread_id, event_id, from_state, to_state, changed_by_type, changed_by_name, changed_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s);
INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
UPDATE forum_threads
SET state=%s,
    updated_at=%s,
    closed_at=%s
WHERE thread_id=%s;
UPDATE forum_thread_views
SET state=%s,
    updated_at=%s,
    closed_at=%s
WHERE thread_id=%s;
COMMIT;`,
		quote(envelope.ThreadID),
		quote(envelope.EventID),
		quote(string(thread.State)),
		quote(string(toState)),
		quote(string(envelope.ActorType)),
		quote(envelope.ActorName),
		quote(nowRFC3339),
		quote(envelope.EventID),
		quote(eventType),
		envelope.EventVersion,
		quote(nowRFC3339),
		quote(envelope.ThreadID),
		quote(thread.RunID),
		quote(thread.Ticket),
		quote(thread.AgentName),
		quote(string(envelope.ActorType)),
		quote(envelope.ActorName),
		quote(envelope.CorrelationID),
		quote(envelope.CausationID),
		quote(string(payload)),
		quote(string(toState)),
		quote(nowRFC3339),
		quote(closedAt),
		quote(envelope.ThreadID),
		quote(string(toState)),
		quote(nowRFC3339),
		quote(closedAt),
		quote(envelope.ThreadID),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadStats(thread.Ticket); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadQueueView(envelope.ThreadID); err != nil {
		return nil, err
	}
	return s.GetForumThread(envelope.ThreadID)
}

func (s *SQLiteStore) ForumSetPriority(cmd model.ForumSetPriorityCommand) (*model.ForumThreadView, error) {
	if ok, err := s.forumEventExists(cmd.Envelope.EventID); err != nil {
		return nil, err
	} else if ok {
		return s.GetForumThread(cmd.Envelope.ThreadID)
	}
	thread, err := s.GetForumThread(cmd.Envelope.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("forum thread %s not found", cmd.Envelope.ThreadID)
	}
	now := cmd.Envelope.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	nowRFC3339 := now.Format(time.RFC3339)
	payload, err := json.Marshal(map[string]any{
		"from_priority": thread.Priority,
		"to_priority":   cmd.Priority,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal forum priority payload: %w", err)
	}

	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
UPDATE forum_threads
SET priority=%s,
    updated_at=%s
WHERE thread_id=%s;
UPDATE forum_thread_views
SET priority=%s,
    updated_at=%s
WHERE thread_id=%s;
COMMIT;`,
		quote(cmd.Envelope.EventID),
		quote(cmd.Envelope.EventType),
		cmd.Envelope.EventVersion,
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(thread.RunID),
		quote(thread.Ticket),
		quote(thread.AgentName),
		quote(string(cmd.Envelope.ActorType)),
		quote(cmd.Envelope.ActorName),
		quote(cmd.Envelope.CorrelationID),
		quote(cmd.Envelope.CausationID),
		quote(string(payload)),
		quote(string(cmd.Priority)),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
		quote(string(cmd.Priority)),
		quote(nowRFC3339),
		quote(cmd.Envelope.ThreadID),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadStats(thread.Ticket); err != nil {
		return nil, err
	}
	if err := s.refreshForumThreadQueueView(cmd.Envelope.ThreadID); err != nil {
		return nil, err
	}
	return s.GetForumThread(cmd.Envelope.ThreadID)
}

func (s *SQLiteStore) GetForumThread(threadID string) (*model.ForumThreadView, error) {
	sql := fmt.Sprintf(
		`SELECT
  v.thread_id,
  v.ticket,
  v.run_id,
  v.agent_name,
  v.title,
  v.state,
  v.priority,
  v.assignee_type,
  v.assignee_name,
  v.opened_by_type,
  v.opened_by_name,
  v.posts_count,
  v.last_post_at,
  v.last_post_by_type,
  v.last_post_by_name,
  v.opened_at,
  v.updated_at,
  v.closed_at,
  COALESCE(q.last_event_sequence, 0) AS last_event_sequence,
  COALESCE(q.last_non_system_actor_type, '') AS last_actor_type
FROM forum_thread_views v
LEFT JOIN forum_thread_queue_view q ON q.thread_id = v.thread_id
WHERE v.thread_id=%s;`,
		quote(threadID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	view, err := parseForumThreadView(rows[0])
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *SQLiteStore) ListForumThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error) {
	clauses := []string{"1=1"}
	if v := strings.TrimSpace(filter.Ticket); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.ticket=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.RunID); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.run_id=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.State)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.state=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.Priority)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.priority=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.Assignee); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.assignee_name=%s", quote(v)))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	sql := fmt.Sprintf(
		`SELECT
  v.thread_id,
  v.ticket,
  v.run_id,
  v.agent_name,
  v.title,
  v.state,
  v.priority,
  v.assignee_type,
  v.assignee_name,
  v.opened_by_type,
  v.opened_by_name,
  v.posts_count,
  v.last_post_at,
  v.last_post_by_type,
  v.last_post_by_name,
  v.opened_at,
  v.updated_at,
  v.closed_at,
  COALESCE(q.last_event_sequence, 0) AS last_event_sequence,
  COALESCE(q.last_non_system_actor_type, '') AS last_actor_type
FROM forum_thread_views v
LEFT JOIN forum_thread_queue_view q ON q.thread_id = v.thread_id
WHERE %s
ORDER BY
  CASE v.priority
    WHEN 'urgent' THEN 1
    WHEN 'high' THEN 2
    WHEN 'normal' THEN 3
    WHEN 'low' THEN 4
    ELSE 5
  END,
  COALESCE(q.last_event_sequence, 0) DESC,
  v.updated_at DESC
LIMIT %d;`,
		strings.Join(clauses, " AND "),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumThreadView, 0, len(rows))
	for _, row := range rows {
		view, err := parseForumThreadView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *SQLiteStore) SearchForumThreads(filter model.ForumThreadSearchFilter) ([]model.ForumThreadView, error) {
	clauses := []string{"1=1"}
	if v := strings.TrimSpace(filter.Ticket); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.ticket=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.RunID); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.run_id=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.State)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.state=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.Priority)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.priority=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.Assignee); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.assignee_name=%s", quote(v)))
	}
	if filter.Cursor > 0 {
		clauses = append(clauses, fmt.Sprintf("COALESCE(q.last_event_sequence, 0) < %d", filter.Cursor))
	}

	viewerType := strings.TrimSpace(string(filter.ViewerType))
	viewerID := strings.TrimSpace(filter.ViewerID)
	viewerJoin := "LEFT JOIN forum_thread_reads r ON 1=0"
	if viewerType != "" || viewerID != "" {
		normalizedViewerType, err := normalizeForumViewerType(filter.ViewerType)
		if err != nil {
			return nil, err
		}
		if viewerID == "" {
			return nil, fmt.Errorf("forum viewer_id is required")
		}
		viewerJoin = fmt.Sprintf(
			"LEFT JOIN forum_thread_reads r ON r.thread_id = v.thread_id AND r.viewer_type = %s AND r.viewer_id = %s",
			quote(string(normalizedViewerType)),
			quote(viewerID),
		)
	}

	query := strings.ToLower(strings.TrimSpace(filter.Query))
	relevanceOrder := ""
	if query != "" {
		likePattern := "%" + query + "%"
		queryPredicate := fmt.Sprintf(
			"(LOWER(v.title) LIKE %s OR EXISTS (SELECT 1 FROM forum_posts p WHERE p.thread_id = v.thread_id AND LOWER(p.body_text) LIKE %s))",
			quote(likePattern),
			quote(likePattern),
		)
		relevanceOrder = fmt.Sprintf(
			"(CASE WHEN LOWER(v.title) LIKE %s THEN 2 ELSE 0 END + CASE WHEN EXISTS (SELECT 1 FROM forum_posts p WHERE p.thread_id = v.thread_id AND LOWER(p.body_text) LIKE %s) THEN 1 ELSE 0 END) DESC,",
			quote(likePattern),
			quote(likePattern),
		)
		clauses = append(clauses, queryPredicate)
	}

	unansweredExpr := "CASE WHEN v.state IN ('new', 'waiting_human', 'waiting_operator') AND COALESCE(q.last_agent_sequence, 0) > COALESCE(q.last_human_or_operator_sequence, 0) AND COALESCE(q.last_non_system_actor_type, '') = 'agent' THEN 1 ELSE 0 END"
	seenExpr := "CASE WHEN COALESCE(q.last_event_sequence, 0) > COALESCE(r.last_seen_event_sequence, 0) THEN 1 ELSE 0 END"
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	sql := fmt.Sprintf(
		`SELECT
  v.thread_id,
  v.ticket,
  v.run_id,
  v.agent_name,
  v.title,
  v.state,
  v.priority,
  v.assignee_type,
  v.assignee_name,
  v.opened_by_type,
  v.opened_by_name,
  v.posts_count,
  v.last_post_at,
  v.last_post_by_type,
  v.last_post_by_name,
  v.opened_at,
  v.updated_at,
  v.closed_at,
  COALESCE(q.last_event_sequence, 0) AS last_event_sequence,
  COALESCE(q.last_non_system_actor_type, '') AS last_actor_type,
  %s AS is_unseen,
  %s AS is_unanswered
FROM forum_thread_views v
LEFT JOIN forum_thread_queue_view q ON q.thread_id = v.thread_id
%s
WHERE %s
ORDER BY
  %s
  CASE v.priority
    WHEN 'urgent' THEN 1
    WHEN 'high' THEN 2
    WHEN 'normal' THEN 3
    WHEN 'low' THEN 4
    ELSE 5
  END,
  COALESCE(q.last_event_sequence, 0) DESC,
  v.updated_at DESC
LIMIT %d;`,
		seenExpr,
		unansweredExpr,
		viewerJoin,
		strings.Join(clauses, " AND "),
		relevanceOrder,
		limit,
	)

	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumThreadView, 0, len(rows))
	for _, row := range rows {
		view, err := parseForumThreadView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *SQLiteStore) ListForumQueue(filter model.ForumQueueFilter) ([]model.ForumThreadView, error) {
	queueType, err := normalizeForumQueueType(filter.QueueType)
	if err != nil {
		return nil, err
	}

	clauses := []string{"1=1"}
	if v := strings.TrimSpace(filter.Ticket); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.ticket=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.RunID); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.run_id=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.State)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.state=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.Priority)); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.priority=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.Assignee); v != "" {
		clauses = append(clauses, fmt.Sprintf("v.assignee_name=%s", quote(v)))
	}
	if filter.Cursor > 0 {
		clauses = append(clauses, fmt.Sprintf("COALESCE(q.last_event_sequence, 0) < %d", filter.Cursor))
	}

	viewerType := strings.TrimSpace(string(filter.ViewerType))
	viewerID := strings.TrimSpace(filter.ViewerID)
	viewerJoin := "LEFT JOIN forum_thread_reads r ON 1=0"
	if queueType == model.ForumQueueUnseen || viewerType != "" || viewerID != "" {
		normalizedViewerType, err := normalizeForumViewerType(filter.ViewerType)
		if err != nil {
			return nil, err
		}
		if viewerID == "" {
			return nil, fmt.Errorf("forum viewer_id is required")
		}
		viewerJoin = fmt.Sprintf(
			"LEFT JOIN forum_thread_reads r ON r.thread_id = v.thread_id AND r.viewer_type = %s AND r.viewer_id = %s",
			quote(string(normalizedViewerType)),
			quote(viewerID),
		)
	}

	unansweredExpr := "CASE WHEN v.state IN ('new', 'waiting_human', 'waiting_operator') AND COALESCE(q.last_agent_sequence, 0) > COALESCE(q.last_human_or_operator_sequence, 0) AND COALESCE(q.last_non_system_actor_type, '') = 'agent' THEN 1 ELSE 0 END"
	seenExpr := "CASE WHEN COALESCE(q.last_event_sequence, 0) > COALESCE(r.last_seen_event_sequence, 0) THEN 1 ELSE 0 END"
	switch queueType {
	case model.ForumQueueUnseen:
		clauses = append(clauses, "COALESCE(q.last_event_sequence, 0) > COALESCE(r.last_seen_event_sequence, 0)")
	case model.ForumQueueUnanswered:
		clauses = append(clauses, "v.state IN ('new', 'waiting_human', 'waiting_operator')")
		clauses = append(clauses, "COALESCE(q.last_agent_sequence, 0) > COALESCE(q.last_human_or_operator_sequence, 0)")
		clauses = append(clauses, "COALESCE(q.last_non_system_actor_type, '') = 'agent'")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	sql := fmt.Sprintf(
		`SELECT
  v.thread_id,
  v.ticket,
  v.run_id,
  v.agent_name,
  v.title,
  v.state,
  v.priority,
  v.assignee_type,
  v.assignee_name,
  v.opened_by_type,
  v.opened_by_name,
  v.posts_count,
  v.last_post_at,
  v.last_post_by_type,
  v.last_post_by_name,
  v.opened_at,
  v.updated_at,
  v.closed_at,
  COALESCE(q.last_event_sequence, 0) AS last_event_sequence,
  COALESCE(q.last_non_system_actor_type, '') AS last_actor_type,
  %s AS is_unseen,
  %s AS is_unanswered
FROM forum_thread_views v
LEFT JOIN forum_thread_queue_view q ON q.thread_id = v.thread_id
%s
WHERE %s
ORDER BY
  CASE v.priority
    WHEN 'urgent' THEN 1
    WHEN 'high' THEN 2
    WHEN 'normal' THEN 3
    WHEN 'low' THEN 4
    ELSE 5
  END,
  COALESCE(q.last_event_sequence, 0) DESC,
  v.updated_at DESC
LIMIT %d;`,
		seenExpr,
		unansweredExpr,
		viewerJoin,
		strings.Join(clauses, " AND "),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumThreadView, 0, len(rows))
	for _, row := range rows {
		view, err := parseForumThreadView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *SQLiteStore) MarkForumThreadSeen(threadID string, viewerType model.ForumViewerType, viewerID string, lastSeenEventSequence int64) (*model.ForumThreadSeen, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("forum thread id is required")
	}
	normalizedViewerType, err := normalizeForumViewerType(viewerType)
	if err != nil {
		return nil, err
	}
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" {
		return nil, fmt.Errorf("forum viewer_id is required")
	}
	thread, err := s.GetForumThread(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("forum thread %s not found", threadID)
	}
	if lastSeenEventSequence <= 0 {
		resolvedSequence, err := s.latestForumThreadEventSequence(threadID)
		if err != nil {
			return nil, err
		}
		lastSeenEventSequence = resolvedSequence
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`INSERT INTO forum_thread_reads
  (thread_id, viewer_type, viewer_id, last_seen_event_sequence, updated_at)
VALUES
  (%s, %s, %s, %d, %s)
ON CONFLICT(thread_id, viewer_type, viewer_id) DO UPDATE SET
  last_seen_event_sequence=MAX(forum_thread_reads.last_seen_event_sequence, excluded.last_seen_event_sequence),
  updated_at=CASE
    WHEN excluded.last_seen_event_sequence > forum_thread_reads.last_seen_event_sequence THEN excluded.updated_at
    ELSE forum_thread_reads.updated_at
  END;`,
		quote(threadID),
		quote(string(normalizedViewerType)),
		quote(viewerID),
		lastSeenEventSequence,
		quote(now),
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	rows, err := s.queryJSON(fmt.Sprintf(
		`SELECT thread_id, viewer_type, viewer_id, last_seen_event_sequence, updated_at
FROM forum_thread_reads
WHERE thread_id=%s AND viewer_type=%s AND viewer_id=%s
LIMIT 1;`,
		quote(threadID),
		quote(string(normalizedViewerType)),
		quote(viewerID),
	))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("forum seen row missing after upsert")
	}
	row := rows[0]
	updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse forum_thread_reads updated_at: %w", err)
	}
	return &model.ForumThreadSeen{
		ThreadID:              asString(row["thread_id"]),
		ViewerType:            model.ForumViewerType(asString(row["viewer_type"])),
		ViewerID:              asString(row["viewer_id"]),
		LastSeenEventSequence: int64(asInt(row["last_seen_event_sequence"])),
		UpdatedAt:             updatedAt,
	}, nil
}

func (s *SQLiteStore) latestForumThreadEventSequence(threadID string) (int64, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return 0, fmt.Errorf("forum thread id is required")
	}
	rows, err := s.queryJSON(fmt.Sprintf(
		`SELECT COALESCE(last_event_sequence, 0) AS sequence
FROM forum_thread_queue_view
WHERE thread_id=%s
LIMIT 1;`,
		quote(threadID),
	))
	if err != nil {
		return 0, err
	}
	if len(rows) > 0 {
		return int64(asInt(rows[0]["sequence"])), nil
	}
	rows, err = s.queryJSON(fmt.Sprintf(
		`SELECT COALESCE(MAX(sequence), 0) AS sequence
FROM forum_events
WHERE thread_id=%s;`,
		quote(threadID),
	))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int64(asInt(rows[0]["sequence"])), nil
}

func (s *SQLiteStore) ListForumPosts(threadID string, limit int) ([]model.ForumPost, error) {
	if limit <= 0 {
		limit = 200
	}
	sql := fmt.Sprintf(
		`SELECT post_id, thread_id, event_id, author_type, author_name, body_text, created_at
FROM forum_posts
WHERE thread_id=%s
ORDER BY created_at
LIMIT %d;`,
		quote(threadID),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumPost, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum post created_at: %w", err)
		}
		out = append(out, model.ForumPost{
			PostID:     asString(row["post_id"]),
			ThreadID:   asString(row["thread_id"]),
			EventID:    asString(row["event_id"]),
			AuthorType: model.ForumActorType(asString(row["author_type"])),
			AuthorName: asString(row["author_name"]),
			Body:       asString(row["body_text"]),
			CreatedAt:  createdAt,
		})
	}
	return out, nil
}

func (s *SQLiteStore) ListForumThreadEvents(threadID string, limit int) ([]model.ForumEvent, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("forum thread id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	sql := fmt.Sprintf(
		`SELECT sequence, event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json
FROM forum_events
WHERE thread_id=%s
ORDER BY sequence
LIMIT %d;`,
		quote(threadID),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumEvent, 0, len(rows))
	for _, row := range rows {
		occurredAt, err := time.Parse(time.RFC3339, asString(row["occurred_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum event occurred_at: %w", err)
		}
		out = append(out, model.ForumEvent{
			Sequence: int64(asInt(row["sequence"])),
			Envelope: model.ForumEnvelope{
				EventID:       asString(row["event_id"]),
				EventType:     asString(row["event_type"]),
				EventVersion:  asInt(row["event_version"]),
				OccurredAt:    occurredAt,
				ThreadID:      asString(row["thread_id"]),
				RunID:         asString(row["run_id"]),
				Ticket:        asString(row["ticket"]),
				AgentName:     asString(row["agent_name"]),
				ActorType:     model.ForumActorType(asString(row["actor_type"])),
				ActorName:     asString(row["actor_name"]),
				CorrelationID: asString(row["correlation_id"]),
				CausationID:   asString(row["causation_id"]),
			},
			PayloadJSON: asString(row["payload_json"]),
		})
	}
	return out, nil
}

func (s *SQLiteStore) ListForumThreadStats(ticket string, runID string) ([]model.ForumThreadStats, error) {
	clauses := []string{"1=1"}
	if strings.TrimSpace(ticket) != "" {
		clauses = append(clauses, fmt.Sprintf("ticket=%s", quote(ticket)))
	}
	if strings.TrimSpace(runID) != "" {
		clauses = append(clauses, fmt.Sprintf("run_id=%s", quote(runID)))
	}
	sql := fmt.Sprintf(
		`SELECT ticket, run_id, state, priority, thread_count, updated_at
FROM forum_thread_stats
WHERE %s
ORDER BY ticket, run_id, state, priority;`,
		strings.Join(clauses, " AND "),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumThreadStats, 0, len(rows))
	for _, row := range rows {
		updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum thread stats updated_at: %w", err)
		}
		out = append(out, model.ForumThreadStats{
			Ticket:      asString(row["ticket"]),
			RunID:       asString(row["run_id"]),
			State:       model.ForumThreadState(asString(row["state"])),
			Priority:    model.ForumPriority(asString(row["priority"])),
			ThreadCount: asInt(row["thread_count"]),
			UpdatedAt:   updatedAt,
		})
	}
	return out, nil
}

func (s *SQLiteStore) WatchForumEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	clauses := []string{fmt.Sprintf("sequence > %d", cursor)}
	if strings.TrimSpace(ticket) != "" {
		clauses = append(clauses, fmt.Sprintf("ticket=%s", quote(ticket)))
	}
	sql := fmt.Sprintf(
		`SELECT sequence, event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json
FROM forum_events
WHERE %s
ORDER BY sequence
LIMIT %d;`,
		strings.Join(clauses, " AND "),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumEvent, 0, len(rows))
	for _, row := range rows {
		occurredAt, err := time.Parse(time.RFC3339, asString(row["occurred_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum event occurred_at: %w", err)
		}
		out = append(out, model.ForumEvent{
			Sequence: int64(asInt(row["sequence"])),
			Envelope: model.ForumEnvelope{
				EventID:       asString(row["event_id"]),
				EventType:     asString(row["event_type"]),
				EventVersion:  asInt(row["event_version"]),
				OccurredAt:    occurredAt,
				ThreadID:      asString(row["thread_id"]),
				RunID:         asString(row["run_id"]),
				Ticket:        asString(row["ticket"]),
				AgentName:     asString(row["agent_name"]),
				ActorType:     model.ForumActorType(asString(row["actor_type"])),
				ActorName:     asString(row["actor_name"]),
				CorrelationID: asString(row["correlation_id"]),
				CausationID:   asString(row["causation_id"]),
			},
			PayloadJSON: asString(row["payload_json"]),
		})
	}
	return out, nil
}

func (s *SQLiteStore) ListRecentForumEvents(ticket string, runID string, limit int) ([]model.ForumEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	clauses := []string{"1=1"}
	ticket = strings.TrimSpace(ticket)
	runID = strings.TrimSpace(runID)
	if ticket != "" {
		clauses = append(clauses, fmt.Sprintf("ticket=%s", quote(ticket)))
	}
	if runID != "" {
		clauses = append(clauses, fmt.Sprintf("run_id=%s", quote(runID)))
	}
	sql := fmt.Sprintf(
		`SELECT sequence, event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json
FROM forum_events
WHERE %s
ORDER BY sequence DESC
LIMIT %d;`,
		strings.Join(clauses, " AND "),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumEvent, 0, len(rows))
	for _, row := range rows {
		occurredAt, err := time.Parse(time.RFC3339, asString(row["occurred_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum event occurred_at: %w", err)
		}
		out = append(out, model.ForumEvent{
			Sequence: int64(asInt(row["sequence"])),
			Envelope: model.ForumEnvelope{
				EventID:       asString(row["event_id"]),
				EventType:     asString(row["event_type"]),
				EventVersion:  asInt(row["event_version"]),
				OccurredAt:    occurredAt,
				ThreadID:      asString(row["thread_id"]),
				RunID:         asString(row["run_id"]),
				Ticket:        asString(row["ticket"]),
				AgentName:     asString(row["agent_name"]),
				ActorType:     model.ForumActorType(asString(row["actor_type"])),
				ActorName:     asString(row["actor_name"]),
				CorrelationID: asString(row["correlation_id"]),
				CausationID:   asString(row["causation_id"]),
			},
			PayloadJSON: asString(row["payload_json"]),
		})
	}
	return out, nil
}

func (s *SQLiteStore) GetForumEvent(eventID string) (*model.ForumEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("forum event id is required")
	}
	sql := fmt.Sprintf(
		`SELECT sequence, event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json
FROM forum_events
WHERE event_id=%s
LIMIT 1;`,
		quote(eventID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	occurredAt, err := time.Parse(time.RFC3339, asString(row["occurred_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse forum event occurred_at: %w", err)
	}
	event := model.ForumEvent{
		Sequence: int64(asInt(row["sequence"])),
		Envelope: model.ForumEnvelope{
			EventID:       asString(row["event_id"]),
			EventType:     asString(row["event_type"]),
			EventVersion:  asInt(row["event_version"]),
			OccurredAt:    occurredAt,
			ThreadID:      asString(row["thread_id"]),
			RunID:         asString(row["run_id"]),
			Ticket:        asString(row["ticket"]),
			AgentName:     asString(row["agent_name"]),
			ActorType:     model.ForumActorType(asString(row["actor_type"])),
			ActorName:     asString(row["actor_name"]),
			CorrelationID: asString(row["correlation_id"]),
			CausationID:   asString(row["causation_id"]),
		},
		PayloadJSON: asString(row["payload_json"]),
	}
	return &event, nil
}

func (s *SQLiteStore) ApplyForumEventProjections(event model.ForumEvent) error {
	eventID := strings.TrimSpace(event.Envelope.EventID)
	threadID := strings.TrimSpace(event.Envelope.ThreadID)
	ticket := strings.TrimSpace(event.Envelope.Ticket)
	if eventID == "" {
		return fmt.Errorf("forum projection event_id is required")
	}
	if threadID == "" {
		return fmt.Errorf("forum projection thread_id is required")
	}
	if ticket == "" {
		return fmt.Errorf("forum projection ticket is required")
	}
	if err := s.applyForumProjectionEvent("forum_thread_views_v1", eventID, func() error {
		return s.refreshForumThreadView(threadID)
	}); err != nil {
		return err
	}
	if err := s.applyForumProjectionEvent("forum_thread_queue_view_v1", eventID, func() error {
		return s.refreshForumThreadQueueView(threadID)
	}); err != nil {
		return err
	}
	return s.applyForumProjectionEvent("forum_thread_stats_v1", eventID, func() error {
		return s.refreshForumThreadStats(ticket)
	})
}

func (s *SQLiteStore) ForumAppendIntegrationEvent(envelope model.ForumEnvelope, payload map[string]any) error {
	if strings.TrimSpace(envelope.EventID) == "" {
		return fmt.Errorf("forum integration event_id is required")
	}
	if ok, err := s.forumEventExists(envelope.EventID); err != nil {
		return err
	} else if ok {
		return nil
	}
	occurredAt := envelope.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	if strings.TrimSpace(envelope.EventType) == "" {
		envelope.EventType = "forum.integration.unknown"
	}
	if envelope.EventVersion <= 0 {
		envelope.EventVersion = 1
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal forum integration payload: %w", err)
	}
	sql := fmt.Sprintf(
		`INSERT INTO forum_events
  (event_id, event_type, event_version, occurred_at, thread_id, run_id, ticket, agent_name, actor_type, actor_name, correlation_id, causation_id, payload_json)
VALUES
  (%s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);`,
		quote(envelope.EventID),
		quote(envelope.EventType),
		envelope.EventVersion,
		quote(occurredAt.Format(time.RFC3339)),
		quote(envelope.ThreadID),
		quote(envelope.RunID),
		quote(envelope.Ticket),
		quote(envelope.AgentName),
		quote(string(envelope.ActorType)),
		quote(envelope.ActorName),
		quote(envelope.CorrelationID),
		quote(envelope.CausationID),
		quote(string(payloadJSON)),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) refreshForumThreadStats(ticket string) error {
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
DELETE FROM forum_thread_stats WHERE ticket=%s;
INSERT INTO forum_thread_stats (ticket, run_id, state, priority, thread_count, updated_at)
SELECT ticket, run_id, state, priority, COUNT(*), %s
FROM forum_thread_views
WHERE ticket=%s
GROUP BY ticket, run_id, state, priority;
COMMIT;`,
		quote(ticket),
		quote(now),
		quote(ticket),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) forumEventExists(eventID string) (bool, error) {
	if strings.TrimSpace(eventID) == "" {
		return false, fmt.Errorf("forum event id is required")
	}
	rows, err := s.queryJSON(fmt.Sprintf(
		`SELECT event_id FROM forum_events WHERE event_id=%s LIMIT 1;`,
		quote(eventID),
	))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *SQLiteStore) applyForumProjectionEvent(projectionName string, eventID string, apply func() error) error {
	projectionName = strings.TrimSpace(projectionName)
	eventID = strings.TrimSpace(eventID)
	if projectionName == "" {
		return fmt.Errorf("forum projection name is required")
	}
	if eventID == "" {
		return fmt.Errorf("forum projection event_id is required")
	}
	applied, err := s.forumProjectionEventExists(projectionName, eventID)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	if err := apply(); err != nil {
		return err
	}
	return s.insertForumProjectionEvent(projectionName, eventID)
}

func (s *SQLiteStore) forumProjectionEventExists(projectionName string, eventID string) (bool, error) {
	rows, err := s.queryJSON(fmt.Sprintf(
		`SELECT event_id
FROM forum_projection_events
WHERE projection_name=%s AND event_id=%s
LIMIT 1;`,
		quote(projectionName),
		quote(eventID),
	))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *SQLiteStore) insertForumProjectionEvent(projectionName string, eventID string) error {
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`INSERT OR IGNORE INTO forum_projection_events
  (projection_name, event_id, applied_at)
VALUES
  (%s, %s, %s);`,
		quote(projectionName),
		quote(eventID),
		quote(now),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) refreshForumThreadView(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("forum thread id is required")
	}
	sql := fmt.Sprintf(
		`INSERT INTO forum_thread_views
  (thread_id, ticket, run_id, agent_name, title, state, priority, assignee_type, assignee_name, opened_by_type, opened_by_name, posts_count, last_post_at, last_post_by_type, last_post_by_name, opened_at, updated_at, closed_at)
SELECT
  t.thread_id,
  t.ticket,
  t.run_id,
  t.agent_name,
  t.title,
  t.state,
  t.priority,
  t.assignee_type,
  t.assignee_name,
  t.opened_by_type,
  t.opened_by_name,
  (SELECT COUNT(*) FROM forum_posts p WHERE p.thread_id=t.thread_id),
  COALESCE((SELECT p.created_at FROM forum_posts p WHERE p.thread_id=t.thread_id ORDER BY p.created_at DESC, p.post_id DESC LIMIT 1), ''),
  COALESCE((SELECT p.author_type FROM forum_posts p WHERE p.thread_id=t.thread_id ORDER BY p.created_at DESC, p.post_id DESC LIMIT 1), ''),
  COALESCE((SELECT p.author_name FROM forum_posts p WHERE p.thread_id=t.thread_id ORDER BY p.created_at DESC, p.post_id DESC LIMIT 1), ''),
  t.opened_at,
  t.updated_at,
  t.closed_at
FROM forum_threads t
WHERE t.thread_id=%s
ON CONFLICT(thread_id) DO UPDATE SET
  ticket=excluded.ticket,
  run_id=excluded.run_id,
  agent_name=excluded.agent_name,
  title=excluded.title,
  state=excluded.state,
  priority=excluded.priority,
  assignee_type=excluded.assignee_type,
  assignee_name=excluded.assignee_name,
  opened_by_type=excluded.opened_by_type,
  opened_by_name=excluded.opened_by_name,
  posts_count=excluded.posts_count,
  last_post_at=excluded.last_post_at,
  last_post_by_type=excluded.last_post_by_type,
  last_post_by_name=excluded.last_post_by_name,
  opened_at=excluded.opened_at,
  updated_at=excluded.updated_at,
  closed_at=excluded.closed_at;`,
		quote(threadID),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) refreshForumThreadQueueView(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("forum thread id is required")
	}
	sql := fmt.Sprintf(
		`INSERT INTO forum_thread_queue_view
  (thread_id, ticket, run_id, state, priority, assignee_name, last_event_sequence, last_actor_type, last_non_system_actor_type, last_human_or_operator_sequence, last_agent_sequence, updated_at)
SELECT
  t.thread_id,
  t.ticket,
  t.run_id,
  t.state,
  t.priority,
  t.assignee_name,
  COALESCE((SELECT e.sequence FROM forum_events e WHERE e.thread_id=t.thread_id ORDER BY e.sequence DESC LIMIT 1), 0),
  COALESCE((SELECT e.actor_type FROM forum_events e WHERE e.thread_id=t.thread_id ORDER BY e.sequence DESC LIMIT 1), ''),
  COALESCE((SELECT e.actor_type FROM forum_events e WHERE e.thread_id=t.thread_id AND e.actor_type != 'system' ORDER BY e.sequence DESC LIMIT 1), ''),
  COALESCE((SELECT MAX(e.sequence) FROM forum_events e WHERE e.thread_id=t.thread_id AND e.actor_type IN ('human', 'operator')), 0),
  COALESCE((SELECT MAX(e.sequence) FROM forum_events e WHERE e.thread_id=t.thread_id AND e.actor_type='agent'), 0),
  t.updated_at
FROM forum_threads t
WHERE t.thread_id=%s
ON CONFLICT(thread_id) DO UPDATE SET
  ticket=excluded.ticket,
  run_id=excluded.run_id,
  state=excluded.state,
  priority=excluded.priority,
  assignee_name=excluded.assignee_name,
  last_event_sequence=excluded.last_event_sequence,
  last_actor_type=excluded.last_actor_type,
  last_non_system_actor_type=excluded.last_non_system_actor_type,
  last_human_or_operator_sequence=excluded.last_human_or_operator_sequence,
  last_agent_sequence=excluded.last_agent_sequence,
  updated_at=excluded.updated_at;`,
		quote(threadID),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) UpsertForumControlThread(mapping model.ForumControlThread) error {
	runID := strings.TrimSpace(mapping.RunID)
	agentName := strings.TrimSpace(mapping.AgentName)
	threadID := strings.TrimSpace(mapping.ThreadID)
	ticket := strings.TrimSpace(mapping.Ticket)
	if runID == "" {
		return fmt.Errorf("forum control thread run_id is required")
	}
	if agentName == "" {
		return fmt.Errorf("forum control thread agent_name is required")
	}
	if threadID == "" {
		return fmt.Errorf("forum control thread thread_id is required")
	}
	if ticket == "" {
		return fmt.Errorf("forum control thread ticket is required")
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`INSERT INTO forum_control_threads
  (run_id, agent_name, ticket, thread_id, created_at, updated_at)
VALUES
  (%s, %s, %s, %s, %s, %s)
ON CONFLICT(run_id, agent_name) DO UPDATE SET
  ticket=excluded.ticket,
  thread_id=excluded.thread_id,
  updated_at=excluded.updated_at;`,
		quote(runID),
		quote(agentName),
		quote(ticket),
		quote(threadID),
		quote(now),
		quote(now),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) GetForumControlThread(runID string, agentName string) (*model.ForumControlThread, error) {
	runID = strings.TrimSpace(runID)
	agentName = strings.TrimSpace(agentName)
	if runID == "" || agentName == "" {
		return nil, nil
	}
	sql := fmt.Sprintf(
		`SELECT run_id, agent_name, ticket, thread_id, created_at, updated_at
FROM forum_control_threads
WHERE run_id=%s AND agent_name=%s
LIMIT 1;`,
		quote(runID),
		quote(agentName),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse forum_control_threads created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse forum_control_threads updated_at: %w", err)
	}
	return &model.ForumControlThread{
		RunID:     asString(row["run_id"]),
		AgentName: asString(row["agent_name"]),
		Ticket:    asString(row["ticket"]),
		ThreadID:  asString(row["thread_id"]),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (s *SQLiteStore) ListForumControlThreads(runID string) ([]model.ForumControlThread, error) {
	runID = strings.TrimSpace(runID)
	clauses := []string{"1=1"}
	if runID != "" {
		clauses = append(clauses, fmt.Sprintf("run_id=%s", quote(runID)))
	}
	sql := fmt.Sprintf(
		`SELECT run_id, agent_name, ticket, thread_id, created_at, updated_at
FROM forum_control_threads
WHERE %s
ORDER BY run_id, agent_name;`,
		strings.Join(clauses, " AND "),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumControlThread, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum_control_threads created_at: %w", err)
		}
		updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum_control_threads updated_at: %w", err)
		}
		out = append(out, model.ForumControlThread{
			RunID:     asString(row["run_id"]),
			AgentName: asString(row["agent_name"]),
			Ticket:    asString(row["ticket"]),
			ThreadID:  asString(row["thread_id"]),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	return out, nil
}

func (s *SQLiteStore) EnqueueForumOutbox(message model.ForumOutboxMessage) error {
	messageID := strings.TrimSpace(message.MessageID)
	topic := strings.TrimSpace(message.Topic)
	payload := strings.TrimSpace(message.PayloadJSON)
	if messageID == "" {
		return fmt.Errorf("forum outbox message_id is required")
	}
	if topic == "" {
		return fmt.Errorf("forum outbox topic is required")
	}
	if payload == "" {
		return fmt.Errorf("forum outbox payload_json is required")
	}
	status := message.Status
	if strings.TrimSpace(string(status)) == "" {
		status = model.ForumOutboxStatusPending
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`INSERT OR IGNORE INTO forum_outbox
  (message_id, topic, message_key, payload_json, status, attempt_count, last_error, created_at, updated_at, sent_at)
VALUES
  (%s, %s, %s, %s, %s, %d, %s, %s, %s, '');`,
		quote(messageID),
		quote(topic),
		quote(strings.TrimSpace(message.MessageKey)),
		quote(payload),
		quote(string(status)),
		message.AttemptCount,
		quote(strings.TrimSpace(message.LastError)),
		quote(now),
		quote(now),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) ClaimForumOutboxPending(limit int) ([]model.ForumOutboxMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	marker := time.Now().UTC().Format(time.RFC3339Nano)
	sql := fmt.Sprintf(
		`BEGIN IMMEDIATE;
UPDATE forum_outbox
SET status=%s,
    attempt_count=attempt_count+1,
    updated_at=%s
WHERE id IN (
  SELECT id
  FROM forum_outbox
  WHERE status IN (%s, %s)
  ORDER BY created_at, id
  LIMIT %d
);
COMMIT;`,
		quote(string(model.ForumOutboxStatusProcessing)),
		quote(marker),
		quote(string(model.ForumOutboxStatusPending)),
		quote(string(model.ForumOutboxStatusFailed)),
		limit,
	)
	if err := s.execSQL(sql); err != nil {
		return nil, err
	}
	return s.listForumOutboxByStatusAndUpdatedAt(model.ForumOutboxStatusProcessing, marker)
}

func (s *SQLiteStore) MarkForumOutboxSent(messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("forum outbox message_id is required")
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`UPDATE forum_outbox
SET status=%s,
    last_error='',
    sent_at=%s,
    updated_at=%s
WHERE message_id=%s;`,
		quote(string(model.ForumOutboxStatusSent)),
		quote(now),
		quote(now),
		quote(messageID),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) MarkForumOutboxFailed(messageID string, lastError string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("forum outbox message_id is required")
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`UPDATE forum_outbox
SET status=%s,
    last_error=%s,
    updated_at=%s
WHERE message_id=%s;`,
		quote(string(model.ForumOutboxStatusFailed)),
		quote(strings.TrimSpace(lastError)),
		quote(now),
		quote(messageID),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) ListForumOutboxByStatus(status model.ForumOutboxStatus, limit int) ([]model.ForumOutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	sql := fmt.Sprintf(
		`SELECT id, message_id, topic, message_key, payload_json, status, attempt_count, last_error, created_at, updated_at, sent_at
FROM forum_outbox
WHERE status=%s
ORDER BY id
LIMIT %d;`,
		quote(string(status)),
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumOutboxMessage, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum_outbox created_at: %w", err)
		}
		updatedAtParsed, err := time.Parse(time.RFC3339Nano, asString(row["updated_at"]))
		if err != nil {
			updatedAtParsed, err = time.Parse(time.RFC3339, asString(row["updated_at"]))
			if err != nil {
				return nil, fmt.Errorf("parse forum_outbox updated_at: %w", err)
			}
		}
		out = append(out, model.ForumOutboxMessage{
			ID:           int64(asInt(row["id"])),
			MessageID:    asString(row["message_id"]),
			Topic:        asString(row["topic"]),
			MessageKey:   asString(row["message_key"]),
			PayloadJSON:  asString(row["payload_json"]),
			Status:       model.ForumOutboxStatus(asString(row["status"])),
			AttemptCount: asInt(row["attempt_count"]),
			LastError:    asString(row["last_error"]),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAtParsed,
			SentAt:       parseTimePtr(asString(row["sent_at"])),
		})
	}
	return out, nil
}

func (s *SQLiteStore) ListRecentForumOutbox(limit int) ([]model.ForumOutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	sql := fmt.Sprintf(
		`SELECT id, message_id, topic, message_key, payload_json, status, attempt_count, last_error, created_at, updated_at, sent_at
FROM forum_outbox
ORDER BY id DESC
LIMIT %d;`,
		limit,
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumOutboxMessage, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum_outbox created_at: %w", err)
		}
		updatedAtParsed, err := time.Parse(time.RFC3339Nano, asString(row["updated_at"]))
		if err != nil {
			updatedAtParsed, err = time.Parse(time.RFC3339, asString(row["updated_at"]))
			if err != nil {
				return nil, fmt.Errorf("parse forum_outbox updated_at: %w", err)
			}
		}
		out = append(out, model.ForumOutboxMessage{
			ID:           int64(asInt(row["id"])),
			MessageID:    asString(row["message_id"]),
			Topic:        asString(row["topic"]),
			MessageKey:   asString(row["message_key"]),
			PayloadJSON:  asString(row["payload_json"]),
			Status:       model.ForumOutboxStatus(asString(row["status"])),
			AttemptCount: asInt(row["attempt_count"]),
			LastError:    asString(row["last_error"]),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAtParsed,
			SentAt:       parseTimePtr(asString(row["sent_at"])),
		})
	}
	return out, nil
}

func (s *SQLiteStore) CountForumOutboxByStatus(status model.ForumOutboxStatus) (int, error) {
	sql := fmt.Sprintf(
		`SELECT count(*) AS count
FROM forum_outbox
WHERE status=%s;`,
		quote(string(status)),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return asInt(rows[0]["count"]), nil
}

func (s *SQLiteStore) OldestForumOutboxCreatedAt(status model.ForumOutboxStatus) (*time.Time, error) {
	sql := fmt.Sprintf(
		`SELECT created_at
FROM forum_outbox
WHERE status=%s
ORDER BY created_at, id
LIMIT 1;`,
		quote(string(status)),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	value := strings.TrimSpace(asString(rows[0]["created_at"]))
	if value == "" {
		return nil, nil
	}
	createdAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("parse oldest forum_outbox created_at: %w", err)
	}
	return &createdAt, nil
}

func (s *SQLiteStore) listForumOutboxByStatusAndUpdatedAt(status model.ForumOutboxStatus, updatedAt string) ([]model.ForumOutboxMessage, error) {
	sql := fmt.Sprintf(
		`SELECT id, message_id, topic, message_key, payload_json, status, attempt_count, last_error, created_at, updated_at, sent_at
FROM forum_outbox
WHERE status=%s AND updated_at=%s
ORDER BY id;`,
		quote(string(status)),
		quote(updatedAt),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.ForumOutboxMessage, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse forum_outbox created_at: %w", err)
		}
		updatedAtParsed, err := time.Parse(time.RFC3339Nano, asString(row["updated_at"]))
		if err != nil {
			updatedAtParsed, err = time.Parse(time.RFC3339, asString(row["updated_at"]))
			if err != nil {
				return nil, fmt.Errorf("parse forum_outbox updated_at: %w", err)
			}
		}
		out = append(out, model.ForumOutboxMessage{
			ID:           int64(asInt(row["id"])),
			MessageID:    asString(row["message_id"]),
			Topic:        asString(row["topic"]),
			MessageKey:   asString(row["message_key"]),
			PayloadJSON:  asString(row["payload_json"]),
			Status:       model.ForumOutboxStatus(asString(row["status"])),
			AttemptCount: asInt(row["attempt_count"]),
			LastError:    asString(row["last_error"]),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAtParsed,
			SentAt:       parseTimePtr(asString(row["sent_at"])),
		})
	}
	return out, nil
}

func parseForumThreadView(row map[string]any) (model.ForumThreadView, error) {
	openedAt, err := time.Parse(time.RFC3339, asString(row["opened_at"]))
	if err != nil {
		return model.ForumThreadView{}, fmt.Errorf("parse forum thread opened_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
	if err != nil {
		return model.ForumThreadView{}, fmt.Errorf("parse forum thread updated_at: %w", err)
	}
	return model.ForumThreadView{
		ThreadID:          asString(row["thread_id"]),
		Ticket:            asString(row["ticket"]),
		RunID:             asString(row["run_id"]),
		AgentName:         asString(row["agent_name"]),
		Title:             asString(row["title"]),
		State:             model.ForumThreadState(asString(row["state"])),
		Priority:          model.ForumPriority(asString(row["priority"])),
		AssigneeType:      model.ForumActorType(asString(row["assignee_type"])),
		AssigneeName:      asString(row["assignee_name"]),
		OpenedByType:      model.ForumActorType(asString(row["opened_by_type"])),
		OpenedByName:      asString(row["opened_by_name"]),
		PostsCount:        asInt(row["posts_count"]),
		LastPostAt:        parseTimePtr(asString(row["last_post_at"])),
		LastPostByType:    model.ForumActorType(asString(row["last_post_by_type"])),
		LastPostByName:    asString(row["last_post_by_name"]),
		LastEventSequence: int64(asInt(row["last_event_sequence"])),
		LastActorType:     model.ForumActorType(asString(row["last_actor_type"])),
		IsUnseen:          asInt(row["is_unseen"]) == 1,
		IsUnanswered:      asInt(row["is_unanswered"]) == 1,
		OpenedAt:          openedAt,
		UpdatedAt:         updatedAt,
		ClosedAt:          parseTimePtr(asString(row["closed_at"])),
	}, nil
}

func normalizeForumViewerType(viewerType model.ForumViewerType) (model.ForumViewerType, error) {
	normalized := model.ForumViewerType(strings.ToLower(strings.TrimSpace(string(viewerType))))
	switch normalized {
	case model.ForumViewerHuman, model.ForumViewerAgent:
		return normalized, nil
	default:
		return "", fmt.Errorf("forum viewer_type must be human or agent")
	}
}

func normalizeForumQueueType(queueType model.ForumQueueType) (model.ForumQueueType, error) {
	normalized := model.ForumQueueType(strings.ToLower(strings.TrimSpace(string(queueType))))
	switch normalized {
	case model.ForumQueueUnseen, model.ForumQueueUnanswered:
		return normalized, nil
	default:
		return "", fmt.Errorf("forum queue type must be unseen or unanswered")
	}
}
