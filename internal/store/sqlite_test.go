package store

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"metawsm/internal/model"
)

func TestSQLiteStoreRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	spec := model.RunSpec{
		RunID:             "run-test",
		Tickets:           []string{"METAWSM-001", "METAWSM-002"},
		Repos:             []string{"metawsm"},
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		DryRun:            true,
		CreatedAt:         time.Now(),
	}
	policyJSON, err := json.Marshal(map[string]any{"version": 1})
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	if err := s.CreateRun(spec, string(policyJSON)); err != nil {
		t.Fatalf("create run: %v", err)
	}

	steps := []model.PlanStep{
		{Index: 1, Name: "first", Kind: "shell", Command: "echo hi", Blocking: true, Status: model.StepStatusPending},
		{Index: 2, Name: "second", Kind: "shell", Command: "echo there", Blocking: true, Status: model.StepStatusPending},
	}
	if err := s.SaveSteps(spec.RunID, steps); err != nil {
		t.Fatalf("save steps: %v", err)
	}

	if err := s.UpdateRunStatus(spec.RunID, model.RunStatusPlanning, "planning"); err != nil {
		t.Fatalf("update run status: %v", err)
	}
	if err := s.UpdateStepStatus(spec.RunID, 1, model.StepStatusDone, "", true, true); err != nil {
		t.Fatalf("update step status: %v", err)
	}

	now := time.Now()
	agent := model.AgentRecord{
		RunID:          spec.RunID,
		Name:           "agent",
		WorkspaceName:  "metawsm-001",
		SessionName:    "agent-metawsm",
		Status:         model.AgentStatusRunning,
		HealthState:    model.HealthStateHealthy,
		LastActivityAt: &now,
		LastProgressAt: &now,
	}
	if err := s.UpsertAgent(agent); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}

	run, _, _, err := s.GetRun(spec.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != model.RunStatusPlanning {
		t.Fatalf("expected planning status, got %s", run.Status)
	}

	tickets, err := s.GetTickets(spec.RunID)
	if err != nil {
		t.Fatalf("get tickets: %v", err)
	}
	if len(tickets) != 2 {
		t.Fatalf("expected 2 tickets, got %d", len(tickets))
	}

	loadedSteps, err := s.GetSteps(spec.RunID)
	if err != nil {
		t.Fatalf("get steps: %v", err)
	}
	if len(loadedSteps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(loadedSteps))
	}
	if loadedSteps[0].Status != model.StepStatusDone {
		t.Fatalf("expected first step done, got %s", loadedSteps[0].Status)
	}

	agents, err := s.GetAgents(spec.RunID)
	if err != nil {
		t.Fatalf("get agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}
