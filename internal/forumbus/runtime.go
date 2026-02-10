package forumbus

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
type MessageObserver func(topic string, message model.ForumOutboxMessage)

type runtimeObserver struct {
	id          int64
	topicPrefix string
	observer    MessageObserver
}

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
	observers     map[int64]runtimeObserver
	nextObserver  int64
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
		observers:     make(map[int64]runtimeObserver),
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

func (r *Runtime) DebugSnapshot(ctx context.Context, topics []string) model.ForumBusDebug {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.RLock()
	running := r.running
	client := r.redisClient
	streamName := r.streamName
	groupName := r.groupName
	consumerName := r.consumerName
	redisURL := sanitizeRedisURL(strings.TrimSpace(r.cfg.Forum.Redis.URL))
	handlerTopics := sortedMapKeys(r.handlers)
	subscriptionTopics := sortedMapKeys(r.subscriptions)
	r.mu.RUnlock()

	debug := model.ForumBusDebug{
		Running:            running,
		Healthy:            true,
		RedisURL:           redisURL,
		StreamName:         streamName,
		ConsumerGroup:      groupName,
		ConsumerName:       consumerName,
		HandlerTopics:      handlerTopics,
		SubscriptionTopics: subscriptionTopics,
		Topics:             []model.ForumBusTopicDebug{},
	}
	if err := r.Healthy(); err != nil {
		debug.Healthy = false
		debug.HealthError = strings.TrimSpace(err.Error())
	}

	topicSet := map[string]struct{}{}
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic != "" {
			topicSet[topic] = struct{}{}
		}
	}
	for _, topic := range handlerTopics {
		topicSet[topic] = struct{}{}
	}
	for _, topic := range subscriptionTopics {
		topicSet[topic] = struct{}{}
	}
	allTopics := make([]string, 0, len(topicSet))
	for topic := range topicSet {
		allTopics = append(allTopics, topic)
	}
	sort.Strings(allTopics)

	for _, topic := range allTopics {
		item := model.ForumBusTopicDebug{
			Topic:             topic,
			Stream:            streamTopicForPrefix(streamName, topic),
			HandlerRegistered: containsString(handlerTopics, topic),
			Subscribed:        containsString(subscriptionTopics, topic),
		}
		if !running || client == nil {
			debug.Topics = append(debug.Topics, item)
			continue
		}

		streamInfo, err := client.XInfoStream(ctx, item.Stream).Result()
		if err != nil {
			if !isRedisNoStreamError(err) {
				item.TopicError = appendTopicError(item.TopicError, err.Error())
			}
		} else {
			item.StreamExists = true
			item.StreamLength = streamInfo.Length
			item.LastGeneratedID = strings.TrimSpace(streamInfo.LastGeneratedID)
		}

		groups, err := client.XInfoGroups(ctx, item.Stream).Result()
		if err != nil {
			if !isRedisNoStreamError(err) {
				item.TopicError = appendTopicError(item.TopicError, err.Error())
			}
		} else {
			for _, group := range groups {
				if strings.TrimSpace(group.Name) != groupName {
					continue
				}
				item.ConsumerGroupPresent = true
				item.ConsumerGroupPending = group.Pending
				item.ConsumerGroupLag = group.Lag
				break
			}
		}
		debug.Topics = append(debug.Topics, item)
	}
	return debug
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

func (r *Runtime) RegisterObserver(topicPrefix string, observer MessageObserver) (func(), error) {
	topicPrefix = strings.TrimSpace(topicPrefix)
	if observer == nil {
		return nil, fmt.Errorf("forum bus observer is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextObserver++
	entry := runtimeObserver{
		id:          r.nextObserver,
		topicPrefix: topicPrefix,
		observer:    observer,
	}
	r.observers[entry.id] = entry
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.observers, entry.id)
	}, nil
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
					r.notifyObservers(topic, forumMsg)
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

func (r *Runtime) notifyObservers(topic string, message model.ForumOutboxMessage) {
	observers := r.matchingObservers(topic)
	for _, observer := range observers {
		func(cb MessageObserver) {
			defer func() {
				_ = recover()
			}()
			cb(topic, message)
		}(observer)
	}
}

func (r *Runtime) matchingObservers(topic string) []MessageObserver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MessageObserver, 0, len(r.observers))
	for _, entry := range r.observers {
		prefix := strings.TrimSpace(entry.topicPrefix)
		if prefix != "" && !strings.HasPrefix(topic, prefix) {
			continue
		}
		if entry.observer == nil {
			continue
		}
		out = append(out, entry.observer)
	}
	return out
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

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsString(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func sanitizeRedisURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.User != nil {
		if username := parsed.User.Username(); username != "" {
			parsed.User = url.UserPassword(username, "***")
		} else {
			parsed.User = url.User("***")
		}
	}
	return parsed.String()
}

func isRedisNoStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, redis.Nil) {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(value, "no such key")
}

func streamTopicForPrefix(prefix string, topic string) string {
	prefix = strings.TrimSpace(prefix)
	topic = strings.TrimSpace(topic)
	if prefix == "" {
		return topic
	}
	if topic == "" {
		return prefix
	}
	return prefix + "." + topic
}

func appendTopicError(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	if strings.Contains(existing, next) {
		return existing
	}
	return existing + "; " + next
}
