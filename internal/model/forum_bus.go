package model

import "time"

type ForumOutboxStatus string

const (
	ForumOutboxStatusPending    ForumOutboxStatus = "pending"
	ForumOutboxStatusProcessing ForumOutboxStatus = "processing"
	ForumOutboxStatusSent       ForumOutboxStatus = "sent"
	ForumOutboxStatusFailed     ForumOutboxStatus = "failed"
)

type ForumOutboxMessage struct {
	ID           int64             `json:"id"`
	MessageID    string            `json:"message_id"`
	Topic        string            `json:"topic"`
	MessageKey   string            `json:"message_key,omitempty"`
	PayloadJSON  string            `json:"payload_json"`
	Status       ForumOutboxStatus `json:"status"`
	AttemptCount int               `json:"attempt_count"`
	LastError    string            `json:"last_error,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	SentAt       *time.Time        `json:"sent_at,omitempty"`
}

type ForumOutboxStats struct {
	PendingCount        int        `json:"pending_count"`
	ProcessingCount     int        `json:"processing_count"`
	FailedCount         int        `json:"failed_count"`
	OldestPendingAt     *time.Time `json:"oldest_pending_at,omitempty"`
	OldestPendingAgeSec int64      `json:"oldest_pending_age_seconds"`
}

type ForumBusTopicDebug struct {
	Topic                string `json:"topic"`
	Stream               string `json:"stream"`
	HandlerRegistered    bool   `json:"handler_registered"`
	Subscribed           bool   `json:"subscribed"`
	StreamExists         bool   `json:"stream_exists"`
	StreamLength         int64  `json:"stream_length"`
	LastGeneratedID      string `json:"last_generated_id,omitempty"`
	ConsumerGroupPresent bool   `json:"consumer_group_present"`
	ConsumerGroupPending int64  `json:"consumer_group_pending"`
	ConsumerGroupLag     int64  `json:"consumer_group_lag"`
	TopicError           string `json:"topic_error,omitempty"`
}

type ForumBusDebug struct {
	Running            bool                 `json:"running"`
	Healthy            bool                 `json:"healthy"`
	HealthError        string               `json:"health_error,omitempty"`
	RedisURL           string               `json:"redis_url,omitempty"`
	StreamName         string               `json:"stream_name"`
	ConsumerGroup      string               `json:"consumer_group"`
	ConsumerName       string               `json:"consumer_name"`
	HandlerTopics      []string             `json:"handler_topics"`
	SubscriptionTopics []string             `json:"subscription_topics"`
	Topics             []ForumBusTopicDebug `json:"topics"`
}

type ForumStreamDebugSnapshot struct {
	GeneratedAt    time.Time            `json:"generated_at"`
	Ticket         string               `json:"ticket,omitempty"`
	RunID          string               `json:"run_id,omitempty"`
	Outbox         ForumOutboxStats     `json:"outbox"`
	OutboxMessages []ForumOutboxMessage `json:"outbox_messages"`
	Events         []ForumEvent         `json:"events"`
	Threads        []ForumThreadView    `json:"threads"`
	ControlThreads []ForumControlThread `json:"control_threads"`
	Bus            ForumBusDebug        `json:"bus"`
}
