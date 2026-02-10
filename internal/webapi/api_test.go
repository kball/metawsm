package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/store"
)

func TestRunsEndpoints(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	seedFixtureRun(t, dbPath)

	api, err := New(dbPath)
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	mux := http.NewServeMux()
	api.Register(mux)

	t.Run("list runs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var payload struct {
			Runs []runSummary `json:"runs"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(payload.Runs) != 1 {
			t.Fatalf("expected 1 run, got %d", len(payload.Runs))
		}
		if payload.Runs[0].RunID != "run-web-1" {
			t.Fatalf("expected run-web-1, got %q", payload.Runs[0].RunID)
		}
		if payload.Runs[0].StepCounts.Total != 2 {
			t.Fatalf("expected 2 steps, got %d", payload.Runs[0].StepCounts.Total)
		}
	})

	t.Run("get run detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-web-1", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var payload runDetail
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload.Run.RunID != "run-web-1" {
			t.Fatalf("expected run-web-1, got %q", payload.Run.RunID)
		}
		if len(payload.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(payload.Steps))
		}
		if payload.Brief == nil || payload.Brief.DoneCriteria != "go test ./... -count=1" {
			t.Fatalf("expected brief with done criteria")
		}
	})

	t.Run("run not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/does-not-exist", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

func seedFixtureRun(t *testing.T, dbPath string) {
	t.Helper()
	s := store.NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	spec := model.RunSpec{
		RunID:             "run-web-1",
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-006"},
		Repos:             []string{"metawsm"},
		DocHomeRepo:       "metawsm",
		BaseBranch:        "main",
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		PolicyPath:        ".metawsm/policy.json",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.CreateRun(spec, "{}"); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.UpdateRunStatus("run-web-1", model.RunStatusRunning, ""); err != nil {
		t.Fatalf("update run status: %v", err)
	}
	if err := s.SaveSteps("run-web-1", []model.PlanStep{
		{Index: 1, Name: "step-1", Kind: "shell", Command: "echo one", Blocking: true, Status: model.StepStatusDone},
		{Index: 2, Name: "step-2", Kind: "shell", Command: "echo two", Blocking: true, Status: model.StepStatusRunning},
	}); err != nil {
		t.Fatalf("save steps: %v", err)
	}
	if err := s.UpsertAgent(model.AgentRecord{
		RunID:         "run-web-1",
		Name:          "agent",
		WorkspaceName: "ws-web-1",
		SessionName:   "agent-ws-web-1",
		Status:        model.AgentStatusRunning,
		HealthState:   model.HealthStateHealthy,
	}); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}
	if err := s.UpsertRunBrief(model.RunBrief{
		RunID:        "run-web-1",
		Ticket:       "METAWSM-006",
		Goal:         "Build web stack",
		Scope:        "cmd/metawsm + internal/web + ui",
		DoneCriteria: "go test ./... -count=1",
		Constraints:  "Use existing tooling",
		MergeIntent:  "default",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert run brief: %v", err)
	}
}
