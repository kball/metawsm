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
