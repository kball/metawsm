package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"metawsm/internal/model"
)

func TestRunDryRunPersistsPlan(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.Run(t.Context(), RunOptions{
		Tickets:           []string{"METAWSM-001", "METAWSM-002"},
		Repos:             []string{"metawsm"},
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		DryRun:            true,
		Mode:              model.RunModeBootstrap,
		RunBrief: &model.RunBrief{
			Ticket:       "METAWSM-001",
			Goal:         "Bootstrap run",
			Scope:        "orchestrator",
			DoneCriteria: "tests pass",
			Constraints:  "keep existing flow",
			MergeIntent:  "default",
			QA: []model.IntakeQA{
				{Question: "Goal?", Answer: "Bootstrap run"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	if result.RunID == "" {
		t.Fatalf("expected run id")
	}
	if len(result.Steps) == 0 {
		t.Fatalf("expected non-empty plan")
	}

	record, _, _, err := svc.store.GetRun(result.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if record.Status != model.RunStatusPaused {
		t.Fatalf("expected paused run status after dry-run, got %s", record.Status)
	}

	steps, err := svc.store.GetSteps(result.RunID)
	if err != nil {
		t.Fatalf("get steps: %v", err)
	}
	if len(steps) < 4 {
		t.Fatalf("expected >= 4 steps for two tickets, got %d", len(steps))
	}

	brief, err := svc.store.GetRunBrief(result.RunID)
	if err != nil {
		t.Fatalf("get run brief: %v", err)
	}
	if brief == nil {
		t.Fatalf("expected run brief")
	}
	if brief.Goal != "Bootstrap run" {
		t.Fatalf("expected run brief goal to persist")
	}

	activeRuns, err := svc.ActiveRuns()
	if err != nil {
		t.Fatalf("active runs: %v", err)
	}
	if len(activeRuns) != 1 {
		t.Fatalf("expected one active run, got %d", len(activeRuns))
	}
	if activeRuns[0].RunID != result.RunID {
		t.Fatalf("expected active run id %s, got %s", result.RunID, activeRuns[0].RunID)
	}
}

func TestGuideAnswersPendingRequest(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	spec := model.RunSpec{
		RunID:             "run-guide",
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-002"},
		Repos:             []string{"metawsm"},
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, `{"version":1}`); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := svc.store.UpdateRunStatus(spec.RunID, model.RunStatusRunning, ""); err != nil {
		t.Fatalf("set running status: %v", err)
	}
	if err := svc.transitionRun(spec.RunID, model.RunStatusRunning, model.RunStatusAwaitingGuidance, "test awaiting"); err != nil {
		t.Fatalf("transition to awaiting: %v", err)
	}

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, ".metawsm", "guidance-request.json"), []byte(`{"run_id":"run-guide","agent":"agent","question":"Need API decision?"}`), 0o644); err != nil {
		t.Fatalf("write guidance request file: %v", err)
	}

	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	workspaceConfigDir := filepath.Join(homeDir, "Library", "Application Support", "workspace-manager", "workspaces")
	if err := os.MkdirAll(workspaceConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace config dir: %v", err)
	}
	configPayload, err := json.Marshal(map[string]string{"path": workspacePath})
	if err != nil {
		t.Fatalf("marshal config payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceConfigDir, "ws-guide.json"), configPayload, 0o644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	reqID, err := svc.store.AddGuidanceRequest(model.GuidanceRequest{
		RunID:         spec.RunID,
		WorkspaceName: "ws-guide",
		AgentName:     "agent",
		Question:      "Need API decision?",
		Context:       "Pick schema",
		Status:        model.GuidanceStatusPending,
	})
	if err != nil {
		t.Fatalf("add guidance request: %v", err)
	}
	if reqID == 0 {
		t.Fatalf("expected non-zero guidance id")
	}

	result, err := svc.Guide(t.Context(), spec.RunID, "Use JSON payload format")
	if err != nil {
		t.Fatalf("guide: %v", err)
	}
	if result.GuidanceID != reqID {
		t.Fatalf("expected guidance id %d, got %d", reqID, result.GuidanceID)
	}

	record, _, _, err := svc.store.GetRun(spec.RunID)
	if err != nil {
		t.Fatalf("get run after guide: %v", err)
	}
	if record.Status != model.RunStatusRunning {
		t.Fatalf("expected run status running after guide, got %s", record.Status)
	}

	pending, err := svc.store.ListGuidanceRequests(spec.RunID, model.GuidanceStatusPending)
	if err != nil {
		t.Fatalf("list pending guidance: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending guidance requests, got %d", len(pending))
	}

	answered, err := svc.store.ListGuidanceRequests(spec.RunID, model.GuidanceStatusAnswered)
	if err != nil {
		t.Fatalf("list answered guidance: %v", err)
	}
	if len(answered) != 1 {
		t.Fatalf("expected one answered guidance request, got %d", len(answered))
	}
	if answered[0].Answer != "Use JSON payload format" {
		t.Fatalf("expected stored answer, got %q", answered[0].Answer)
	}

	if _, err := os.Stat(filepath.Join(workspacePath, ".metawsm", "guidance-response.json")); err != nil {
		t.Fatalf("expected guidance-response file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".metawsm", "guidance-request.json")); !os.IsNotExist(err) {
		t.Fatalf("expected guidance-request file to be removed")
	}
}

func TestWorkspaceNameForUsesUniqueRunToken(t *testing.T) {
	a := workspaceNameFor("METAWSM-003", "run-20260207-075350")
	b := workspaceNameFor("METAWSM-003", "run-20260207-075412")
	if a == b {
		t.Fatalf("expected different workspace names for different run ids, got same value %q", a)
	}
	if !strings.HasPrefix(a, "metawsm-003-") {
		t.Fatalf("expected workspace name prefix, got %q", a)
	}
}

func TestCloseBootstrapRequiresValidationResult(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)

	runID := "run-close-missing-validation"
	workspaceName := "ws-close-missing"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createBootstrapRunFixture(t, svc, runID, workspaceName)

	err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected close to fail without validation-result file")
	}
	if !strings.Contains(err.Error(), "validation-result.json") {
		t.Fatalf("expected validation-result close error, got: %v", err)
	}
}

func TestCloseBootstrapDryRunPassesWithValidationResult(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)

	runID := "run-close-with-validation"
	workspaceName := "ws-close-valid"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeValidationResult(t, workspacePath, runID, "tests pass")
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createBootstrapRunFixture(t, svc, runID, workspaceName)

	if err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true}); err != nil {
		t.Fatalf("close dry-run with validation: %v", err)
	}
	record, _, _, err := svc.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run after close: %v", err)
	}
	if record.Status != model.RunStatusClosed {
		t.Fatalf("expected run status closed, got %s", record.Status)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func setupWorkspaceConfigRoot(t *testing.T) string {
	t.Helper()
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	return homeDir
}

func writeWorkspaceConfig(t *testing.T, workspaceName string, workspacePath string) {
	t.Helper()
	configDir := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "workspace-manager", "workspaces")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace config dir: %v", err)
	}
	payload, err := json.Marshal(map[string]string{"path": workspacePath})
	if err != nil {
		t.Fatalf("marshal workspace config payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, workspaceName+".json"), payload, 0o644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
}

func createBootstrapRunFixture(t *testing.T, svc *Service, runID string, workspaceName string) {
	t.Helper()
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-002"},
		Repos:             []string{"metawsm"},
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, `{"version":1}`); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	if err := svc.store.UpdateRunStatus(runID, model.RunStatusComplete, ""); err != nil {
		t.Fatalf("set run status complete: %v", err)
	}
	now := time.Now()
	if err := svc.store.UpsertAgent(model.AgentRecord{
		RunID:          runID,
		Name:           "agent",
		WorkspaceName:  workspaceName,
		SessionName:    fmt.Sprintf("agent-%s", workspaceName),
		Status:         model.AgentStatusRunning,
		HealthState:    model.HealthStateHealthy,
		LastActivityAt: &now,
		LastProgressAt: &now,
	}); err != nil {
		t.Fatalf("upsert agent fixture: %v", err)
	}
	if err := svc.store.UpsertRunBrief(model.RunBrief{
		RunID:        runID,
		Ticket:       "METAWSM-002",
		Goal:         "Implement bootstrap flow",
		Scope:        "orchestrator",
		DoneCriteria: "tests pass",
		Constraints:  "none",
		MergeIntent:  "default",
		QA: []model.IntakeQA{
			{Question: "Goal?", Answer: "Implement bootstrap flow"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("upsert run brief fixture: %v", err)
	}
}

func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".gitkeep"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write .gitkeep: %v", err)
	}
	runCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	runCmd("init")
	runCmd("config", "user.email", "metawsm-test@example.com")
	runCmd("config", "user.name", "metawsm test")
	runCmd("add", ".")
	runCmd("commit", "-m", "init")
}

func writeValidationResult(t *testing.T, workspacePath string, runID string, doneCriteria string) {
	t.Helper()
	payload := map[string]string{
		"run_id":        runID,
		"status":        "passed",
		"done_criteria": doneCriteria,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal validation payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, ".metawsm", "validation-result.json"), b, 0o644); err != nil {
		t.Fatalf("write validation result: %v", err)
	}
}
