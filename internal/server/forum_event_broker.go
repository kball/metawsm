package server

import (
	"strings"
	"sync"

	"metawsm/internal/model"
)

type forumEventSubscriber struct {
	id     int64
	ticket string
	runID  string
	ch     chan model.ForumEvent
}

type ForumEventBroker struct {
	mu          sync.RWMutex
	closed      bool
	nextID      int64
	bufferSize  int
	subscribers map[int64]forumEventSubscriber
}

func NewForumEventBroker(bufferSize int) *ForumEventBroker {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &ForumEventBroker{
		bufferSize:  bufferSize,
		subscribers: make(map[int64]forumEventSubscriber),
	}
}

func (b *ForumEventBroker) Subscribe(ticket string, runID string) (<-chan model.ForumEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan model.ForumEvent, b.bufferSize)
	if b.closed {
		close(ch)
		return ch, func() {}
	}

	b.nextID++
	subscriber := forumEventSubscriber{
		id:     b.nextID,
		ticket: strings.TrimSpace(ticket),
		runID:  strings.TrimSpace(runID),
		ch:     ch,
	}
	b.subscribers[subscriber.id] = subscriber
	return ch, func() {
		b.unsubscribe(subscriber.id)
	}
}

func (b *ForumEventBroker) Publish(event model.ForumEvent) int {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return 0
	}
	snapshot := make([]forumEventSubscriber, 0, len(b.subscribers))
	for _, subscriber := range b.subscribers {
		snapshot = append(snapshot, subscriber)
	}
	b.mu.RUnlock()

	delivered := 0
	for _, subscriber := range snapshot {
		if !matchesEventFilter(subscriber, event) {
			continue
		}
		if tryPublishEvent(subscriber.ch, event) {
			delivered++
		}
	}
	return delivered
}

func (b *ForumEventBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, subscriber := range b.subscribers {
		close(subscriber.ch)
		delete(b.subscribers, id)
	}
}

func (b *ForumEventBroker) unsubscribe(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subscriber, ok := b.subscribers[id]
	if !ok {
		return
	}
	delete(b.subscribers, id)
	close(subscriber.ch)
}

func matchesEventFilter(subscriber forumEventSubscriber, event model.ForumEvent) bool {
	if subscriber.ticket != "" && !strings.EqualFold(strings.TrimSpace(event.Envelope.Ticket), subscriber.ticket) {
		return false
	}
	if subscriber.runID != "" && !strings.EqualFold(strings.TrimSpace(event.Envelope.RunID), subscriber.runID) {
		return false
	}
	return true
}

func tryPublishEvent(ch chan model.ForumEvent, event model.ForumEvent) bool {
	select {
	case ch <- event:
		return true
	default:
		// Drop one stale message and retry once to avoid blocking broker fanout.
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- event:
			return true
		default:
			return false
		}
	}
}
