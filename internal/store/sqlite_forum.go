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
	return s.GetForumThread(cmd.Envelope.ThreadID)
}

func (s *SQLiteStore) GetForumThread(threadID string) (*model.ForumThreadView, error) {
	sql := fmt.Sprintf(
		`SELECT thread_id, ticket, run_id, agent_name, title, state, priority, assignee_type, assignee_name, opened_by_type, opened_by_name, posts_count, last_post_at, last_post_by_type, last_post_by_name, opened_at, updated_at, closed_at
FROM forum_thread_views
WHERE thread_id=%s;`,
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
		clauses = append(clauses, fmt.Sprintf("ticket=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.RunID); v != "" {
		clauses = append(clauses, fmt.Sprintf("run_id=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.State)); v != "" {
		clauses = append(clauses, fmt.Sprintf("state=%s", quote(v)))
	}
	if v := strings.TrimSpace(string(filter.Priority)); v != "" {
		clauses = append(clauses, fmt.Sprintf("priority=%s", quote(v)))
	}
	if v := strings.TrimSpace(filter.Assignee); v != "" {
		clauses = append(clauses, fmt.Sprintf("assignee_name=%s", quote(v)))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	sql := fmt.Sprintf(
		`SELECT thread_id, ticket, run_id, agent_name, title, state, priority, assignee_type, assignee_name, opened_by_type, opened_by_name, posts_count, last_post_at, last_post_by_type, last_post_by_name, opened_at, updated_at, closed_at
FROM forum_thread_views
WHERE %s
ORDER BY
  CASE priority
    WHEN 'urgent' THEN 1
    WHEN 'high' THEN 2
    WHEN 'normal' THEN 3
    WHEN 'low' THEN 4
    ELSE 5
  END,
  updated_at DESC
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
		ThreadID:       asString(row["thread_id"]),
		Ticket:         asString(row["ticket"]),
		RunID:          asString(row["run_id"]),
		AgentName:      asString(row["agent_name"]),
		Title:          asString(row["title"]),
		State:          model.ForumThreadState(asString(row["state"])),
		Priority:       model.ForumPriority(asString(row["priority"])),
		AssigneeType:   model.ForumActorType(asString(row["assignee_type"])),
		AssigneeName:   asString(row["assignee_name"]),
		OpenedByType:   model.ForumActorType(asString(row["opened_by_type"])),
		OpenedByName:   asString(row["opened_by_name"]),
		PostsCount:     asInt(row["posts_count"]),
		LastPostAt:     parseTimePtr(asString(row["last_post_at"])),
		LastPostByType: model.ForumActorType(asString(row["last_post_by_type"])),
		LastPostByName: asString(row["last_post_by_name"]),
		OpenedAt:       openedAt,
		UpdatedAt:      updatedAt,
		ClosedAt:       parseTimePtr(asString(row["closed_at"])),
	}, nil
}
