package server

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/serviceapi"
)

type ForumWorkerSnapshot struct {
	Running           bool                   `json:"running"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	LastTickAt        *time.Time             `json:"last_tick_at,omitempty"`
	LastProcessedAt   *time.Time             `json:"last_processed_at,omitempty"`
	LastErrorAt       *time.Time             `json:"last_error_at,omitempty"`
	LastError         string                 `json:"last_error,omitempty"`
	ConsecutiveErrors int                    `json:"consecutive_errors"`
	TotalProcessed    int64                  `json:"total_processed"`
	TotalBatches      int64                  `json:"total_batches"`
	IdleBatches       int64                  `json:"idle_batches"`
	BusHealthy        bool                   `json:"bus_healthy"`
	BusError          string                 `json:"bus_error,omitempty"`
	Outbox            model.ForumOutboxStats `json:"outbox"`
}

type ForumWorker struct {
	service     serviceapi.Core
	interval    time.Duration
	batchSize   int
	logInterval time.Duration
	logger      *log.Logger

	mu       sync.RWMutex
	running  bool
	started  *time.Time
	doneChan chan struct{}
	snapshot ForumWorkerSnapshot
}

func NewForumWorker(service serviceapi.Core, interval time.Duration, batchSize int, logInterval time.Duration, logger *log.Logger) *ForumWorker {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if logInterval <= 0 {
		logInterval = 15 * time.Second
	}
	return &ForumWorker{
		service:     service,
		interval:    interval,
		batchSize:   batchSize,
		logInterval: logInterval,
		logger:      logger,
		snapshot: ForumWorkerSnapshot{
			BusHealthy: true,
			Outbox:     model.ForumOutboxStats{},
		},
	}
}

func (w *ForumWorker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	now := time.Now().UTC()
	w.started = &now
	w.snapshot.Running = true
	w.snapshot.StartedAt = timePtr(now)
	w.doneChan = make(chan struct{})
	done := w.doneChan
	w.mu.Unlock()

	go func() {
		defer close(done)
		w.loop(ctx)
		w.mu.Lock()
		w.running = false
		w.snapshot.Running = false
		w.mu.Unlock()
	}()
}

func (w *ForumWorker) Wait(timeout time.Duration) bool {
	w.mu.RLock()
	done := w.doneChan
	w.mu.RUnlock()
	if done == nil {
		return true
	}
	if timeout <= 0 {
		<-done
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (w *ForumWorker) Snapshot() ForumWorkerSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	copySnapshot := w.snapshot
	copySnapshot.StartedAt = cloneTimePtr(w.snapshot.StartedAt)
	copySnapshot.LastTickAt = cloneTimePtr(w.snapshot.LastTickAt)
	copySnapshot.LastProcessedAt = cloneTimePtr(w.snapshot.LastProcessedAt)
	copySnapshot.LastErrorAt = cloneTimePtr(w.snapshot.LastErrorAt)
	copySnapshot.Outbox = w.snapshot.Outbox
	return copySnapshot
}

func (w *ForumWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	logTicker := time.NewTicker(w.logInterval)
	defer logTicker.Stop()

	w.runIteration(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runIteration(ctx)
		case <-logTicker.C:
			w.logSnapshot()
		}
	}
}

func (w *ForumWorker) runIteration(ctx context.Context) {
	if w.service == nil {
		return
	}
	now := time.Now().UTC()

	processed, processErr := w.service.ProcessForumBusOnce(ctx, w.batchSize)
	if processErr != nil && ctx.Err() != nil {
		return
	}
	busErr := w.service.ForumBusHealth()
	outbox, outboxErr := w.service.ForumOutboxStats()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.snapshot.LastTickAt = timePtr(now)
	w.snapshot.TotalBatches++
	if processed > 0 {
		w.snapshot.TotalProcessed += int64(processed)
		w.snapshot.LastProcessedAt = timePtr(now)
	} else {
		w.snapshot.IdleBatches++
	}

	switch {
	case processErr != nil:
		w.snapshot.ConsecutiveErrors++
		w.snapshot.LastErrorAt = timePtr(now)
		w.snapshot.LastError = strings.TrimSpace(processErr.Error())
	case outboxErr != nil:
		w.snapshot.ConsecutiveErrors++
		w.snapshot.LastErrorAt = timePtr(now)
		w.snapshot.LastError = strings.TrimSpace(outboxErr.Error())
	default:
		w.snapshot.ConsecutiveErrors = 0
	}
	if busErr != nil {
		w.snapshot.BusHealthy = false
		w.snapshot.BusError = strings.TrimSpace(busErr.Error())
	} else {
		w.snapshot.BusHealthy = true
		w.snapshot.BusError = ""
	}
	if outboxErr == nil {
		w.snapshot.Outbox = outbox
	}
}

func (w *ForumWorker) logSnapshot() {
	if w.logger == nil {
		return
	}
	snapshot := w.Snapshot()
	w.logger.Printf(
		"forum worker: bus_healthy=%t pending=%d processing=%d failed=%d oldest_pending_age=%ds total_processed=%d errors=%d",
		snapshot.BusHealthy,
		snapshot.Outbox.PendingCount,
		snapshot.Outbox.ProcessingCount,
		snapshot.Outbox.FailedCount,
		snapshot.Outbox.OldestPendingAgeSec,
		snapshot.TotalProcessed,
		snapshot.ConsecutiveErrors,
	)
}

func timePtr(value time.Time) *time.Time {
	clone := value
	return &clone
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
