package forumbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

type MessageHandler func(context.Context, model.ForumOutboxMessage) error

type Runtime struct {
	store    *store.SQLiteStore
	cfg      policy.Config
	mu       sync.RWMutex
	running  bool
	handlers map[string]MessageHandler
}

func NewRuntime(sqliteStore *store.SQLiteStore, cfg policy.Config) *Runtime {
	return &Runtime{
		store:    sqliteStore,
		cfg:      cfg,
		handlers: make(map[string]MessageHandler),
	}
}

func (r *Runtime) Start(ctx context.Context) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	r.running = true
	return nil
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
}

func (r *Runtime) Healthy() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.running {
		return fmt.Errorf("forum bus runtime not started")
	}
	if strings.TrimSpace(r.cfg.Forum.Redis.URL) == "" {
		return fmt.Errorf("forum redis url is empty")
	}
	return nil
}

func (r *Runtime) RegisterHandler(topic string, handler MessageHandler) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("forum bus topic is required")
	}
	if handler == nil {
		return fmt.Errorf("forum bus handler is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[topic] = handler
	return nil
}

func (r *Runtime) Publish(topic string, messageKey string, payload any) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", fmt.Errorf("forum bus publish topic is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal forum bus payload: %w", err)
	}
	messageID := fmt.Sprintf("fmsg-%d", time.Now().UnixNano())
	if err := r.store.EnqueueForumOutbox(model.ForumOutboxMessage{
		MessageID:   messageID,
		Topic:       topic,
		MessageKey:  strings.TrimSpace(messageKey),
		PayloadJSON: string(encoded),
		Status:      model.ForumOutboxStatusPending,
	}); err != nil {
		return "", err
	}
	return messageID, nil
}

func (r *Runtime) ProcessOnce(ctx context.Context, limit int) (int, error) {
	if err := r.Healthy(); err != nil {
		return 0, err
	}
	batch, err := r.store.ClaimForumOutboxPending(limit)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}

	r.mu.RLock()
	handlers := make(map[string]MessageHandler, len(r.handlers))
	for k, v := range r.handlers {
		handlers[k] = v
	}
	r.mu.RUnlock()

	for _, msg := range batch {
		handler := handlers[msg.Topic]
		if handler == nil {
			_ = r.store.MarkForumOutboxFailed(msg.MessageID, fmt.Sprintf("no handler for topic %s", msg.Topic))
			continue
		}
		if err := handler(ctx, msg); err != nil {
			_ = r.store.MarkForumOutboxFailed(msg.MessageID, err.Error())
			continue
		}
		if err := r.store.MarkForumOutboxSent(msg.MessageID); err != nil {
			return 0, err
		}
	}
	return len(batch), nil
}
