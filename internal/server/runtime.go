package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/serviceapi"
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
	opts      Options
	service   serviceapi.Core
	worker    *ForumWorker
	startedAt time.Time
	server    *http.Server
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
		opts:      options,
		service:   service,
		worker:    NewForumWorker(service, options.WorkerInterval, options.WorkerBatchSize, options.WorkerLogPeriod, logger),
		startedAt: time.Now().UTC(),
	}
	mux := http.NewServeMux()
	runtime.registerRoutes(mux)
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
			r.service.Shutdown()
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.opts.ShutdownTimeout)
	defer cancel()
	if err := r.server.Shutdown(shutdownCtx); err != nil {
		workerCancel()
		_ = r.worker.Wait(2 * time.Second)
		r.service.Shutdown()
		return err
	}
	workerCancel()
	_ = r.worker.Wait(2 * time.Second)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
