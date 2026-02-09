package forumbus

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"

	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

type MessageHandler func(context.Context, model.ForumOutboxMessage) error

type Runtime struct {
	store         *store.SQLiteStore
	cfg           policy.Config
	mu            sync.RWMutex
	running       bool
	handlers      map[string]MessageHandler
	redisClient   redis.UniversalClient
	publisher     *redisstream.Publisher
	subscriber    *redisstream.Subscriber
	subscribeCtx  context.Context
	subscribeStop context.CancelFunc
	subscriptions map[string]<-chan *message.Message
	streamName    string
	groupName     string
	consumerName  string
}

func NewRuntime(sqliteStore *store.SQLiteStore, cfg policy.Config) *Runtime {
	streamName, groupName, consumerName := deriveStreamNamespace(
		strings.TrimSpace(cfg.Forum.Redis.Stream),
		strings.TrimSpace(cfg.Forum.Redis.Group),
		strings.TrimSpace(cfg.Forum.Redis.Consumer),
		strings.TrimSpace(sqliteStore.DBPath),
	)
	return &Runtime{
		store:         sqliteStore,
		cfg:           cfg,
		handlers:      make(map[string]MessageHandler),
		subscriptions: make(map[string]<-chan *message.Message),
		streamName:    streamName,
		groupName:     groupName,
		consumerName:  consumerName,
	}
}

func (r *Runtime) Start(ctx context.Context) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	redisURL := strings.TrimSpace(r.cfg.Forum.Redis.URL)
	if redisURL == "" {
		return fmt.Errorf("forum redis url is empty")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(options)
	logger := watermill.NopLogger{}
	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client:     client,
		Marshaller: redisstream.DefaultMarshallerUnmarshaller{},
	}, logger)
	if err != nil {
		return fmt.Errorf("create redisstream publisher: %w", err)
	}
	subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        client,
		Unmarshaller:  redisstream.DefaultMarshallerUnmarshaller{},
		ConsumerGroup: r.groupName,
		Consumer:      r.consumerName,
	}, logger)
	if err != nil {
		_ = publisher.Close()
		return fmt.Errorf("create redisstream subscriber: %w", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = subscriber.Close()
		_ = publisher.Close()
		return fmt.Errorf("redis ping failed: %w", err)
	}
	subCtx, cancel := context.WithCancel(context.Background())
	r.redisClient = client
	r.publisher = publisher
	r.subscriber = subscriber
	r.subscribeCtx = subCtx
	r.subscribeStop = cancel
	r.subscriptions = make(map[string]<-chan *message.Message)
	for topic := range r.handlers {
		if err := r.subscribeTopicLocked(topic); err != nil {
			cancel()
			_ = subscriber.Close()
			_ = publisher.Close()
			_ = client.Close()
			r.redisClient = nil
			r.publisher = nil
			r.subscriber = nil
			r.subscribeCtx = nil
			r.subscribeStop = nil
			r.subscriptions = make(map[string]<-chan *message.Message)
			return err
		}
	}
	r.running = true
	return nil
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return
	}
	r.running = false
	if r.subscribeStop != nil {
		r.subscribeStop()
	}
	if r.subscriber != nil {
		_ = r.subscriber.Close()
	}
	if r.publisher != nil {
		_ = r.publisher.Close()
	}
	if r.redisClient != nil {
		_ = r.redisClient.Close()
	}
	r.redisClient = nil
	r.publisher = nil
	r.subscriber = nil
	r.subscribeCtx = nil
	r.subscribeStop = nil
	r.subscriptions = make(map[string]<-chan *message.Message)
}

func (r *Runtime) Healthy() error {
	r.mu.RLock()
	running := r.running
	client := r.redisClient
	r.mu.RUnlock()
	if !running {
		return fmt.Errorf("forum bus runtime not started")
	}
	if strings.TrimSpace(r.cfg.Forum.Redis.URL) == "" {
		return fmt.Errorf("forum redis url is empty")
	}
	if client == nil {
		return fmt.Errorf("forum redis client is not configured")
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("forum redis ping failed: %w", err)
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
	if r.running {
		if err := r.subscribeTopicLocked(topic); err != nil {
			return err
		}
	}
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
	if limit <= 0 {
		limit = 20
	}
	handlers := r.snapshotHandlers()
	flushed, err := r.flushOutboxToRedis(limit, handlers)
	if err != nil {
		return flushed, err
	}
	consumed, err := r.consumeRedisMessages(ctx, limit, handlers)
	if err != nil {
		return flushed, err
	}
	return flushed + consumed, nil
}

func (r *Runtime) snapshotHandlers() map[string]MessageHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handlers := make(map[string]MessageHandler, len(r.handlers))
	for k, v := range r.handlers {
		handlers[k] = v
	}
	return handlers
}

func (r *Runtime) flushOutboxToRedis(limit int, handlers map[string]MessageHandler) (int, error) {
	batch, err := r.store.ClaimForumOutboxPending(limit)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}
	r.mu.RLock()
	publisher := r.publisher
	r.mu.RUnlock()
	if publisher == nil {
		return 0, fmt.Errorf("forum redis publisher is not configured")
	}
	sent := 0
	for _, msg := range batch {
		if handlers[msg.Topic] == nil {
			_ = r.store.MarkForumOutboxFailed(msg.MessageID, fmt.Sprintf("no handler for topic %s", msg.Topic))
			continue
		}
		wm := message.NewMessage(msg.MessageID, []byte(msg.PayloadJSON))
		wm.Metadata.Set("message_id", msg.MessageID)
		wm.Metadata.Set("topic", msg.Topic)
		wm.Metadata.Set("message_key", msg.MessageKey)
		err := publisher.Publish(r.streamTopic(msg.Topic), wm)
		if err != nil {
			_ = r.store.MarkForumOutboxFailed(msg.MessageID, err.Error())
			continue
		}
		if err := r.store.MarkForumOutboxSent(msg.MessageID); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (r *Runtime) consumeRedisMessages(ctx context.Context, limit int, handlers map[string]MessageHandler) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	if err := r.ensureSubscriptions(handlers); err != nil {
		return 0, err
	}
	r.mu.RLock()
	topics := make([]string, 0, len(r.subscriptions))
	for topic := range r.subscriptions {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	channels := make(map[string]<-chan *message.Message, len(r.subscriptions))
	for topic, ch := range r.subscriptions {
		channels[topic] = ch
	}
	r.mu.RUnlock()

	processed := 0
	deadline := time.Now().Add(300 * time.Millisecond)
	for processed < limit {
		progressed := false
		for _, topic := range topics {
			ch := channels[topic]
			if ch == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return processed, ctx.Err()
			case msg, ok := <-ch:
				if !ok || msg == nil {
					continue
				}
				progressed = true
				forumMsg := model.ForumOutboxMessage{
					MessageID:   strings.TrimSpace(msg.Metadata.Get("message_id")),
					Topic:       topic,
					MessageKey:  strings.TrimSpace(msg.Metadata.Get("message_key")),
					PayloadJSON: string(msg.Payload),
				}
				if forumMsg.MessageID == "" {
					forumMsg.MessageID = strings.TrimSpace(msg.UUID)
				}
				handler := handlers[topic]
				if handler == nil {
					msg.Ack()
				} else if err := handler(ctx, forumMsg); err != nil {
					_ = r.store.MarkForumOutboxFailed(forumMsg.MessageID, err.Error())
					msg.Nack()
				} else {
					msg.Ack()
				}
				processed++
				if processed >= limit {
					return processed, nil
				}
			default:
			}
		}
		if !progressed {
			if time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return processed, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	return processed, nil
}

func (r *Runtime) ensureSubscriptions(handlers map[string]MessageHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for topic := range handlers {
		if err := r.subscribeTopicLocked(topic); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) subscribeTopicLocked(topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("forum bus topic is required")
	}
	if _, exists := r.subscriptions[topic]; exists {
		return nil
	}
	if r.subscriber == nil || r.subscribeCtx == nil {
		if r.running {
			return fmt.Errorf("forum redis subscriber is not configured")
		}
		return nil
	}
	ch, err := r.subscriber.Subscribe(r.subscribeCtx, r.streamTopic(topic))
	if err != nil {
		return fmt.Errorf("subscribe to redis stream topic %s: %w", topic, err)
	}
	r.subscriptions[topic] = ch
	return nil
}

func (r *Runtime) streamTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return topic
	}
	prefix := strings.TrimSpace(r.streamName)
	if prefix == "" {
		return topic
	}
	return prefix + "." + topic
}

func deriveStreamNamespace(stream string, group string, consumer string, dbPath string) (string, string, string) {
	stream = strings.TrimSpace(stream)
	group = strings.TrimSpace(group)
	consumer = strings.TrimSpace(consumer)
	dbPath = strings.TrimSpace(dbPath)
	if stream == "" {
		stream = "metawsm-forum"
	}
	if group == "" {
		group = "metawsm-forum"
	}
	if consumer == "" {
		consumer = "operator"
	}
	if dbPath == "" {
		return stream, group, consumer
	}
	hash := sha1.Sum([]byte(dbPath))
	suffix := fmt.Sprintf("%x", hash[:4])
	return stream + "." + suffix, group + "." + suffix, consumer + "-" + suffix
}
