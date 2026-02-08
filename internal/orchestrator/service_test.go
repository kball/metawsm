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
	if err := svc.syncBootstrapSignals(t.Context(), "run-agent-failed", model.RunStatusRunning, []model.AgentRecord{
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

	err := svc.Close(t.Context(), CloseOptions{RunID: runID, DryRun: true})
	if err == nil {
		t.Fatalf("expected close to fail when nested repo is dirty")
	}
	if !strings.Contains(err.Error(), "repo-b") {
		t.Fatalf("expected dirty nested repo path in error, got: %v", err)
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
		WorkspaceStrategy: model.WorkspaceStrategyCreate,
		Agents:            []model.AgentSpec{{Name: "agent", Command: "bash"}},
		PolicyPath:        ".metawsm/policy.json",
		DryRun:            dryRun,
		CreatedAt:         time.Now(),
	}
	if err := svc.store.CreateRun(spec, `{"version":1}`); err != nil {
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
