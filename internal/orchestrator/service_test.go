package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/policy"
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

	record, specJSON, _, err := svc.store.GetRun(result.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if record.Status != model.RunStatusPaused {
		t.Fatalf("expected paused run status after dry-run, got %s", record.Status)
	}
	var storedSpec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &storedSpec); err != nil {
		t.Fatalf("unmarshal stored spec: %v", err)
	}
	if storedSpec.DocRepo != "metawsm" {
		t.Fatalf("expected default doc repo metawsm, got %q", storedSpec.DocRepo)
	}
	if storedSpec.DocHomeRepo != "metawsm" {
		t.Fatalf("expected default doc home repo metawsm, got %q", storedSpec.DocHomeRepo)
	}
	if storedSpec.DocAuthorityMode != model.DocAuthorityModeWorkspaceActive {
		t.Fatalf("expected doc authority mode workspace_active, got %q", storedSpec.DocAuthorityMode)
	}
	if storedSpec.DocSeedMode != model.DocSeedModeCopyFromRepoOnStart {
		t.Fatalf("expected doc seed mode copy_from_repo_on_start, got %q", storedSpec.DocSeedMode)
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

func TestRunDryRunUsesExplicitDocRepoOverride(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.Run(t.Context(), RunOptions{
		Tickets:           []string{"METAWSM-001"},
		Repos:             []string{"metawsm", "workspace-manager"},
		DocRepo:           "workspace-manager",
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		DryRun:            true,
	})
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	_, specJSON, _, err := svc.store.GetRun(result.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	var storedSpec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &storedSpec); err != nil {
		t.Fatalf("unmarshal stored spec: %v", err)
	}
	if storedSpec.DocRepo != "workspace-manager" {
		t.Fatalf("expected doc repo workspace-manager, got %q", storedSpec.DocRepo)
	}
	if storedSpec.DocHomeRepo != "workspace-manager" {
		t.Fatalf("expected doc home repo workspace-manager, got %q", storedSpec.DocHomeRepo)
	}
}

func TestRunRejectsDocRepoOutsideRepos(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.Run(t.Context(), RunOptions{
		Tickets:           []string{"METAWSM-001"},
		Repos:             []string{"metawsm"},
		DocRepo:           "workspace-manager",
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		DryRun:            true,
	})
	if err == nil {
		t.Fatalf("expected doc repo validation error")
	}
	if !strings.Contains(err.Error(), "must be one of --repos") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsConflictingDocHomeAndLegacyDocRepo(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.Run(t.Context(), RunOptions{
		Tickets:           []string{"METAWSM-001"},
		Repos:             []string{"metawsm", "workspace-manager"},
		DocHomeRepo:       "metawsm",
		DocRepo:           "workspace-manager",
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		DryRun:            true,
	})
	if err == nil {
		t.Fatalf("expected doc home/doc repo conflict error")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("unexpected error: %v", err)
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

func TestReadGuidanceRequestFileSupportsStructuredContext(t *testing.T) {
	workspacePath := t.TempDir()
	requestDir := filepath.Join(workspacePath, ".metawsm")
	if err := os.MkdirAll(requestDir, 0o755); err != nil {
		t.Fatalf("mkdir .metawsm: %v", err)
	}

	content := `{
  "run_id": "run-structured",
  "agent": "agent",
  "question": "Can we defer npm install?",
  "context": {
    "blocked_checks": [
      "npm --prefix ui install"
    ],
    "reason": "network unavailable"
  }
}`
	if err := os.WriteFile(filepath.Join(requestDir, "guidance-request.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write guidance request file: %v", err)
	}

	payload, ok := readGuidanceRequestFile(workspacePath)
	if !ok {
		t.Fatalf("expected structured guidance request to parse")
	}
	if payload.Question != "Can we defer npm install?" {
		t.Fatalf("unexpected question: %q", payload.Question)
	}
	if !strings.Contains(payload.Context, "blocked_checks") {
		t.Fatalf("expected serialized context payload, got %q", payload.Context)
	}
}

func TestReadGuidanceRequestFileFromRootsFindsDocHomeRepo(t *testing.T) {
	workspacePath := t.TempDir()
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(filepath.Join(repoPath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir repo .metawsm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".metawsm", "guidance-request.json"), []byte(`{"question":"Need decision","context":{"reason":"blocked"}}`), 0o644); err != nil {
		t.Fatalf("write guidance request file: %v", err)
	}

	spec := model.RunSpec{
		Repos:       []string{"metawsm"},
		DocHomeRepo: "metawsm",
	}
	roots := bootstrapSignalRoots(workspacePath, spec)
	payload, ok := readGuidanceRequestFileFromRoots(roots)
	if !ok {
		t.Fatalf("expected to find guidance request in doc-home repo")
	}
	if payload.Question != "Need decision" {
		t.Fatalf("unexpected question: %q", payload.Question)
	}
	if !strings.Contains(payload.Context, "blocked") {
		t.Fatalf("expected serialized context in payload, got %q", payload.Context)
	}
}

func TestGuideWritesResponseInDocHomeRepoAndRemovesRequest(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	spec := model.RunSpec{
		RunID:             "run-guide-doc-root",
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-002"},
		Repos:             []string{"metawsm"},
		DocHomeRepo:       "metawsm",
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
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(filepath.Join(repoPath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir repo workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".metawsm", "guidance-request.json"), []byte(`{"question":"Need API decision?"}`), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(workspaceConfigDir, "ws-guide-doc-root.json"), configPayload, 0o644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	_, err = svc.store.AddGuidanceRequest(model.GuidanceRequest{
		RunID:         spec.RunID,
		WorkspaceName: "ws-guide-doc-root",
		AgentName:     "agent",
		Question:      "Need API decision?",
		Context:       "Pick schema",
		Status:        model.GuidanceStatusPending,
	})
	if err != nil {
		t.Fatalf("add guidance request: %v", err)
	}

	if _, err := svc.Guide(t.Context(), spec.RunID, "Use JSON payload format"); err != nil {
		t.Fatalf("guide: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoPath, ".metawsm", "guidance-response.json")); err != nil {
		t.Fatalf("expected guidance-response file in doc-home repo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".metawsm", "guidance-request.json")); !os.IsNotExist(err) {
		t.Fatalf("expected guidance-request file to be removed from doc-home repo")
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

func TestBuildPlanBootstrapIncludesTicketContextSyncBeforeTmux(t *testing.T) {
	spec := model.RunSpec{
		RunID:             "run-20260207-093000",
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-004"},
		Repos:             []string{"metawsm"},
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents: []model.AgentSpec{
			{Name: "agent", Command: "bash"},
		},
	}

	steps := buildPlan(spec, policy.Default())
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps for single bootstrap ticket/agent, got %d", len(steps))
	}
	expectedKinds := []string{"shell", "shell", "ticket_context_sync", "tmux_start"}
	for i, expected := range expectedKinds {
		if steps[i].Kind != expected {
			t.Fatalf("expected step %d kind %q, got %q", i+1, expected, steps[i].Kind)
		}
	}
	if steps[2].Ticket != "METAWSM-004" {
		t.Fatalf("expected ticket on sync step, got %q", steps[2].Ticket)
	}
}

func TestBuildPlanStandardModeIncludesTicketContextSyncWhenSeedEnabled(t *testing.T) {
	spec := model.RunSpec{
		RunID:             "run-20260207-093100",
		Mode:              model.RunModeStandard,
		Tickets:           []string{"METAWSM-004"},
		Repos:             []string{"metawsm"},
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents: []model.AgentSpec{
			{Name: "agent", Command: "bash"},
		},
	}

	steps := buildPlan(spec, policy.Default())
	foundSync := false
	for _, step := range steps {
		if step.Kind == "ticket_context_sync" {
			foundSync = true
			break
		}
	}
	if !foundSync {
		t.Fatalf("expected ticket_context_sync step in standard mode when seeding is enabled")
	}
}

func TestBuildPlanSkipsTicketContextSyncWhenSeedDisabled(t *testing.T) {
	spec := model.RunSpec{
		RunID:             "run-20260207-093101",
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-004"},
		Repos:             []string{"metawsm"},
		DocSeedMode:       model.DocSeedModeNone,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents: []model.AgentSpec{
			{Name: "agent", Command: "bash"},
		},
	}

	steps := buildPlan(spec, policy.Default())
	for _, step := range steps {
		if step.Kind == "ticket_context_sync" {
			t.Fatalf("did not expect ticket_context_sync step when seeding is disabled")
		}
	}
}

func TestParseDocmgrTicketListPaths(t *testing.T) {
	output := []byte("" +
		"Docs root: `/tmp/metawsm/ttmp`\n" +
		"\n" +
		"## Tickets (1)\n" +
		"\n" +
		"### METAWSM-004\n" +
		"- Path: `2026/02/07/METAWSM-004--bootstrap-workspace-context-handoff-and-nested-repo-ambiguity`\n")

	docsRoot, ticketPath, err := parseDocmgrTicketListPaths(output)
	if err != nil {
		t.Fatalf("parse docmgr ticket list output: %v", err)
	}
	if docsRoot != filepath.Clean("/tmp/metawsm/ttmp") {
		t.Fatalf("unexpected docs root: %q", docsRoot)
	}
	expectedPath := filepath.Join("2026", "02", "07", "METAWSM-004--bootstrap-workspace-context-handoff-and-nested-repo-ambiguity")
	if ticketPath != expectedPath {
		t.Fatalf("unexpected ticket path: got %q want %q", ticketPath, expectedPath)
	}
}

func TestParseDocmgrTicketListPathsRejectsUnsafeRelativePath(t *testing.T) {
	output := []byte("" +
		"Docs root: `/tmp/metawsm/ttmp`\n" +
		"- Path: `../escape`\n")

	_, _, err := parseDocmgrTicketListPaths(output)
	if err == nil {
		t.Fatalf("expected unsafe path parse to fail")
	}
	if !strings.Contains(err.Error(), "unsafe ticket path") {
		t.Fatalf("expected unsafe path error, got %v", err)
	}
}

func TestSyncTicketDocsDirectoryCopiesTreeAndRemovesStaleFiles(t *testing.T) {
	root := t.TempDir()
	relativePath := filepath.Join("2026", "02", "07", "METAWSM-004--context-sync")
	sourcePath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Join(sourcePath, "reference"), 0o755); err != nil {
		t.Fatalf("mkdir source path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "README.md"), []byte("ticket docs\n"), 0o644); err != nil {
		t.Fatalf("write source README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "reference", "01-analysis.md"), []byte("analysis\n"), 0o644); err != nil {
		t.Fatalf("write source analysis: %v", err)
	}

	workspacePath := filepath.Join(root, "workspace")
	staleFile := filepath.Join(workspacePath, "ttmp", relativePath, "stale.md")
	if err := os.MkdirAll(filepath.Dir(staleFile), 0o755); err != nil {
		t.Fatalf("mkdir stale file dir: %v", err)
	}
	if err := os.WriteFile(staleFile, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	if err := syncTicketDocsDirectory(sourcePath, relativePath, workspacePath); err != nil {
		t.Fatalf("sync ticket docs directory: %v", err)
	}

	readmePath := filepath.Join(workspacePath, "ttmp", relativePath, "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read copied README: %v", err)
	}
	if strings.TrimSpace(string(readmeBytes)) != "ticket docs" {
		t.Fatalf("unexpected copied README content: %q", string(readmeBytes))
	}

	analysisPath := filepath.Join(workspacePath, "ttmp", relativePath, "reference", "01-analysis.md")
	if _, err := os.Stat(analysisPath); err != nil {
		t.Fatalf("expected copied nested file: %v", err)
	}

	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removal, stat err=%v", err)
	}
}

func TestSyncTicketDocsDirectoryTargetsDocRepoRoot(t *testing.T) {
	root := t.TempDir()
	relativePath := filepath.Join("2026", "02", "07", "METAWSM-004--context-sync")
	sourcePath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Join(sourcePath, "reference"), 0o755); err != nil {
		t.Fatalf("mkdir source path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "README.md"), []byte("ticket docs\n"), 0o644); err != nil {
		t.Fatalf("write source README: %v", err)
	}

	workspacePath := filepath.Join(root, "workspace")
	docRootPath := filepath.Join(workspacePath, "metawsm")
	staleFile := filepath.Join(docRootPath, "ttmp", relativePath, "stale.md")
	if err := os.MkdirAll(filepath.Dir(staleFile), 0o755); err != nil {
		t.Fatalf("mkdir stale file dir: %v", err)
	}
	if err := os.WriteFile(staleFile, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	if err := syncTicketDocsDirectory(sourcePath, relativePath, docRootPath); err != nil {
		t.Fatalf("sync ticket docs directory: %v", err)
	}

	readmePath := filepath.Join(docRootPath, "ttmp", relativePath, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("expected copied README in doc repo root: %v", err)
	}
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removal in doc repo root, stat err=%v", err)
	}
}

func TestRestartDryRunResolvesLatestRunByTicket(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	ticket := "METAWSM-003"

	oldWorkspace := "ws-restart-old"
	newWorkspace := "ws-restart-new"
	oldPath := filepath.Join(homeDir, "workspaces", oldWorkspace)
	newPath := filepath.Join(homeDir, "workspaces", newWorkspace)
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("mkdir old workspace: %v", err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("mkdir new workspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(oldPath, "metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir old doc repo path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(newPath, "metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir new doc repo path: %v", err)
	}
	writeWorkspaceConfig(t, oldWorkspace, oldPath)
	writeWorkspaceConfig(t, newWorkspace, newPath)

	createRunWithTicketFixture(t, svc, "run-restart-a", ticket, oldWorkspace, model.RunStatusPaused, false)
	createRunWithTicketFixture(t, svc, "run-restart-z", ticket, newWorkspace, model.RunStatusPaused, false)

	result, err := svc.Restart(t.Context(), RestartOptions{
		Ticket: ticket,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("restart dry-run: %v", err)
	}
	if result.RunID != "run-restart-z" {
		t.Fatalf("expected latest run id run-restart-z, got %s", result.RunID)
	}
	if len(result.Actions) != 2 {
		t.Fatalf("expected 2 restart actions, got %d", len(result.Actions))
	}
	if !strings.Contains(result.Actions[0], "tmux kill-session -t") {
		t.Fatalf("expected kill-session action, got %q", result.Actions[0])
	}
	if !strings.Contains(result.Actions[1], "tmux new-session -d -s") {
		t.Fatalf("expected new-session action, got %q", result.Actions[1])
	}
	expectedWorkdir := filepath.Join(newPath, "metawsm")
	if !strings.Contains(result.Actions[1], shellQuote(expectedWorkdir)) {
		t.Fatalf("expected workdir path %q in action %q", expectedWorkdir, result.Actions[1])
	}
}

func TestCleanupDryRunByTicketIncludesWorkspaceDelete(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	ticket := "METAWSM-004"
	workspaceName := "ws-cleanup"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixture(t, svc, "run-cleanup-z", ticket, workspaceName, model.RunStatusRunning, false)

	result, err := svc.Cleanup(t.Context(), CleanupOptions{
		Ticket:           ticket,
		DryRun:           true,
		DeleteWorkspaces: true,
	})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if result.RunID != "run-cleanup-z" {
		t.Fatalf("expected run id run-cleanup-z, got %s", result.RunID)
	}
	if len(result.Actions) != 2 {
		t.Fatalf("expected 2 cleanup actions, got %d", len(result.Actions))
	}
	if !strings.Contains(result.Actions[0], "tmux kill-session -t") {
		t.Fatalf("expected kill-session action, got %q", result.Actions[0])
	}
	if !strings.Contains(result.Actions[1], "wsm delete") {
		t.Fatalf("expected workspace delete action, got %q", result.Actions[1])
	}
	if !strings.Contains(result.Actions[1], shellQuote(workspaceName)) {
		t.Fatalf("expected workspace name %q in delete action %q", workspaceName, result.Actions[1])
	}
}

func TestCleanupDryRunByTicketPrefersNonDryRun(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	ticket := "METAWSM-005"
	createRunWithTicketFixture(t, svc, "run-nondry", ticket, "ws-real", model.RunStatusRunning, false)
	createRunWithTicketFixture(t, svc, "run-dry", ticket, "ws-dry", model.RunStatusPaused, true)

	result, err := svc.Cleanup(t.Context(), CleanupOptions{
		Ticket:           ticket,
		DryRun:           true,
		DeleteWorkspaces: true,
	})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if result.RunID != "run-nondry" {
		t.Fatalf("expected non-dry run selection run-nondry, got %s", result.RunID)
	}
}

func TestResolveRunIDByTicketPrefersNonDryRun(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	ticket := "METAWSM-009"
	createRunWithTicketFixture(t, svc, "run-resolve-nondry", ticket, "ws-resolve-real", model.RunStatusRunning, false)
	createRunWithTicketFixture(t, svc, "run-resolve-dry", ticket, "ws-resolve-dry", model.RunStatusPaused, true)

	runID, err := svc.ResolveRunID("", ticket)
	if err != nil {
		t.Fatalf("resolve run id by ticket: %v", err)
	}
	if runID != "run-resolve-nondry" {
		t.Fatalf("expected non-dry run id run-resolve-nondry, got %s", runID)
	}

	explicit, err := svc.ResolveRunID("run-explicit", ticket)
	if err != nil {
		t.Fatalf("resolve explicit run id: %v", err)
	}
	if explicit != "run-explicit" {
		t.Fatalf("expected explicit run id passthrough, got %s", explicit)
	}
}

func TestBootstrapStatusTransitionsRunToFailedWhenAgentFailed(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	workspaceName := "ws-failed-agent"
	createRunWithTicketFixture(t, svc, "run-agent-failed", "METAWSM-006", workspaceName, model.RunStatusRunning, false)
	if err := svc.syncBootstrapSignals(t.Context(), "run-agent-failed", model.RunStatusRunning, model.RunSpec{}, []model.AgentRecord{
		{
			RunID:         "run-agent-failed",
			Name:          "agent",
			WorkspaceName: workspaceName,
			Status:        model.AgentStatusFailed,
		},
	}); err != nil {
		t.Fatalf("sync bootstrap signals: %v", err)
	}
	record, _, _, err := svc.store.GetRun("run-agent-failed")
	if err != nil {
		t.Fatalf("get run after sync: %v", err)
	}
	if record.Status != model.RunStatusFailed {
		t.Fatalf("expected run status failed, got %s", record.Status)
	}
}

func TestResetRepoToBaseBranchUsesLocalBranchWhenOriginMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.email", "metawsm-test@example.com")
	runGit(t, repoPath, "config", "user.name", "metawsm test")
	runGit(t, repoPath, "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(repoPath, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	runGit(t, repoPath, "add", "a.txt")
	runGit(t, repoPath, "commit", "-m", "first")
	mainBefore := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))

	if err := os.WriteFile(filepath.Join(repoPath, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write a.txt second: %v", err)
	}
	runGit(t, repoPath, "add", "a.txt")
	runGit(t, repoPath, "commit", "-m", "second")
	mainAfter := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	if mainBefore == mainAfter {
		t.Fatalf("expected distinct commits for test setup")
	}

	runGit(t, repoPath, "checkout", "-b", "task/test", mainBefore)
	taskHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	if taskHead != mainBefore {
		t.Fatalf("expected task branch to start at first commit")
	}

	if err := resetRepoToBaseBranch(t.Context(), repoPath, "main"); err != nil {
		t.Fatalf("reset to base branch: %v", err)
	}
	afterReset := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	if afterReset != mainAfter {
		t.Fatalf("expected reset head %s, got %s", mainAfter, afterReset)
	}
}

func TestWrapAgentCommandForTmuxKeepsShellAlive(t *testing.T) {
	wrapped := wrapAgentCommandForTmux("echo hi")
	if !strings.HasPrefix(wrapped, "bash -lc ") {
		t.Fatalf("expected wrapped command to start with bash -lc, got %q", wrapped)
	}
	if !strings.Contains(wrapped, "exec bash") {
		t.Fatalf("expected wrapped command to keep shell alive, got %q", wrapped)
	}
}

func TestParseAgentExitCode(t *testing.T) {
	if _, ok := parseAgentExitCode("no marker"); ok {
		t.Fatalf("expected no exit code match")
	}
	code, ok := parseAgentExitCode("[metawsm] agent command exited with status 1 at 2026-02-07T08:47:30-08:00")
	if !ok || code != 1 {
		t.Fatalf("expected exit code 1, got %d ok=%v", code, ok)
	}
	code, ok = parseAgentExitCode(
		"[metawsm] agent command exited with status 1 at 2026-02-07T08:47:30-08:00\n" +
			"[metawsm] agent command exited with status 0 at 2026-02-07T08:49:30-08:00\n",
	)
	if !ok || code != 0 {
		t.Fatalf("expected latest exit code 0, got %d ok=%v", code, ok)
	}
}

func TestNormalizeAgentCommand(t *testing.T) {
	normalized := normalizeAgentCommand("codex exec --full-auto \"do work\"")
	if !strings.Contains(normalized, "--skip-git-repo-check") {
		t.Fatalf("expected codex command to include skip git repo check flag, got %q", normalized)
	}

	already := normalizeAgentCommand("codex exec --skip-git-repo-check --full-auto \"do work\"")
	if strings.Count(already, "--skip-git-repo-check") != 1 {
		t.Fatalf("expected skip flag not to be duplicated, got %q", already)
	}

	other := normalizeAgentCommand("bash")
	if other != "bash" {
		t.Fatalf("expected non-codex command unchanged, got %q", other)
	}
}

func TestIsWorkspaceNotFoundOutput(t *testing.T) {
	if !isWorkspaceNotFoundOutput("Error: workspace 'abc' not found") {
		t.Fatalf("expected workspace-not-found output to match")
	}
	if isWorkspaceNotFoundOutput("permission denied removing worktree") {
		t.Fatalf("expected unrelated output not to match")
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
	writeTicketDocDirFixture(t, workspacePath, "METAWSM-002")
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createBootstrapRunFixture(t, svc, runID, workspaceName)
	upsertDocSyncStateFixture(t, svc, runID, "METAWSM-002", workspaceName, model.DocSyncStatusSynced, "rev-1")

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
	writeTicketDocDirFixture(t, workspacePath, "METAWSM-002")
	writeValidationResult(t, workspacePath, runID, "tests pass")
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createBootstrapRunFixture(t, svc, runID, workspaceName)
	upsertDocSyncStateFixture(t, svc, runID, "METAWSM-002", workspaceName, model.DocSyncStatusSynced, "rev-2")

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

func TestCloseBootstrapDryRunBlocksWhenNestedRepoDirty(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)

	runID := "run-close-dirty-nested-repo"
	workspaceName := "ws-close-dirty-nested"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeValidationResult(t, workspacePath, runID, "tests pass")
	repoA := filepath.Join(workspacePath, "repo-a")
	repoB := filepath.Join(workspacePath, "repo-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatalf("mkdir repo-a: %v", err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatalf("mkdir repo-b: %v", err)
	}
	initGitRepo(t, repoA)
	initGitRepo(t, repoB)
	if err := os.WriteFile(filepath.Join(repoB, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty repo file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createBootstrapRunFixtureWithRepos(t, svc, runID, workspaceName, []string{"repo-a", "repo-b"})
	ticketDir := filepath.Join(workspacePath, "repo-a", "ttmp", "2026", "02", "08", "metawsm-002--fixture")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("mkdir ticket dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write ticket fixture: %v", err)
	}
	runGit(t, repoA, "add", ".")
	runGit(t, repoA, "commit", "-m", "add ticket fixture")
	upsertDocSyncStateFixture(t, svc, runID, "METAWSM-002", workspaceName, model.DocSyncStatusSynced, "rev-3")

	err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected close to fail when nested repo is dirty")
	}
	if !strings.Contains(err.Error(), "repo-b") {
		t.Fatalf("expected dirty nested repo path in error, got: %v", err)
	}
}

func TestCloseDryRunBlocksWhenDocSyncStateMissing(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-close-missing-sync"
	workspaceName := "ws-close-missing-sync"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTicketDocDirFixture(t, workspacePath, "METAWSM-002")
	writeValidationResult(t, workspacePath, runID, "tests pass")
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createBootstrapRunFixture(t, svc, runID, workspaceName)

	err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected close to fail when doc sync state is missing")
	}
	if !strings.Contains(err.Error(), "missing doc sync state") {
		t.Fatalf("expected missing doc sync state error, got: %v", err)
	}
}

func TestCloseDryRunBlocksWhenDocSyncFailed(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-close-failed-sync"
	workspaceName := "ws-close-failed-sync"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTicketDocDirFixture(t, workspacePath, "METAWSM-002")
	writeValidationResult(t, workspacePath, runID, "tests pass")
	initGitRepo(t, workspacePath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createBootstrapRunFixture(t, svc, runID, workspaceName)
	upsertDocSyncStateFixture(t, svc, runID, "METAWSM-002", workspaceName, model.DocSyncStatusFailed, "")

	err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected close to fail when doc sync state is failed")
	}
	if !strings.Contains(err.Error(), "doc sync status=failed") {
		t.Fatalf("expected failed doc sync state error, got: %v", err)
	}
}

func TestMergeDryRunByRunIDIncludesOnlyDirtyWorkspaces(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	ticket := "METAWSM-007"
	workspaceName := "ws-merge-diff"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoA := filepath.Join(workspacePath, "repo-a")
	repoB := filepath.Join(workspacePath, "repo-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatalf("mkdir repo-a: %v", err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatalf("mkdir repo-b: %v", err)
	}
	initGitRepo(t, repoA)
	initGitRepo(t, repoB)
	if err := os.WriteFile(filepath.Join(repoB, "feature.txt"), []byte("work in progress\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createRunWithTicketFixtureWithRepos(t, svc, "run-merge-z", ticket, workspaceName, model.RunStatusComplete, false, []string{"repo-a", "repo-b"})

	result, err := svc.Merge(t.Context(), MergeOptions{
		RunID:  "run-merge-z",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("merge dry-run: %v", err)
	}
	if result.RunID != "run-merge-z" {
		t.Fatalf("expected merge run id run-merge-z, got %s", result.RunID)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one merge action, got %d", len(result.Actions))
	}
	if !strings.Contains(result.Actions[0], "wsm merge") || !strings.Contains(result.Actions[0], workspaceName) {
		t.Fatalf("unexpected merge action: %q", result.Actions[0])
	}
}

func TestStatusIncludesPerRepoDiffsForWorkspace(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	ticket := "METAWSM-008"
	runID := "run-status-diffs"
	workspaceName := "ws-status-diffs"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoA := filepath.Join(workspacePath, "repo-a")
	repoB := filepath.Join(workspacePath, "repo-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatalf("mkdir repo-a: %v", err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatalf("mkdir repo-b: %v", err)
	}
	initGitRepo(t, repoA)
	initGitRepo(t, repoB)
	if err := os.WriteFile(filepath.Join(repoB, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"repo-a", "repo-b"})

	status, err := svc.Status(t.Context(), runID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "Diffs:") {
		t.Fatalf("expected Diffs section in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "repo-a clean") {
		t.Fatalf("expected clean repo-a line in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "repo-b dirty") {
		t.Fatalf("expected dirty repo-b line in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "?? dirty.txt") {
		t.Fatalf("expected dirty file line in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "metawsm merge --ticket "+ticket) {
		t.Fatalf("expected ticket-based merge next-step guidance in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "metawsm iterate --ticket "+ticket) {
		t.Fatalf("expected iterate next-step guidance in status output, got:\n%s", status)
	}
}

func TestStatusShowsWarningOnlyForStaleDocFreshness(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-status-stale-docs"
	ticket := "METAWSM-012"
	workspaceName := "ws-status-stale-docs"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})
	if err := svc.store.UpsertDocSyncState(model.DocSyncState{
		RunID:            runID,
		Ticket:           ticket,
		WorkspaceName:    workspaceName,
		DocHomeRepo:      "metawsm",
		DocAuthorityMode: string(model.DocAuthorityModeWorkspaceActive),
		DocSeedMode:      string(model.DocSeedModeCopyFromRepoOnStart),
		Status:           model.DocSyncStatusSynced,
		Revision:         "stale-revision",
		UpdatedAt:        time.Now().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("upsert stale doc sync state: %v", err)
	}

	status, err := svc.Status(t.Context(), runID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "warning=docmgr index freshness stale") {
		t.Fatalf("expected stale freshness warning in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "(warning-only)") {
		t.Fatalf("expected warning-only marker in status output, got:\n%s", status)
	}
}

func TestStatusShowsPersistedRunPullRequests(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-status-prs"
	ticket := "METAWSM-012"
	workspaceName := "ws-status-prs"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	writeWorkspaceConfig(t, workspaceName, workspacePath)

	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:      runID,
		Ticket:     ticket,
		Repo:       "metawsm",
		HeadBranch: "METAWSM-012/metawsm/run-status-prs",
		BaseBranch: "main",
		PRNumber:   17,
		PRURL:      "https://github.com/example/metawsm/pull/17",
		PRState:    model.PullRequestStateOpen,
		Actor:      "kball",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request: %v", err)
	}

	status, err := svc.Status(t.Context(), runID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "Pull Requests:") {
		t.Fatalf("expected pull requests section in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "METAWSM-012/metawsm state=open") {
		t.Fatalf("expected ticket/repo/state line in status output, got:\n%s", status)
	}
	if !strings.Contains(status, "https://github.com/example/metawsm/pull/17") {
		t.Fatalf("expected pr url in status output, got:\n%s", status)
	}
}

func TestCommitDryRunPreviewsActionsForDirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-dry"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-dry"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("draft change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	before := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		DryRun:  true,
		Message: "METAWSM-009: preview commit",
	})
	if err != nil {
		t.Fatalf("commit dry-run: %v", err)
	}
	if result.RunID != runID {
		t.Fatalf("expected run id %q, got %q", runID, result.RunID)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if !repoResult.Dirty {
		t.Fatalf("expected dirty repo result")
	}
	if len(repoResult.Actions) != 6 {
		t.Fatalf("expected 6 dry-run actions with snapshot+checkout flow, got %d", len(repoResult.Actions))
	}
	if !strings.Contains(repoResult.Actions[0], "stash push -u") {
		t.Fatalf("expected first dry-run action to snapshot workspace, got %q", repoResult.Actions[0])
	}
	if !strings.Contains(repoResult.Actions[2], "stash apply --index") {
		t.Fatalf("expected dry-run action to reapply snapshot, got %q", repoResult.Actions[2])
	}
	expectedBranch := policy.RenderGitBranch(policy.Default().GitPR.BranchTemplate, ticket, "metawsm", runID)
	if repoResult.Branch != expectedBranch {
		t.Fatalf("expected branch %q, got %q", expectedBranch, repoResult.Branch)
	}
	after := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	if before != after {
		t.Fatalf("expected dry-run to keep head unchanged, before=%s after=%s", before, after)
	}
}

func TestCommitCreatesBranchCommitAndPersistsPullRequestRow(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-real"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-real"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("real change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	commitMessage := "METAWSM-009: persist commit primitive result"
	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: commitMessage,
		Actor:   "kball",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if strings.TrimSpace(repoResult.CommitSHA) == "" {
		t.Fatalf("expected commit sha to be recorded")
	}
	expectedBranch := policy.RenderGitBranch(policy.Default().GitPR.BranchTemplate, ticket, "metawsm", runID)
	currentBranch := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
	if currentBranch != expectedBranch {
		t.Fatalf("expected branch %q, got %q", expectedBranch, currentBranch)
	}
	subject := strings.TrimSpace(runGit(t, repoPath, "log", "-1", "--pretty=%s"))
	if subject != commitMessage {
		t.Fatalf("expected commit message %q, got %q", commitMessage, subject)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one run pull request row, got %d", len(rows))
	}
	row := rows[0]
	if row.CommitSHA != repoResult.CommitSHA {
		t.Fatalf("expected stored commit sha %q, got %q", repoResult.CommitSHA, row.CommitSHA)
	}
	if row.HeadBranch != expectedBranch {
		t.Fatalf("expected stored head branch %q, got %q", expectedBranch, row.HeadBranch)
	}
	if row.BaseBranch != "main" {
		t.Fatalf("expected stored base branch main, got %q", row.BaseBranch)
	}
	if row.CredentialMode != "local_user_auth" {
		t.Fatalf("expected credential mode local_user_auth, got %q", row.CredentialMode)
	}
	if row.Actor != "kball" {
		t.Fatalf("expected actor kball, got %q", row.Actor)
	}
	if row.PRState != model.PullRequestStateDraft {
		t.Fatalf("expected draft PR state, got %q", row.PRState)
	}
}

func TestCommitSkipsCleanRepoWithoutPersistingRow(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-clean"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-clean"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID: runID,
	})
	if err != nil {
		t.Fatalf("commit clean repo: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if repoResult.Dirty {
		t.Fatalf("expected clean repo result")
	}
	if !strings.Contains(repoResult.SkippedReason, "clean") {
		t.Fatalf("expected clean skip reason, got %q", repoResult.SkippedReason)
	}
	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no persisted pull request rows for clean repo, got %d", len(rows))
	}
}

func TestCommitFansOutAcrossWorkspaceTicketsWhenRunHasMultipleTickets(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-fanout"
	ticketA := "METAWSM-009"
	ticketB := "METAWSM-010"
	workspaceA := "ws-commit-fanout-a"
	workspaceB := "ws-commit-fanout-b"
	workspacePathA := filepath.Join(homeDir, "workspaces", workspaceA)
	workspacePathB := filepath.Join(homeDir, "workspaces", workspaceB)
	repoPathA := filepath.Join(workspacePathA, "metawsm")
	repoPathB := filepath.Join(workspacePathB, "metawsm")
	for _, repoPath := range []string{repoPathA, repoPathB} {
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			t.Fatalf("mkdir repo path: %v", err)
		}
		initGitRepo(t, repoPath)
		runGit(t, repoPath, "checkout", "-B", "main")
	}
	if err := os.WriteFile(filepath.Join(repoPathA, "ticket-a.txt"), []byte("change a\n"), 0o644); err != nil {
		t.Fatalf("write ticket-a file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPathB, "ticket-b.txt"), []byte("change b\n"), 0o644); err != nil {
		t.Fatalf("write ticket-b file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceA, workspacePathA)
	writeWorkspaceConfig(t, workspaceB, workspacePathB)
	createRunWithTicketsFixture(t, svc, runID, []string{ticketA, ticketB}, map[string]string{
		ticketA: workspaceA,
		ticketB: workspaceB,
	}, model.RunStatusComplete, `{"version":2}`)

	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM: fanout commit",
		Actor:   "kball",
	})
	if err != nil {
		t.Fatalf("commit fanout: %v", err)
	}
	if len(result.Repos) != 2 {
		t.Fatalf("expected 2 repo results, got %d", len(result.Repos))
	}
	seenTickets := map[string]bool{}
	for _, repoResult := range result.Repos {
		if strings.TrimSpace(repoResult.CommitSHA) == "" {
			t.Fatalf("expected commit SHA for repo result %+v", repoResult)
		}
		seenTickets[repoResult.Ticket] = true
	}
	if !seenTickets[ticketA] || !seenTickets[ticketB] {
		t.Fatalf("expected fanout across tickets %s and %s, got %+v", ticketA, ticketB, seenTickets)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 persisted rows, got %d", len(rows))
	}
	rowTickets := map[string]bool{}
	for _, row := range rows {
		rowTickets[row.Ticket] = true
	}
	if !rowTickets[ticketA] || !rowTickets[ticketB] {
		t.Fatalf("expected persisted rows for both tickets, got %+v", rowTickets)
	}
}

func TestOpenPullRequestsFansOutAcrossTicketsWhenRunHasMultipleTickets(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-fanout"
	ticketA := "METAWSM-009"
	ticketB := "METAWSM-010"
	workspaceA := "ws-pr-fanout-a"
	workspaceB := "ws-pr-fanout-b"
	workspacePathA := filepath.Join(homeDir, "workspaces", workspaceA)
	workspacePathB := filepath.Join(homeDir, "workspaces", workspaceB)
	repoPathA := filepath.Join(workspacePathA, "metawsm")
	repoPathB := filepath.Join(workspacePathB, "metawsm")
	for _, repoPath := range []string{repoPathA, repoPathB} {
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			t.Fatalf("mkdir repo path: %v", err)
		}
		initGitRepo(t, repoPath)
		runGit(t, repoPath, "checkout", "-B", "main")
	}
	if err := os.WriteFile(filepath.Join(repoPathA, "ticket-a.txt"), []byte("change a\n"), 0o644); err != nil {
		t.Fatalf("write ticket-a file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPathB, "ticket-b.txt"), []byte("change b\n"), 0o644); err != nil {
		t.Fatalf("write ticket-b file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceA, workspacePathA)
	writeWorkspaceConfig(t, workspaceB, workspacePathB)
	createRunWithTicketsFixture(t, svc, runID, []string{ticketA, ticketB}, map[string]string{
		ticketA: workspaceA,
		ticketB: workspaceB,
	}, model.RunStatusComplete, `{"version":2}`)

	if _, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM: fanout commit",
		Actor:   "kball",
	}); err != nil {
		t.Fatalf("prepare commit rows for PR fanout: %v", err)
	}

	result, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{
		RunID:  runID,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("pr fanout dry-run: %v", err)
	}
	if len(result.Repos) != 2 {
		t.Fatalf("expected 2 PR repo results, got %d", len(result.Repos))
	}
	seenTickets := map[string]bool{}
	for _, repoResult := range result.Repos {
		seenTickets[repoResult.Ticket] = true
		if len(repoResult.Actions) == 0 {
			t.Fatalf("expected dry-run actions for %s", repoResult.Ticket)
		}
	}
	if !seenTickets[ticketA] || !seenTickets[ticketB] {
		t.Fatalf("expected PR fanout across tickets %s and %s, got %+v", ticketA, ticketB, seenTickets)
	}
}

func TestCommitRejectsWhenRunNotComplete(t *testing.T) {
	svc := newTestService(t)
	runID := "run-commit-reject-status"
	ticket := "METAWSM-009"
	createRunWithTicketFixture(t, svc, runID, ticket, "ws-commit-reject-status", model.RunStatusRunning, false)

	_, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected commit status preflight error")
	}
	if !strings.Contains(err.Error(), "must be completed before commit") {
		t.Fatalf("unexpected commit preflight error: %v", err)
	}
}

func TestCommitRejectsWhenGitPRModeOff(t *testing.T) {
	svc := newTestService(t)
	runID := "run-commit-reject-mode"
	ticket := "METAWSM-009"
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{ticket},
		Repos:             []string{"metawsm"},
		DocRepo:           "metawsm",
		DocHomeRepo:       "metawsm",
		DocAuthorityMode:  model.DocAuthorityModeWorkspaceActive,
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, `{"git_pr":{"mode":"off"}}`); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	if err := svc.store.UpdateRunStatus(runID, model.RunStatusComplete, ""); err != nil {
		t.Fatalf("set run status complete: %v", err)
	}

	_, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected commit mode-off preflight error")
	}
	if !strings.Contains(err.Error(), "git_pr.mode is off") {
		t.Fatalf("unexpected commit mode-off error: %v", err)
	}
}

func TestCommitRejectsWhenRunMutationLockExists(t *testing.T) {
	svc := newTestService(t)
	runID := "run-commit-lock-busy"
	ticket := "METAWSM-009"
	createRunWithTicketFixture(t, svc, runID, ticket, "ws-commit-lock-busy", model.RunStatusComplete, false)

	lockPath := runMutationLockPath(svc.store.DBPath, runID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("pid=999 operation=commit at=2026-02-08T00:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write mutation lock fixture: %v", err)
	}

	_, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected mutation lock rejection")
	}
	var lockErr *RunMutationInProgressError
	if !errors.As(err, &lockErr) {
		t.Fatalf("expected RunMutationInProgressError, got %T (%v)", err, err)
	}
	if lockErr.Operation != "commit" {
		t.Fatalf("expected operation commit, got %q", lockErr.Operation)
	}
	if lockErr.LockPath != lockPath {
		t.Fatalf("expected lock path %q, got %q", lockPath, lockErr.LockPath)
	}
}

func TestCommitHandlesStaleBaseDirtyTreeWithoutManualRebase(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-stale-base"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-stale-base"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")

	originPath := filepath.Join(homeDir, "remotes", "metawsm-stale-origin.git")
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "-u", "origin", "main")

	runGit(t, repoPath, "checkout", "-B", "stale-work", "main")
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "upstream.txt"), []byte("upstream base move\n"), 0o644); err != nil {
		t.Fatalf("write upstream file: %v", err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "advance base")
	runGit(t, repoPath, "push", "origin", "main")

	runGit(t, repoPath, "checkout", "stale-work")
	if err := os.WriteFile(filepath.Join(repoPath, "local.txt"), []byte("workspace local change\n"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM-009: stale base commit",
	})
	if err != nil {
		t.Fatalf("commit stale-base dirty tree: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one commit repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if strings.TrimSpace(repoResult.CommitSHA) == "" {
		t.Fatalf("expected commit sha for stale-base commit")
	}
	if !strings.Contains(strings.Join(repoResult.Preflight, "\n"), "base_drift=true") {
		t.Fatalf("expected base drift preflight signal, got %+v", repoResult.Preflight)
	}
	if stashList := strings.TrimSpace(runGit(t, repoPath, "stash", "list")); stashList != "" {
		t.Fatalf("expected temporary commit snapshot stash to be dropped, got %q", stashList)
	}
	if show := strings.TrimSpace(runGit(t, repoPath, "show", "--name-only", "--pretty=format:", "HEAD")); !strings.Contains(show, "local.txt") {
		t.Fatalf("expected local.txt in commit, got:\n%s", show)
	}
}

func TestCommitSnapshotReapplyConflictReturnsHelpfulError(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-stash-conflict"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-stash-conflict"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base conflict file: %v", err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "seed conflict file")

	originPath := filepath.Join(homeDir, "remotes", "metawsm-conflict-origin.git")
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "-u", "origin", "main")

	runGit(t, repoPath, "checkout", "-B", "stale-work", "main")
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("upstream-change\n"), 0o644); err != nil {
		t.Fatalf("write upstream conflict file: %v", err)
	}
	runGit(t, repoPath, "add", "conflict.txt")
	runGit(t, repoPath, "commit", "-m", "upstream conflict change")
	runGit(t, repoPath, "push", "origin", "main")

	runGit(t, repoPath, "checkout", "stale-work")
	if err := os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("workspace-change\n"), 0o644); err != nil {
		t.Fatalf("write workspace conflict file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	_, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM-009: conflict commit",
	})
	if err == nil {
		t.Fatalf("expected conflict error from snapshot reapply")
	}
	if !strings.Contains(err.Error(), "failed to reapply workspace snapshot") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
	if !strings.Contains(err.Error(), "snapshot retained at") {
		t.Fatalf("expected snapshot retention guidance in error: %v", err)
	}
	if stashList := strings.TrimSpace(runGit(t, repoPath, "stash", "list")); stashList == "" {
		t.Fatalf("expected stash snapshot to remain for manual recovery")
	}
}

func TestCommitRejectsWhenRequiredTestCommandFails(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-reject-tests"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-reject-tests"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"}, `{"version":2,"git_pr":{"required_checks":["tests"],"test_commands":["false"]}}`)

	_, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected commit validation failure")
	}
	if !strings.Contains(err.Error(), "commit validation failed") {
		t.Fatalf("unexpected commit validation error: %v", err)
	}
	if !strings.Contains(err.Error(), "tests") {
		t.Fatalf("expected tests check name in error: %v", err)
	}
}

func TestCommitAllowsFailedTestWhenRequireAllDisabled(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-require-any"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-require-any"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"}, `{"version":2,"git_pr":{"require_all":false,"required_checks":["tests","forbidden_files"],"test_commands":["false"],"forbidden_file_patterns":["*.pem"]}}`)

	result, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err != nil {
		t.Fatalf("commit with require_all=false should pass when one check passes: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one repo result, got %d", len(result.Repos))
	}
	if strings.TrimSpace(result.Repos[0].CommitSHA) == "" {
		t.Fatalf("expected commit SHA for successful commit")
	}
	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(rows))
	}
	if !strings.Contains(rows[0].ValidationJSON, "\"tests\"") {
		t.Fatalf("expected validation report to include tests result, got %q", rows[0].ValidationJSON)
	}
}

func TestCommitRejectsForbiddenFiles(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-reject-forbidden"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-reject-forbidden"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "secret.pem"), []byte("private\n"), 0o644); err != nil {
		t.Fatalf("write forbidden file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"}, `{"version":2,"git_pr":{"required_checks":["forbidden_files"],"forbidden_file_patterns":["*.pem"]}}`)

	_, err := svc.Commit(t.Context(), CommitOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected commit forbidden-files validation failure")
	}
	if !strings.Contains(err.Error(), "forbidden_files") {
		t.Fatalf("expected forbidden_files check in error: %v", err)
	}
	if !strings.Contains(err.Error(), "secret.pem") {
		t.Fatalf("expected forbidden file path in error: %v", err)
	}
}

func TestOpenPullRequestsRejectsWithoutPreparedCommitMetadata(t *testing.T) {
	svc := newTestService(t)
	runID := "run-pr-reject-empty"
	ticket := "METAWSM-009"
	createRunWithTicketFixture(t, svc, runID, ticket, "ws-pr-reject-empty", model.RunStatusComplete, false)

	_, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected missing prepared-commit metadata error")
	}
	if !strings.Contains(err.Error(), "no prepared commit metadata found") {
		t.Fatalf("unexpected PR preflight error: %v", err)
	}
}

func TestOpenPullRequestsRejectsWhenGitPRModeOff(t *testing.T) {
	svc := newTestService(t)
	runID := "run-pr-reject-mode"
	ticket := "METAWSM-009"
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{ticket},
		Repos:             []string{"metawsm"},
		DocRepo:           "metawsm",
		DocHomeRepo:       "metawsm",
		DocAuthorityMode:  model.DocAuthorityModeWorkspaceActive,
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, `{"git_pr":{"mode":"off"}}`); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	if err := svc.store.UpdateRunStatus(runID, model.RunStatusComplete, ""); err != nil {
		t.Fatalf("set run status complete: %v", err)
	}

	_, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected PR mode-off preflight error")
	}
	if !strings.Contains(err.Error(), "git_pr.mode is off") {
		t.Fatalf("unexpected PR mode-off error: %v", err)
	}
}

func TestOpenPullRequestsRejectsWhenRunMutationLockExists(t *testing.T) {
	svc := newTestService(t)
	runID := "run-pr-lock-busy"
	ticket := "METAWSM-009"
	createRunWithTicketFixture(t, svc, runID, ticket, "ws-pr-lock-busy", model.RunStatusComplete, false)

	lockPath := runMutationLockPath(svc.store.DBPath, runID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("pid=888 operation=commit at=2026-02-08T00:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write mutation lock fixture: %v", err)
	}

	_, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID})
	if err == nil {
		t.Fatalf("expected mutation lock rejection")
	}
	var lockErr *RunMutationInProgressError
	if !errors.As(err, &lockErr) {
		t.Fatalf("expected RunMutationInProgressError, got %T (%v)", err, err)
	}
	if lockErr.Operation != "pr" {
		t.Fatalf("expected operation pr, got %q", lockErr.Operation)
	}
	if lockErr.LockPath != lockPath {
		t.Fatalf("expected lock path %q, got %q", lockPath, lockErr.LockPath)
	}
}

func TestCommitPersistsActorFallbackFromGitIdentity(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-commit-actor-git"
	ticket := "METAWSM-009"
	workspaceName := "ws-commit-actor-git"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("actor fallback test\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	result, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM-009: actor fallback",
	})
	if err != nil {
		t.Fatalf("commit with git actor fallback: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one commit result, got %d", len(result.Repos))
	}
	if result.Repos[0].ActorSource != "git" {
		t.Fatalf("expected git actor source, got %q", result.Repos[0].ActorSource)
	}
	if !strings.Contains(result.Repos[0].Actor, "metawsm test") {
		t.Fatalf("expected git identity actor, got %q", result.Repos[0].Actor)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull request rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(rows))
	}
	if rows[0].Actor != result.Repos[0].Actor {
		t.Fatalf("expected persisted actor %q, got %q", result.Repos[0].Actor, rows[0].Actor)
	}
}

func TestOpenPullRequestsPersistsActorFallbackFromGitIdentity(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	ghScript := "#!/bin/sh\nif [ \"$1\" = \"api\" ] && [ \"$2\" = \"user\" ]; then\n  exit 1\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  echo \"https://github.com/example/metawsm/pull/52\"\n  exit 0\nfi\necho \"unexpected gh invocation: $@\" >&2\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-actor-git"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-actor-git"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	originPath := filepath.Join(homeDir, "remotes", "metawsm-pr-actor-origin.git")
	if err := os.MkdirAll(filepath.Dir(originPath), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	headBranch := "metawsm-009/metawsm/run-pr-actor-git"
	runGit(t, repoPath, "checkout", "-B", headBranch)
	commitSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:         runID,
		Ticket:        ticket,
		Repo:          "metawsm",
		WorkspaceName: workspaceName,
		HeadBranch:    headBranch,
		BaseBranch:    "main",
		CommitSHA:     commitSHA,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request fixture: %v", err)
	}

	result, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID})
	if err != nil {
		t.Fatalf("open pull requests with git actor fallback: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one PR result repo, got %d", len(result.Repos))
	}
	if result.Repos[0].ActorSource != "git" {
		t.Fatalf("expected actor source git, got %q", result.Repos[0].ActorSource)
	}
	if !strings.Contains(result.Repos[0].Actor, "metawsm test") {
		t.Fatalf("expected git actor identity, got %q", result.Repos[0].Actor)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(rows))
	}
	if rows[0].Actor != result.Repos[0].Actor {
		t.Fatalf("expected persisted actor %q, got %q", result.Repos[0].Actor, rows[0].Actor)
	}
}

func TestResolveOperationActorPrefersGitHubWhenAvailable(t *testing.T) {
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	ghScript := "#!/bin/sh\nif [ \"$1\" = \"api\" ] && [ \"$2\" = \"user\" ]; then\n  echo \"gh-actor\"\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	actor, source := resolveOperationActor(t.Context(), "", repoPath)
	if actor != "gh-actor" {
		t.Fatalf("expected gh actor, got %q", actor)
	}
	if source != "gh" {
		t.Fatalf("expected actor source gh, got %q", source)
	}
}

func TestOpenPullRequestsRejectsWhenRequiredTestCommandFails(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-reject-tests"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-reject-tests"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"}, `{"version":2,"git_pr":{"required_checks":["tests"],"test_commands":["false"]}}`)
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:         runID,
		Ticket:        ticket,
		Repo:          "metawsm",
		WorkspaceName: workspaceName,
		HeadBranch:    "metawsm-009/metawsm/run-pr-reject-tests",
		BaseBranch:    "main",
		CommitSHA:     "abc123",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request fixture: %v", err)
	}

	_, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected pull request validation failure")
	}
	if !strings.Contains(err.Error(), "pull request validation failed") {
		t.Fatalf("unexpected pull request validation error: %v", err)
	}
	if !strings.Contains(err.Error(), "tests") {
		t.Fatalf("expected tests check name in error: %v", err)
	}
}

func TestOpenPullRequestsRejectsWhenCleanTreeRequired(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-reject-clean-tree"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-reject-clean-tree"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"}, `{"version":2,"git_pr":{"required_checks":["clean_tree"]}}`)
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:         runID,
		Ticket:        ticket,
		Repo:          "metawsm",
		WorkspaceName: workspaceName,
		HeadBranch:    "metawsm-009/metawsm/run-pr-reject-clean-tree",
		BaseBranch:    "main",
		CommitSHA:     "abc123",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request fixture: %v", err)
	}

	_, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected clean-tree validation failure")
	}
	if !strings.Contains(err.Error(), "clean_tree") {
		t.Fatalf("expected clean_tree check in error: %v", err)
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty-tree detail in error: %v", err)
	}
}

func TestOpenPullRequestsDryRunPreviewsCreateCommand(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-dry"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-dry"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:         runID,
		Ticket:        ticket,
		Repo:          "metawsm",
		WorkspaceName: workspaceName,
		HeadBranch:    "metawsm-009/metawsm/run-pr-dry",
		BaseBranch:    "main",
		CommitSHA:     "abc123",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request fixture: %v", err)
	}

	result, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{
		RunID:  runID,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("open pull requests dry-run: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one pull request repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if len(repoResult.Actions) != 2 {
		t.Fatalf("expected two dry-run actions (push + pr create), got %d", len(repoResult.Actions))
	}
	if !strings.Contains(repoResult.Actions[0], "git '-C'") || !strings.Contains(repoResult.Actions[0], "push") {
		t.Fatalf("expected git push preview, got %q", repoResult.Actions[0])
	}
	if !strings.Contains(repoResult.Actions[1], "gh 'pr' 'create'") {
		t.Fatalf("expected gh pr create preview, got %q", repoResult.Actions[1])
	}
	if repoResult.PRURL != "" {
		t.Fatalf("expected empty pr url for dry-run, got %q", repoResult.PRURL)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(rows))
	}
	if rows[0].PRURL != "" || rows[0].PRNumber != 0 {
		t.Fatalf("expected dry-run not to persist PR URL/number, got url=%q number=%d", rows[0].PRURL, rows[0].PRNumber)
	}
}

func TestOpenPullRequestsCreatesAndPersistsMetadata(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	ghScript := "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  echo \"https://github.com/example/metawsm/pull/42\"\n  exit 0\nfi\necho \"unexpected gh invocation: $@\" >&2\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-real"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-real"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	originPath := filepath.Join(homeDir, "remotes", "metawsm-origin.git")
	if err := os.MkdirAll(filepath.Dir(originPath), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	headBranch := "metawsm-009/metawsm/run-pr-real"
	runGit(t, repoPath, "checkout", "-B", headBranch)
	commitSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})
	if err := svc.UpsertRunPullRequest(model.RunPullRequest{
		RunID:         runID,
		Ticket:        ticket,
		Repo:          "metawsm",
		WorkspaceName: workspaceName,
		HeadBranch:    headBranch,
		BaseBranch:    "main",
		CommitSHA:     commitSHA,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("upsert run pull request fixture: %v", err)
	}

	result, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{
		RunID: runID,
		Actor: "kball",
	})
	if err != nil {
		t.Fatalf("open pull requests: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one pull request repo result, got %d", len(result.Repos))
	}
	repoResult := result.Repos[0]
	if repoResult.PRURL != "https://github.com/example/metawsm/pull/42" {
		t.Fatalf("unexpected PR URL %q", repoResult.PRURL)
	}
	if repoResult.PRNumber != 42 {
		t.Fatalf("expected PR number 42, got %d", repoResult.PRNumber)
	}
	if repoResult.PRState != model.PullRequestStateOpen {
		t.Fatalf("expected PR state open, got %q", repoResult.PRState)
	}
	if remoteHeads := strings.TrimSpace(runGit(t, repoPath, "ls-remote", "--heads", "origin", headBranch)); remoteHeads == "" {
		t.Fatalf("expected pushed branch %q on origin after PR open", headBranch)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(rows))
	}
	row := rows[0]
	if row.PRURL != "https://github.com/example/metawsm/pull/42" {
		t.Fatalf("expected stored PR URL, got %q", row.PRURL)
	}
	if row.PRNumber != 42 {
		t.Fatalf("expected stored PR number 42, got %d", row.PRNumber)
	}
	if row.PRState != model.PullRequestStateOpen {
		t.Fatalf("expected stored PR state open, got %q", row.PRState)
	}
	if row.CredentialMode != "local_user_auth" {
		t.Fatalf("expected credential mode local_user_auth, got %q", row.CredentialMode)
	}
	if row.Actor != "kball" {
		t.Fatalf("expected actor kball, got %q", row.Actor)
	}
}

func TestCommitAndOpenPullRequestsEndToEndPushesBranchAndPersistsMetadata(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	ghScript := "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  echo \"https://github.com/example/metawsm/pull/99\"\n  exit 0\nfi\necho \"unexpected gh invocation: $@\" >&2\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-e2e"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-e2e"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")
	originPath := filepath.Join(homeDir, "remotes", "metawsm-e2e-origin.git")
	if err := os.MkdirAll(filepath.Dir(originPath), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("end-to-end change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	commitResult, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM-009: end-to-end commit",
		Actor:   "kball",
	})
	if err != nil {
		t.Fatalf("commit end-to-end: %v", err)
	}
	if len(commitResult.Repos) != 1 {
		t.Fatalf("expected one commit result repo, got %d", len(commitResult.Repos))
	}
	headBranch := strings.TrimSpace(commitResult.Repos[0].Branch)
	if headBranch == "" {
		t.Fatalf("expected commit branch in result")
	}
	if strings.TrimSpace(commitResult.Repos[0].CommitSHA) == "" {
		t.Fatalf("expected commit sha in result")
	}

	prResult, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{
		RunID: runID,
		Actor: "kball",
	})
	if err != nil {
		t.Fatalf("open pull requests end-to-end: %v", err)
	}
	if len(prResult.Repos) != 1 {
		t.Fatalf("expected one PR result repo, got %d", len(prResult.Repos))
	}
	if prResult.Repos[0].PRURL != "https://github.com/example/metawsm/pull/99" {
		t.Fatalf("unexpected PR URL %q", prResult.Repos[0].PRURL)
	}
	if remoteHeads := strings.TrimSpace(runGit(t, repoPath, "ls-remote", "--heads", "origin", headBranch)); remoteHeads == "" {
		t.Fatalf("expected pushed head branch %q on origin", headBranch)
	}

	rows, err := svc.ListRunPullRequests(runID)
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one persisted PR row, got %d", len(rows))
	}
	row := rows[0]
	if row.PRURL != "https://github.com/example/metawsm/pull/99" {
		t.Fatalf("expected persisted PR URL, got %q", row.PRURL)
	}
	if row.PRState != model.PullRequestStateOpen {
		t.Fatalf("expected persisted PR state open, got %q", row.PRState)
	}
	if row.CredentialMode != "local_user_auth" {
		t.Fatalf("expected local_user_auth credential mode, got %q", row.CredentialMode)
	}
	if row.Actor != "kball" {
		t.Fatalf("expected actor kball, got %q", row.Actor)
	}
}

func TestCommitAndOpenPullRequestsEndToEndHandlesStaleBaseWorkspace(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	ghScript := "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  echo \"https://github.com/example/metawsm/pull/101\"\n  exit 0\nfi\necho \"unexpected gh invocation: $@\" >&2\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-pr-e2e-stale-base"
	ticket := "METAWSM-009"
	workspaceName := "ws-pr-e2e-stale-base"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	initGitRepo(t, repoPath)
	runGit(t, repoPath, "checkout", "-B", "main")

	originPath := filepath.Join(homeDir, "remotes", "metawsm-e2e-stale-origin.git")
	if err := os.MkdirAll(filepath.Dir(originPath), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}
	runGit(t, repoPath, "init", "--bare", originPath)
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "-u", "origin", "main")

	runGit(t, repoPath, "checkout", "-B", "workspace-stale", "main")
	runGit(t, repoPath, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "upstream.txt"), []byte("base moved ahead\n"), 0o644); err != nil {
		t.Fatalf("write upstream base move: %v", err)
	}
	runGit(t, repoPath, "add", "upstream.txt")
	runGit(t, repoPath, "commit", "-m", "advance base")
	runGit(t, repoPath, "push", "origin", "main")
	runGit(t, repoPath, "checkout", "workspace-stale")

	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("stale-base local feature change\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}

	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	commitResult, err := svc.Commit(t.Context(), CommitOptions{
		RunID:   runID,
		Message: "METAWSM-009: stale-base e2e commit",
	})
	if err != nil {
		t.Fatalf("commit stale-base e2e: %v", err)
	}
	if len(commitResult.Repos) != 1 {
		t.Fatalf("expected one commit result repo, got %d", len(commitResult.Repos))
	}
	headBranch := strings.TrimSpace(commitResult.Repos[0].Branch)
	if headBranch == "" {
		t.Fatalf("expected head branch from commit result")
	}
	if !strings.Contains(strings.Join(commitResult.Repos[0].Preflight, "\n"), "base_drift=true") {
		t.Fatalf("expected stale-base preflight signal, got %+v", commitResult.Repos[0].Preflight)
	}

	prResult, err := svc.OpenPullRequests(t.Context(), PullRequestOptions{
		RunID: runID,
	})
	if err != nil {
		t.Fatalf("open pull requests stale-base e2e: %v", err)
	}
	if len(prResult.Repos) != 1 {
		t.Fatalf("expected one PR result repo, got %d", len(prResult.Repos))
	}
	if prResult.Repos[0].PRURL != "https://github.com/example/metawsm/pull/101" {
		t.Fatalf("unexpected PR URL %q", prResult.Repos[0].PRURL)
	}
	if remoteHeads := strings.TrimSpace(runGit(t, repoPath, "ls-remote", "--heads", "origin", headBranch)); remoteHeads == "" {
		t.Fatalf("expected pushed head branch %q on origin", headBranch)
	}
}

func TestIterateDryRunIncludesFeedbackAndRestartActions(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	svc := newTestService(t)
	homeDir := setupWorkspaceConfigRoot(t)
	runID := "run-iterate-dry"
	ticket := "METAWSM-010"
	workspaceName := "ws-iterate-dry"
	workspacePath := filepath.Join(homeDir, "workspaces", workspaceName)
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, ".metawsm", "implementation-complete.json"), []byte(`{"status":"done"}`), 0o644); err != nil {
		t.Fatalf("write completion marker: %v", err)
	}
	ticketDir := filepath.Join(workspacePath, "metawsm", "ttmp", "2026", "02", "07", strings.ToLower(ticket)+"--feedback-test", "reference")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("mkdir ticket reference dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir doc repo dir: %v", err)
	}
	writeWorkspaceConfig(t, workspaceName, workspacePath)
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, model.RunStatusComplete, false, []string{"metawsm"})

	result, err := svc.Iterate(t.Context(), IterateOptions{
		RunID:    runID,
		Feedback: "Please update the status page card layout and tighten tests.",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("iterate dry-run: %v", err)
	}
	if result.RunID != runID {
		t.Fatalf("expected run id %s, got %s", runID, result.RunID)
	}
	joined := strings.Join(result.Actions, "\n")
	if !strings.Contains(joined, ".metawsm/operator-feedback.md") {
		t.Fatalf("expected operator feedback action, got:\n%s", joined)
	}
	if !strings.Contains(joined, "99-operator-feedback.md") {
		t.Fatalf("expected ticket feedback doc action, got:\n%s", joined)
	}
	if !strings.Contains(joined, "tmux new-session -d -s") {
		t.Fatalf("expected restart action in iterate dry-run, got:\n%s", joined)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".metawsm", "implementation-complete.json")); err != nil {
		t.Fatalf("expected completion marker to remain during dry-run, err=%v", err)
	}
}

func TestRecordIterationFeedbackWritesAndClearsSignals(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, ".metawsm"), 0o755); err != nil {
		t.Fatalf("mkdir .metawsm: %v", err)
	}
	signalFiles := []string{
		filepath.Join(workspacePath, ".metawsm", "implementation-complete.json"),
		filepath.Join(workspacePath, ".metawsm", "validation-result.json"),
		filepath.Join(workspacePath, ".metawsm", "guidance-request.json"),
		filepath.Join(workspacePath, ".metawsm", "guidance-response.json"),
	}
	for _, path := range signalFiles {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write signal file %s: %v", path, err)
		}
	}

	ticket := "METAWSM-011"
	referenceDir := filepath.Join(workspacePath, "metawsm", "ttmp", "2026", "02", "07", strings.ToLower(ticket)+"--iteration-feedback", "reference")
	if err := os.MkdirAll(referenceDir, 0o755); err != nil {
		t.Fatalf("mkdir reference dir: %v", err)
	}
	now := time.Date(2026, 2, 7, 10, 15, 0, 0, time.UTC)
	feedback := "Address the button spacing regression and add a request spec."
	if _, err := recordIterationFeedback(workspacePath, "metawsm", []string{"metawsm"}, []string{ticket}, feedback, now, false); err != nil {
		t.Fatalf("record iteration feedback: %v", err)
	}

	for _, path := range signalFiles {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected signal file removed %s, stat err=%v", path, err)
		}
	}

	mainFeedback := filepath.Join(workspacePath, ".metawsm", "operator-feedback.md")
	mainBytes, err := os.ReadFile(mainFeedback)
	if err != nil {
		t.Fatalf("read main feedback file: %v", err)
	}
	mainText := string(mainBytes)
	if !strings.Contains(mainText, feedback) {
		t.Fatalf("expected main feedback text in file, got:\n%s", mainText)
	}
	if !strings.Contains(mainText, "# Operator Feedback") {
		t.Fatalf("expected main feedback header, got:\n%s", mainText)
	}

	ticketFeedback := filepath.Join(referenceDir, "99-operator-feedback.md")
	ticketBytes, err := os.ReadFile(ticketFeedback)
	if err != nil {
		t.Fatalf("read ticket feedback file: %v", err)
	}
	ticketText := string(ticketBytes)
	if !strings.Contains(ticketText, feedback) {
		t.Fatalf("expected ticket feedback text in file, got:\n%s", ticketText)
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
	createBootstrapRunFixtureWithRepos(t, svc, runID, workspaceName, []string{"metawsm"})
}

func createBootstrapRunFixtureWithRepos(t *testing.T, svc *Service, runID string, workspaceName string, repos []string) {
	t.Helper()
	docRepo := ""
	if len(repos) > 0 {
		docRepo = repos[0]
	}
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{"METAWSM-002"},
		Repos:             repos,
		DocRepo:           docRepo,
		DocHomeRepo:       docRepo,
		DocAuthorityMode:  model.DocAuthorityModeWorkspaceActive,
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
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

func createRunWithTicketFixture(t *testing.T, svc *Service, runID string, ticket string, workspaceName string, status model.RunStatus, dryRun bool) {
	t.Helper()
	createRunWithTicketFixtureWithRepos(t, svc, runID, ticket, workspaceName, status, dryRun, []string{"metawsm"})
}

func createRunWithTicketFixtureWithRepos(t *testing.T, svc *Service, runID string, ticket string, workspaceName string, status model.RunStatus, dryRun bool, repos []string) {
	t.Helper()
	createRunWithTicketFixtureWithReposAndPolicy(t, svc, runID, ticket, workspaceName, status, dryRun, repos, `{"version":1}`)
}

func createRunWithTicketFixtureWithReposAndPolicy(t *testing.T, svc *Service, runID string, ticket string, workspaceName string, status model.RunStatus, dryRun bool, repos []string, policyJSON string) {
	t.Helper()
	docRepo := ""
	if len(repos) > 0 {
		docRepo = repos[0]
	}
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           []string{ticket},
		Repos:             repos,
		DocRepo:           docRepo,
		DocHomeRepo:       docRepo,
		DocAuthorityMode:  model.DocAuthorityModeWorkspaceActive,
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		DryRun:            dryRun,
		CreatedAt:         time.Now(),
	}
	if strings.TrimSpace(policyJSON) == "" {
		policyJSON = `{"version":1}`
	}
	if err := svc.store.CreateRun(spec, policyJSON); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	if status != model.RunStatusCreated {
		if err := svc.store.UpdateRunStatus(runID, status, ""); err != nil {
			t.Fatalf("set run status fixture: %v", err)
		}
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
}

func createRunWithTicketsFixture(t *testing.T, svc *Service, runID string, tickets []string, workspaceByTicket map[string]string, status model.RunStatus, policyJSON string) {
	t.Helper()
	if len(tickets) == 0 {
		t.Fatalf("tickets are required")
	}
	if strings.TrimSpace(policyJSON) == "" {
		policyJSON = `{"version":2}`
	}

	agents := make([]model.AgentSpec, 0, len(tickets))
	for idx := range tickets {
		agents = append(agents, model.AgentSpec{
			Name:    fmt.Sprintf("agent-%d", idx+1),
			Command: "bash",
		})
	}
	spec := model.RunSpec{
		RunID:             runID,
		Mode:              model.RunModeBootstrap,
		Tickets:           tickets,
		Repos:             []string{"metawsm"},
		DocRepo:           "metawsm",
		DocHomeRepo:       "metawsm",
		DocAuthorityMode:  model.DocAuthorityModeWorkspaceActive,
		DocSeedMode:       model.DocSeedModeCopyFromRepoOnStart,
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            agents,
		PolicyPath:        ".metawsm/policy.json",
		DryRun:            false,
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, policyJSON); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	if status != model.RunStatusCreated {
		if err := svc.store.UpdateRunStatus(runID, status, ""); err != nil {
			t.Fatalf("set run status fixture: %v", err)
		}
	}

	now := time.Now()
	steps := make([]model.PlanStep, 0, len(tickets))
	for idx, ticket := range tickets {
		workspaceName := strings.TrimSpace(workspaceByTicket[ticket])
		if workspaceName == "" {
			t.Fatalf("workspace mapping missing for ticket %s", ticket)
		}
		agentName := fmt.Sprintf("agent-%d", idx+1)
		if err := svc.store.UpsertAgent(model.AgentRecord{
			RunID:          runID,
			Name:           agentName,
			WorkspaceName:  workspaceName,
			SessionName:    fmt.Sprintf("%s-%s", agentName, workspaceName),
			Status:         model.AgentStatusRunning,
			HealthState:    model.HealthStateHealthy,
			LastActivityAt: &now,
			LastProgressAt: &now,
		}); err != nil {
			t.Fatalf("upsert multi-ticket agent fixture: %v", err)
		}
		steps = append(steps, model.PlanStep{
			Index:         idx + 1,
			Name:          fmt.Sprintf("workspace-create-%s", workspaceName),
			Kind:          "workspace",
			Ticket:        ticket,
			WorkspaceName: workspaceName,
			Agent:         agentName,
			Status:        model.StepStatusDone,
		})
	}
	if err := svc.store.SaveSteps(runID, steps); err != nil {
		t.Fatalf("save multi-ticket step fixtures: %v", err)
	}
}

func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".gitkeep"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write .gitkeep: %v", err)
	}
	runGit(t, path, "init")
	runGit(t, path, "config", "user.email", "metawsm-test@example.com")
	runGit(t, path, "config", "user.name", "metawsm test")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "init")
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

func upsertDocSyncStateFixture(t *testing.T, svc *Service, runID string, ticket string, workspaceName string, status model.DocSyncStatus, revision string) {
	t.Helper()
	if err := svc.store.UpsertDocSyncState(model.DocSyncState{
		RunID:            runID,
		Ticket:           ticket,
		WorkspaceName:    workspaceName,
		DocHomeRepo:      "metawsm",
		DocAuthorityMode: string(model.DocAuthorityModeWorkspaceActive),
		DocSeedMode:      string(model.DocSeedModeCopyFromRepoOnStart),
		Status:           status,
		Revision:         revision,
		UpdatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("upsert doc sync state fixture: %v", err)
	}
}

func writeTicketDocDirFixture(t *testing.T, workspacePath string, ticket string) {
	t.Helper()
	ticketDir := filepath.Join(workspacePath, "ttmp", "2026", "02", "08", strings.ToLower(ticket)+"--fixture")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("mkdir ticket doc fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "README.md"), []byte("# Fixture\n"), 0o644); err != nil {
		t.Fatalf("write ticket doc fixture file: %v", err)
	}
}

func runGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
