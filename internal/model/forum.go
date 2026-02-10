package model

import "time"

type ForumActorType string

const (
	ForumActorAgent    ForumActorType = "agent"
	ForumActorOperator ForumActorType = "operator"
	ForumActorHuman    ForumActorType = "human"
	ForumActorSystem   ForumActorType = "system"
)

type ForumViewerType string

const (
	ForumViewerHuman ForumViewerType = "human"
	ForumViewerAgent ForumViewerType = "agent"
)

type ForumQueueType string

const (
	ForumQueueUnseen     ForumQueueType = "unseen"
	ForumQueueUnanswered ForumQueueType = "unanswered"
)

type ForumThreadState string

const (
	ForumThreadStateNew             ForumThreadState = "new"
	ForumThreadStateTriaged         ForumThreadState = "triaged"
	ForumThreadStateWaitingOperator ForumThreadState = "waiting_operator"
	ForumThreadStateWaitingHuman    ForumThreadState = "waiting_human"
	ForumThreadStateAnswered        ForumThreadState = "answered"
	ForumThreadStateClosed          ForumThreadState = "closed"
)

type ForumPriority string

const (
	ForumPriorityLow    ForumPriority = "low"
	ForumPriorityNormal ForumPriority = "normal"
	ForumPriorityHigh   ForumPriority = "high"
	ForumPriorityUrgent ForumPriority = "urgent"
)

type ForumEnvelope struct {
	EventID       string         `json:"event_id"`
	EventType     string         `json:"event_type"`
	EventVersion  int            `json:"event_version"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ThreadID      string         `json:"thread_id"`
	RunID         string         `json:"run_id,omitempty"`
	Ticket        string         `json:"ticket"`
	AgentName     string         `json:"agent_name,omitempty"`
	ActorType     ForumActorType `json:"actor_type"`
	ActorName     string         `json:"actor_name,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	CausationID   string         `json:"causation_id,omitempty"`
}

type ForumTopicRegistry struct {
	CommandPrefix     string `json:"command_prefix"`
	EventPrefix       string `json:"event_prefix"`
	IntegrationPrefix string `json:"integration_prefix"`
}

func DefaultForumTopicRegistry() ForumTopicRegistry {
	return ForumTopicRegistry{
		CommandPrefix:     "forum.commands",
		EventPrefix:       "forum.events",
		IntegrationPrefix: "forum.integration",
	}
}

func (r ForumTopicRegistry) CommandTopic(name string) string {
	return r.CommandPrefix + "." + name
}

func (r ForumTopicRegistry) EventTopic(name string) string {
	return r.EventPrefix + "." + name
}

func (r ForumTopicRegistry) IntegrationTopic(name string) string {
	return r.IntegrationPrefix + "." + name
}

type ForumOpenThreadCommand struct {
	Envelope ForumEnvelope `json:"envelope"`
	Title    string        `json:"title"`
	Body     string        `json:"body"`
	Priority ForumPriority `json:"priority"`
}

type ForumAddPostCommand struct {
	Envelope ForumEnvelope `json:"envelope"`
	Body     string        `json:"body"`
}

type ForumAssignThreadCommand struct {
	Envelope       ForumEnvelope  `json:"envelope"`
	AssigneeType   ForumActorType `json:"assignee_type"`
	AssigneeName   string         `json:"assignee_name,omitempty"`
	AssignmentNote string         `json:"assignment_note,omitempty"`
}

type ForumChangeStateCommand struct {
	Envelope ForumEnvelope    `json:"envelope"`
	ToState  ForumThreadState `json:"to_state"`
}

type ForumSetPriorityCommand struct {
	Envelope ForumEnvelope `json:"envelope"`
	Priority ForumPriority `json:"priority"`
}

type ForumCloseThreadCommand struct {
	Envelope ForumEnvelope `json:"envelope"`
}

type ForumThreadView struct {
	ThreadID          string           `json:"thread_id"`
	Ticket            string           `json:"ticket"`
	RunID             string           `json:"run_id,omitempty"`
	AgentName         string           `json:"agent_name,omitempty"`
	Title             string           `json:"title"`
	State             ForumThreadState `json:"state"`
	Priority          ForumPriority    `json:"priority"`
	AssigneeType      ForumActorType   `json:"assignee_type,omitempty"`
	AssigneeName      string           `json:"assignee_name,omitempty"`
	OpenedByType      ForumActorType   `json:"opened_by_type"`
	OpenedByName      string           `json:"opened_by_name,omitempty"`
	PostsCount        int              `json:"posts_count"`
	LastPostAt        *time.Time       `json:"last_post_at,omitempty"`
	LastPostByType    ForumActorType   `json:"last_post_by_type,omitempty"`
	LastPostByName    string           `json:"last_post_by_name,omitempty"`
	LastEventSequence int64            `json:"last_event_sequence,omitempty"`
	LastActorType     ForumActorType   `json:"last_actor_type,omitempty"`
	IsUnseen          bool             `json:"is_unseen,omitempty"`
	IsUnanswered      bool             `json:"is_unanswered,omitempty"`
	OpenedAt          time.Time        `json:"opened_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	ClosedAt          *time.Time       `json:"closed_at,omitempty"`
}

type ForumPost struct {
	PostID     string         `json:"post_id"`
	ThreadID   string         `json:"thread_id"`
	EventID    string         `json:"event_id"`
	AuthorType ForumActorType `json:"author_type"`
	AuthorName string         `json:"author_name,omitempty"`
	Body       string         `json:"body"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ForumThreadStats struct {
	Ticket      string           `json:"ticket"`
	RunID       string           `json:"run_id,omitempty"`
	State       ForumThreadState `json:"state"`
	Priority    ForumPriority    `json:"priority"`
	ThreadCount int              `json:"thread_count"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type ForumEvent struct {
	Sequence    int64         `json:"sequence"`
	Envelope    ForumEnvelope `json:"envelope"`
	PayloadJSON string        `json:"payload_json,omitempty"`
}

type ForumThreadFilter struct {
	Ticket   string
	RunID    string
	State    ForumThreadState
	Priority ForumPriority
	Assignee string
	Limit    int
}

type ForumThreadSearchFilter struct {
	Query      string
	Ticket     string
	RunID      string
	State      ForumThreadState
	Priority   ForumPriority
	Assignee   string
	ViewerType ForumViewerType
	ViewerID   string
	Limit      int
	Cursor     int64
}

type ForumQueueFilter struct {
	QueueType  ForumQueueType
	Ticket     string
	RunID      string
	State      ForumThreadState
	Priority   ForumPriority
	Assignee   string
	ViewerType ForumViewerType
	ViewerID   string
	Limit      int
	Cursor     int64
}

type ForumThreadSeen struct {
	ThreadID              string          `json:"thread_id"`
	ViewerType            ForumViewerType `json:"viewer_type"`
	ViewerID              string          `json:"viewer_id"`
	LastSeenEventSequence int64           `json:"last_seen_event_sequence"`
	UpdatedAt             time.Time       `json:"updated_at"`
}
