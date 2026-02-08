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
		Mode:              model.RunModeBootstrap,
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
	brief := model.RunBrief{
		RunID:        spec.RunID,
		Ticket:       "METAWSM-001",
		Goal:         "Implement bootstrap flow",
		Scope:        "cmd and orchestrator",
		DoneCriteria: "tests pass",
		Constraints:  "no policy regressions",
		MergeIntent:  "default merge flow",
		QA: []model.IntakeQA{
			{Question: "Goal?", Answer: "Implement bootstrap flow"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.UpsertRunBrief(brief); err != nil {
		t.Fatalf("upsert run brief: %v", err)
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

	runs, err := s.ListRuns()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 listed run, got %d", len(runs))
	}
	if runs[0].RunID != spec.RunID {
		t.Fatalf("expected listed run id %s, got %s", spec.RunID, runs[0].RunID)
	}

	loadedBrief, err := s.GetRunBrief(spec.RunID)
	if err != nil {
		t.Fatalf("get run brief: %v", err)
	}
	if loadedBrief == nil {
		t.Fatalf("expected run brief")
	}
	if loadedBrief.Goal != brief.Goal {
		t.Fatalf("expected run brief goal %q, got %q", brief.Goal, loadedBrief.Goal)
	}
	if len(loadedBrief.QA) != 1 || loadedBrief.QA[0].Answer != "Implement bootstrap flow" {
		t.Fatalf("expected run brief QA to round-trip")
	}

	reqID, err := s.AddGuidanceRequest(model.GuidanceRequest{
		RunID:         spec.RunID,
		WorkspaceName: "metawsm-001",
		AgentName:     "agent",
		Question:      "Need schema decision",
		Context:       "migration",
		Status:        model.GuidanceStatusPending,
	})
	if err != nil {
		t.Fatalf("add guidance request: %v", err)
	}
	if reqID == 0 {
		t.Fatalf("expected non-zero guidance request id")
	}

	pending, err := s.ListGuidanceRequests(spec.RunID, model.GuidanceStatusPending)
	if err != nil {
		t.Fatalf("list pending guidance: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending guidance request, got %d", len(pending))
	}

	if err := s.MarkGuidanceAnswered(reqID, "Use workspace sentinel"); err != nil {
		t.Fatalf("mark guidance answered: %v", err)
	}

	answered, err := s.ListGuidanceRequests(spec.RunID, model.GuidanceStatusAnswered)
	if err != nil {
		t.Fatalf("list answered guidance: %v", err)
	}
	if len(answered) != 1 {
		t.Fatalf("expected one answered guidance request, got %d", len(answered))
	}
	if answered[0].Answer != "Use workspace sentinel" {
		t.Fatalf("expected stored guidance answer, got %q", answered[0].Answer)
	}

	if err := s.UpsertDocSyncState(model.DocSyncState{
		RunID:            spec.RunID,
		Ticket:           "METAWSM-001",
		WorkspaceName:    "metawsm-001",
		DocHomeRepo:      "metawsm",
		DocAuthorityMode: "workspace_active",
		DocSeedMode:      "copy_from_repo_on_start",
		Status:           model.DocSyncStatusSynced,
		Revision:         "12345",
		UpdatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("upsert doc sync state: %v", err)
	}
	docSyncStates, err := s.ListDocSyncStates(spec.RunID)
	if err != nil {
		t.Fatalf("list doc sync states: %v", err)
	}
	if len(docSyncStates) != 1 {
		t.Fatalf("expected one doc sync state, got %d", len(docSyncStates))
	}
	if docSyncStates[0].Revision != "12345" {
		t.Fatalf("expected doc sync revision 12345, got %q", docSyncStates[0].Revision)
	}

	if err := s.UpdateRunDocFreshnessRevision(spec.RunID, "67890"); err != nil {
		t.Fatalf("update run doc freshness revision: %v", err)
	}
	_, updatedSpecJSON, _, err := s.GetRun(spec.RunID)
	if err != nil {
		t.Fatalf("get run after freshness revision update: %v", err)
	}
	var updatedSpec model.RunSpec
	if err := json.Unmarshal([]byte(updatedSpecJSON), &updatedSpec); err != nil {
		t.Fatalf("unmarshal updated spec: %v", err)
	}
	if updatedSpec.DocFreshnessRevision != "67890" {
		t.Fatalf("expected doc freshness revision 67890, got %q", updatedSpec.DocFreshnessRevision)
	}

	latestRunID, err := s.FindLatestRunIDByTicket("METAWSM-001")
	if err != nil {
		t.Fatalf("find latest run id by ticket: %v", err)
	}
	if latestRunID != spec.RunID {
		t.Fatalf("expected latest run id %s, got %s", spec.RunID, latestRunID)
	}
}

func TestOperatorRunStatePersistsAcrossStoreReopen(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	lastRestart := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	cooldownUntil := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := s.UpsertOperatorRunState(model.OperatorRunState{
		RunID:           "run-operator-state",
		RestartAttempts: 2,
		LastRestartAt:   &lastRestart,
		CooldownUntil:   &cooldownUntil,
		UpdatedAt:       time.Now(),
	}); err != nil {
		t.Fatalf("upsert operator run state: %v", err)
	}

	reopened := NewSQLiteStore(dbPath)
	if err := reopened.Init(); err != nil {
		t.Fatalf("re-init reopened store: %v", err)
	}

	state, err := reopened.GetOperatorRunState("run-operator-state")
	if err != nil {
		t.Fatalf("get operator run state: %v", err)
	}
	if state == nil {
		t.Fatalf("expected operator run state to persist")
	}
	if state.RestartAttempts != 2 {
		t.Fatalf("expected restart attempts 2, got %d", state.RestartAttempts)
	}
	if state.LastRestartAt == nil || !state.LastRestartAt.Equal(lastRestart) {
		t.Fatalf("expected last restart at %s, got %v", lastRestart.Format(time.RFC3339), state.LastRestartAt)
	}
	if state.CooldownUntil == nil || !state.CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("expected cooldown until %s, got %v", cooldownUntil.Format(time.RFC3339), state.CooldownUntil)
	}
}

func TestRunPullRequestStatePersistsAcrossStoreReopen(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	if err := s.UpsertRunPullRequest(model.RunPullRequest{
		RunID:          "run-pr-1",
		Ticket:         "METAWSM-009",
		Repo:           "metawsm",
		WorkspaceName:  "metawsm-009-ws",
		HeadBranch:     "METAWSM-009/metawsm/run-pr-1",
		BaseBranch:     "main",
		RemoteName:     "origin",
		CommitSHA:      "abc123",
		PRNumber:       42,
		PRURL:          "https://github.com/example/metawsm/pull/42",
		PRState:        model.PullRequestStateOpen,
		CredentialMode: "local_user_auth",
		Actor:          "kball",
		ValidationJSON: `{"checks":[{"name":"tests","status":"passed"}]}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("upsert run pull request: %v", err)
	}

	reopened := NewSQLiteStore(dbPath)
	if err := reopened.Init(); err != nil {
		t.Fatalf("re-init reopened store: %v", err)
	}

	rows, err := reopened.ListRunPullRequests("run-pr-1")
	if err != nil {
		t.Fatalf("list run pull requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one run pull request row, got %d", len(rows))
	}
	row := rows[0]
	if row.Ticket != "METAWSM-009" || row.Repo != "metawsm" {
		t.Fatalf("unexpected ticket/repo: %s/%s", row.Ticket, row.Repo)
	}
	if row.PRNumber != 42 || row.PRState != model.PullRequestStateOpen {
		t.Fatalf("unexpected pr metadata: number=%d state=%s", row.PRNumber, row.PRState)
	}
	if row.CredentialMode != "local_user_auth" || row.Actor != "kball" {
		t.Fatalf("unexpected auth metadata: mode=%q actor=%q", row.CredentialMode, row.Actor)
	}
	if row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() {
		t.Fatalf("expected created_at and updated_at to be populated")
	}
}
