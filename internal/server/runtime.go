package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/serviceapi"
	"metawsm/internal/web"
)

type Options struct {
	Addr            string
	DBPath          string
	WorkerInterval  time.Duration
	WorkerBatchSize int
	WorkerLogPeriod time.Duration
	ShutdownTimeout time.Duration
}

type Runtime struct {
	opts          Options
	service       serviceapi.Core
	worker        *ForumWorker
	startedAt     time.Time
	server        *http.Server
	eventBroker   *ForumEventBroker
	stopEventPump func()
}

type HealthResponse struct {
	Status    string                 `json:"status"`
	StartedAt time.Time              `json:"started_at"`
	Now       time.Time              `json:"now"`
	Worker    ForumWorkerSnapshot    `json:"worker"`
	Outbox    model.ForumOutboxStats `json:"outbox"`
	ForumBus  HealthBusStatus        `json:"forum_bus"`
}

type HealthBusStatus struct {
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

func NewRuntime(options Options) (*Runtime, error) {
	options = normalizeOptions(options)
	service, err := serviceapi.NewLocalCore(options.DBPath)
	if err != nil {
		return nil, err
	}
	logger := log.New(os.Stdout, "", 0)
	runtime := &Runtime{
		opts:        options,
		service:     service,
		worker:      NewForumWorker(service, options.WorkerInterval, options.WorkerBatchSize, options.WorkerLogPeriod, logger),
		startedAt:   time.Now().UTC(),
		eventBroker: NewForumEventBroker(128),
	}
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)
	spaMounted := web.RegisterSPA(mux, web.PublicFS, web.SPAOptions{
		APIPrefix: "/api",
		WSPath:    "/api/v1/forum/stream",
	})
	if !spaMounted {
		mux.HandleFunc("/", runtime.handleSPAFallback)
	}
	runtime.server = &http.Server{
		Addr:    options.Addr,
		Handler: mux,
	}
	return runtime, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	r.worker.Start(workerCtx)
	r.startEventPump()

	errCh := make(chan error, 1)
	go func() {
		if err := r.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			workerCancel()
			_ = r.worker.Wait(2 * time.Second)
			r.stopForumEventPump()
			r.service.Shutdown()
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.opts.ShutdownTimeout)
	defer cancel()
	if err := r.server.Shutdown(shutdownCtx); err != nil {
		workerCancel()
		_ = r.worker.Wait(2 * time.Second)
		r.stopForumEventPump()
		r.service.Shutdown()
		return err
	}
	workerCancel()
	_ = r.worker.Wait(2 * time.Second)
	r.stopForumEventPump()
	r.service.Shutdown()
	return nil
}

func normalizeOptions(options Options) Options {
	if options.Addr == "" {
		options.Addr = ":3001"
	}
	if options.DBPath == "" {
		options.DBPath = ".metawsm/metawsm.db"
	}
	if options.WorkerInterval <= 0 {
		options.WorkerInterval = 500 * time.Millisecond
	}
	if options.WorkerBatchSize <= 0 {
		options.WorkerBatchSize = 100
	}
	if options.WorkerLogPeriod <= 0 {
		options.WorkerLogPeriod = 15 * time.Second
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 5 * time.Second
	}
	return options
}

func (r *Runtime) startEventPump() {
	if r == nil || r.service == nil {
		return
	}
	subscriber, ok := r.service.(serviceapi.LiveForumEventSubscriber)
	if !ok {
		return
	}
	stop, err := subscriber.SubscribeForumEvents(func(event model.ForumEvent) {
		if r.eventBroker == nil {
			return
		}
		r.eventBroker.Publish(event)
	})
	if err != nil {
		return
	}
	r.stopEventPump = stop
}

func (r *Runtime) stopForumEventPump() {
	if r == nil {
		return
	}
	if r.stopEventPump != nil {
		r.stopEventPump()
		r.stopEventPump = nil
	}
	if r.eventBroker != nil {
		r.eventBroker.Close()
	}
}

func (r *Runtime) handleHealth(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC()
	busErr := r.service.ForumBusHealth()
	bus := HealthBusStatus{Healthy: busErr == nil}
	if busErr != nil {
		bus.Error = busErr.Error()
	}
	outboxStats, outboxErr := r.service.ForumOutboxStats()
	if outboxErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"error":  outboxErr.Error(),
		})
		return
	}

	response := HealthResponse{
		Status:    "ok",
		StartedAt: r.startedAt,
		Now:       now,
		Worker:    r.worker.Snapshot(),
		Outbox:    outboxStats,
		ForumBus:  bus,
	}
	statusCode := http.StatusOK
	if !bus.Healthy {
		response.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(w, statusCode, response)
}

func (r *Runtime) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error": map[string]string{
			"code":    "not_found",
			"message": "route not found",
		},
	})
}

func (r *Runtime) handleSPAFallback(w http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/api") {
		r.handleNotFound(w, req)
		return
	}
	http.Error(w, "web ui assets are unavailable; run `go generate ./internal/web` for dev or build with `-tags embed`", http.StatusServiceUnavailable)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
