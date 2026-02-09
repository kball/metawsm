package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/policy"
)

type ForumOpenThreadOptions struct {
	ThreadID      string
	Ticket        string
	RunID         string
	AgentName     string
	Title         string
	Body          string
	Priority      model.ForumPriority
	ActorType     model.ForumActorType
	ActorName     string
	CorrelationID string
	CausationID   string
}

type ForumAddPostOptions struct {
	ThreadID      string
	Body          string
	ActorType     model.ForumActorType
	ActorName     string
	CorrelationID string
	CausationID   string
}

type ForumAssignThreadOptions struct {
	ThreadID       string
	AssigneeType   model.ForumActorType
	AssigneeName   string
	AssignmentNote string
	ActorType      model.ForumActorType
	ActorName      string
	CorrelationID  string
	CausationID    string
}

type ForumChangeStateOptions struct {
	ThreadID      string
	ToState       model.ForumThreadState
	ActorType     model.ForumActorType
	ActorName     string
	CorrelationID string
	CausationID   string
}

type ForumSetPriorityOptions struct {
	ThreadID      string
	Priority      model.ForumPriority
	ActorType     model.ForumActorType
	ActorName     string
	CorrelationID string
	CausationID   string
}

type ForumThreadDetail struct {
	Thread model.ForumThreadView `json:"thread"`
	Posts  []model.ForumPost     `json:"posts"`
}

type ForumControlSignalOptions struct {
	RunID         string
	Ticket        string
	AgentName     string
	ActorType     model.ForumActorType
	ActorName     string
	CorrelationID string
	CausationID   string
	Payload       model.ForumControlPayloadV1
}

func (s *Service) ForumOpenThread(ctx context.Context, options ForumOpenThreadOptions) (model.ForumThreadView, error) {
	_ = ctx
	ticket := strings.TrimSpace(options.Ticket)
	if ticket == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum ticket is required")
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum title is required")
	}
	body := strings.TrimSpace(options.Body)
	if body == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum body is required")
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	priority, err := normalizeForumPriority(options.Priority)
	if err != nil {
		return model.ForumThreadView{}, err
	}

	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		threadID = generateForumID("fthr")
	}
	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}

	thread, err := s.store.ForumOpenThread(model.ForumOpenThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.thread.opened",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         strings.TrimSpace(options.RunID),
			Ticket:        ticket,
			AgentName:     strings.TrimSpace(options.AgentName),
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		Title:    title,
		Body:     body,
		Priority: priority,
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread open returned nil thread")
	}
	return *thread, nil
}

func (s *Service) ForumAddPost(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	body := strings.TrimSpace(options.Body)
	if body == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum body is required")
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if current.State == model.ForumThreadStateClosed {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s is closed", threadID)
	}

	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	thread, err := s.store.ForumAddPost(model.ForumAddPostCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.post.added",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		Body: body,
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum add-post returned nil thread")
	}
	return *thread, nil
}

func (s *Service) ForumAnswerThread(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if current.State == model.ForumThreadStateClosed {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s is closed", threadID)
	}

	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = generateForumID("fcorr")
	}
	addEventID := generateForumID("fevt")
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if _, err := s.store.ForumAddPost(model.ForumAddPostCommand{
		Envelope: model.ForumEnvelope{
			EventID:       addEventID,
			EventType:     "forum.post.added",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		Body: strings.TrimSpace(options.Body),
	}); err != nil {
		return model.ForumThreadView{}, err
	}

	stateChanged, err := s.store.ForumChangeState(model.ForumChangeStateCommand{
		Envelope: model.ForumEnvelope{
			EventID:       generateForumID("fevt"),
			EventType:     "forum.state.changed",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   addEventID,
		},
		ToState: model.ForumThreadStateAnswered,
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if stateChanged == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum answer returned nil thread")
	}
	_ = s.forumEmitDocsSyncRequestedEvent(*stateChanged, actorType, strings.TrimSpace(options.ActorName), correlationID, addEventID, "answered")
	return *stateChanged, nil
}

func (s *Service) ForumAssignThread(ctx context.Context, options ForumAssignThreadOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	assigneeType, err := normalizeForumActorType(options.AssigneeType)
	if err != nil {
		return model.ForumThreadView{}, fmt.Errorf("invalid assignee type: %w", err)
	}
	if strings.TrimSpace(options.AssigneeName) == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum assignee name is required")
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if current.State == model.ForumThreadStateClosed {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s is closed", threadID)
	}

	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	thread, err := s.store.ForumAssignThread(model.ForumAssignThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.assigned",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		AssigneeType:   assigneeType,
		AssigneeName:   strings.TrimSpace(options.AssigneeName),
		AssignmentNote: strings.TrimSpace(options.AssignmentNote),
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum assign returned nil thread")
	}
	return *thread, nil
}

func (s *Service) ForumChangeState(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	toState, err := normalizeForumState(options.ToState)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if !forumTransitionAllowed(current.State, toState) {
		return model.ForumThreadView{}, fmt.Errorf("forum state transition %s -> %s is not allowed", current.State, toState)
	}
	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	thread, err := s.store.ForumChangeState(model.ForumChangeStateCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.state.changed",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		ToState: toState,
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum state change returned nil thread")
	}
	return *thread, nil
}

func (s *Service) ForumSetPriority(ctx context.Context, options ForumSetPriorityOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	priority, err := normalizeForumPriority(options.Priority)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if current.State == model.ForumThreadStateClosed {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s is closed", threadID)
	}
	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	thread, err := s.store.ForumSetPriority(model.ForumSetPriorityCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.priority.changed",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		Priority: priority,
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum priority update returned nil thread")
	}
	return *thread, nil
}

func (s *Service) ForumCloseThread(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	_ = ctx
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum thread id is required")
	}
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	current, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if current == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum thread %s not found", threadID)
	}
	if current.State == model.ForumThreadStateClosed {
		return *current, nil
	}
	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	thread, err := s.store.ForumCloseThread(model.ForumCloseThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.thread.closed",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      threadID,
			RunID:         current.RunID,
			Ticket:        current.Ticket,
			AgentName:     current.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if thread == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum close returned nil thread")
	}
	_ = s.forumEmitDocsSyncRequestedEvent(*thread, actorType, strings.TrimSpace(options.ActorName), correlationID, eventID, "closed")
	return *thread, nil
}

func (s *Service) ForumListThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error) {
	return s.store.ListForumThreads(filter)
}

func (s *Service) ForumGetThread(threadID string) (*ForumThreadDetail, error) {
	thread, err := s.store.GetForumThread(strings.TrimSpace(threadID))
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, nil
	}
	posts, err := s.store.ListForumPosts(thread.ThreadID, 200)
	if err != nil {
		return nil, err
	}
	return &ForumThreadDetail{Thread: *thread, Posts: posts}, nil
}

func (s *Service) ForumListStats(ticket string, runID string) ([]model.ForumThreadStats, error) {
	return s.store.ListForumThreadStats(strings.TrimSpace(ticket), strings.TrimSpace(runID))
}

func (s *Service) ForumWatchEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
	return s.store.WatchForumEvents(strings.TrimSpace(ticket), cursor, limit)
}

func (s *Service) ForumAppendControlSignal(ctx context.Context, options ForumControlSignalOptions) (model.ForumThreadView, error) {
	_ = ctx
	actorType, err := normalizeForumActorType(options.ActorType)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	payload := options.Payload
	if err := payload.Validate(); err != nil {
		return model.ForumThreadView{}, err
	}
	if payload.RunID != strings.TrimSpace(options.RunID) {
		return model.ForumThreadView{}, fmt.Errorf("forum control payload run_id mismatch")
	}
	if payload.AgentName != strings.TrimSpace(options.AgentName) {
		return model.ForumThreadView{}, fmt.Errorf("forum control payload agent_name mismatch")
	}
	thread, err := s.ensureForumControlThread(payload.RunID, payload.AgentName, strings.TrimSpace(options.Ticket))
	if err != nil {
		return model.ForumThreadView{}, err
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return model.ForumThreadView{}, fmt.Errorf("marshal forum control payload: %w", err)
	}
	eventID := generateForumID("fevt")
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = eventID
	}
	next, err := s.store.ForumAddPost(model.ForumAddPostCommand{
		Envelope: model.ForumEnvelope{
			EventID:       eventID,
			EventType:     "forum.control.signal",
			EventVersion:  1,
			OccurredAt:    time.Now(),
			ThreadID:      thread.ThreadID,
			RunID:         thread.RunID,
			Ticket:        thread.Ticket,
			AgentName:     thread.AgentName,
			ActorType:     actorType,
			ActorName:     strings.TrimSpace(options.ActorName),
			CorrelationID: correlationID,
			CausationID:   strings.TrimSpace(options.CausationID),
		},
		Body: string(bodyBytes),
	})
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if next == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum control append returned nil thread")
	}
	return *next, nil
}

func normalizeForumActorType(actorType model.ForumActorType) (model.ForumActorType, error) {
	switch strings.TrimSpace(strings.ToLower(string(actorType))) {
	case string(model.ForumActorAgent):
		return model.ForumActorAgent, nil
	case string(model.ForumActorOperator):
		return model.ForumActorOperator, nil
	case string(model.ForumActorHuman):
		return model.ForumActorHuman, nil
	case string(model.ForumActorSystem):
		return model.ForumActorSystem, nil
	default:
		return "", fmt.Errorf("forum actor_type must be one of agent|operator|human|system")
	}
}

func normalizeForumPriority(priority model.ForumPriority) (model.ForumPriority, error) {
	switch strings.TrimSpace(strings.ToLower(string(priority))) {
	case string(model.ForumPriorityLow):
		return model.ForumPriorityLow, nil
	case string(model.ForumPriorityNormal), "":
		return model.ForumPriorityNormal, nil
	case string(model.ForumPriorityHigh):
		return model.ForumPriorityHigh, nil
	case string(model.ForumPriorityUrgent):
		return model.ForumPriorityUrgent, nil
	default:
		return "", fmt.Errorf("forum priority must be one of low|normal|high|urgent")
	}
}

func normalizeForumState(state model.ForumThreadState) (model.ForumThreadState, error) {
	switch strings.TrimSpace(strings.ToLower(string(state))) {
	case string(model.ForumThreadStateNew):
		return model.ForumThreadStateNew, nil
	case string(model.ForumThreadStateTriaged):
		return model.ForumThreadStateTriaged, nil
	case string(model.ForumThreadStateWaitingOperator):
		return model.ForumThreadStateWaitingOperator, nil
	case string(model.ForumThreadStateWaitingHuman):
		return model.ForumThreadStateWaitingHuman, nil
	case string(model.ForumThreadStateAnswered):
		return model.ForumThreadStateAnswered, nil
	case string(model.ForumThreadStateClosed):
		return model.ForumThreadStateClosed, nil
	default:
		return "", fmt.Errorf("forum state must be one of new|triaged|waiting_operator|waiting_human|answered|closed")
	}
}

func forumTransitionAllowed(from model.ForumThreadState, to model.ForumThreadState) bool {
	if from == to {
		return true
	}
	switch from {
	case model.ForumThreadStateNew:
		return to == model.ForumThreadStateTriaged || to == model.ForumThreadStateWaitingOperator || to == model.ForumThreadStateWaitingHuman || to == model.ForumThreadStateAnswered || to == model.ForumThreadStateClosed
	case model.ForumThreadStateTriaged:
		return to == model.ForumThreadStateWaitingOperator || to == model.ForumThreadStateWaitingHuman || to == model.ForumThreadStateAnswered || to == model.ForumThreadStateClosed
	case model.ForumThreadStateWaitingOperator:
		return to == model.ForumThreadStateWaitingHuman || to == model.ForumThreadStateAnswered || to == model.ForumThreadStateTriaged || to == model.ForumThreadStateClosed
	case model.ForumThreadStateWaitingHuman:
		return to == model.ForumThreadStateWaitingOperator || to == model.ForumThreadStateAnswered || to == model.ForumThreadStateTriaged || to == model.ForumThreadStateClosed
	case model.ForumThreadStateAnswered:
		return to == model.ForumThreadStateWaitingOperator || to == model.ForumThreadStateWaitingHuman || to == model.ForumThreadStateClosed
	case model.ForumThreadStateClosed:
		return false
	default:
		return false
	}
}

func generateForumID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "forum"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func sanitizeForumIDSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func forumControlThreadID(runID string, agentName string) string {
	return fmt.Sprintf("fctrl-%s-%s", sanitizeForumIDSegment(runID), sanitizeForumIDSegment(agentName))
}

func (s *Service) ensureForumControlThread(runID string, agentName string, ticket string) (model.ForumThreadView, error) {
	runID = strings.TrimSpace(runID)
	agentName = strings.TrimSpace(agentName)
	ticket = strings.TrimSpace(ticket)
	if runID == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum control run_id is required")
	}
	if agentName == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum control agent_name is required")
	}
	if ticket == "" {
		return model.ForumThreadView{}, fmt.Errorf("forum control ticket is required")
	}
	mapping, err := s.store.GetForumControlThread(runID, agentName)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if mapping != nil {
		thread, err := s.store.GetForumThread(mapping.ThreadID)
		if err != nil {
			return model.ForumThreadView{}, err
		}
		if thread != nil {
			return *thread, nil
		}
	}
	threadID := forumControlThreadID(runID, agentName)
	existing, err := s.store.GetForumThread(threadID)
	if err != nil {
		return model.ForumThreadView{}, err
	}
	if existing == nil {
		open, err := s.store.ForumOpenThread(model.ForumOpenThreadCommand{
			Envelope: model.ForumEnvelope{
				EventID:       generateForumID("fevt"),
				EventType:     "forum.thread.opened",
				EventVersion:  1,
				OccurredAt:    time.Now(),
				ThreadID:      threadID,
				RunID:         runID,
				Ticket:        ticket,
				AgentName:     agentName,
				ActorType:     model.ForumActorSystem,
				ActorName:     "metawsm",
				CorrelationID: generateForumID("fcorr"),
			},
			Title:    fmt.Sprintf("Control thread for %s in %s", agentName, runID),
			Body:     "System-managed control thread for forum-first run lifecycle signals.",
			Priority: model.ForumPriorityNormal,
		})
		if err != nil {
			return model.ForumThreadView{}, err
		}
		existing = open
	}
	if err := s.store.UpsertForumControlThread(model.ForumControlThread{
		RunID:     runID,
		AgentName: agentName,
		Ticket:    ticket,
		ThreadID:  threadID,
	}); err != nil {
		return model.ForumThreadView{}, err
	}
	if existing == nil {
		return model.ForumThreadView{}, fmt.Errorf("forum control thread unresolved")
	}
	return *existing, nil
}

func (s *Service) forumEmitDocsSyncRequestedEvent(
	thread model.ForumThreadView,
	actorType model.ForumActorType,
	actorName string,
	correlationID string,
	causationID string,
	trigger string,
) error {
	cfg, _, err := policy.Load("")
	if err != nil {
		cfg = policy.Default()
	}
	if !cfg.Forum.DocsSync.Enabled {
		return nil
	}
	return s.store.ForumAppendIntegrationEvent(model.ForumEnvelope{
		EventID:       generateForumID("fevt"),
		EventType:     "forum.integration.docs_sync.requested",
		EventVersion:  1,
		OccurredAt:    time.Now(),
		ThreadID:      thread.ThreadID,
		RunID:         thread.RunID,
		Ticket:        thread.Ticket,
		AgentName:     thread.AgentName,
		ActorType:     actorType,
		ActorName:     actorName,
		CorrelationID: correlationID,
		CausationID:   causationID,
	}, map[string]any{
		"trigger":       trigger,
		"thread_id":     thread.ThreadID,
		"title":         thread.Title,
		"state":         thread.State,
		"priority":      thread.Priority,
		"posts_count":   thread.PostsCount,
		"updated_at":    thread.UpdatedAt.Format(time.RFC3339),
		"assignee_type": thread.AssigneeType,
		"assignee_name": thread.AssigneeName,
	})
}
