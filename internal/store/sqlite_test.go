package store

import (
	"encoding/json"
	"io"
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

func TestSQLiteStoreRetriesBusyWriteLock(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	s.BusyTimeoutMS = 50
	s.BusyRetryCount = 10
	s.BusyRetryBackoffMS = 50
	retries := 0
	s.retryObserver = func(operation string, attempt int, err error) {
		if operation == "exec" {
			retries++
		}
	}
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := s.execSQL("CREATE TABLE IF NOT EXISTS lock_test (id INTEGER PRIMARY KEY);"); err != nil {
		t.Fatalf("create lock_test table: %v", err)
	}

	locker := exec.Command("sqlite3", dbPath)
	lockerStdin, err := locker.StdinPipe()
	if err != nil {
		t.Fatalf("open locker stdin: %v", err)
	}
	if err := locker.Start(); err != nil {
		t.Fatalf("start locker sqlite3 process: %v", err)
	}
	defer func() {
		_ = locker.Process.Kill()
		_, _ = locker.Process.Wait()
	}()

	if _, err := io.WriteString(lockerStdin, "BEGIN IMMEDIATE;\nINSERT INTO lock_test (id) VALUES (1);\n"); err != nil {
		t.Fatalf("seed locker transaction: %v", err)
	}
	time.Sleep(120 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		time.Sleep(350 * time.Millisecond)
		_, _ = io.WriteString(lockerStdin, "COMMIT;\n.quit\n")
		_ = lockerStdin.Close()
		_ = locker.Wait()
		close(done)
	}()

	if err := s.execSQL("INSERT INTO lock_test (id) VALUES (2);"); err != nil {
		t.Fatalf("exec sql with retry while locked: %v", err)
	}
	<-done

	rows, err := s.queryJSON("SELECT COUNT(*) AS c FROM lock_test;")
	if err != nil {
		t.Fatalf("query lock_test count: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if asInt(rows[0]["c"]) != 2 {
		t.Fatalf("expected two inserted rows after lock release, got %d", asInt(rows[0]["c"]))
	}
	if retries == 0 {
		t.Fatalf("expected at least one retry when DB was locked")
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

func TestRunReviewFeedbackPersistsAcrossStoreReopen(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	if err := s.UpsertRunReviewFeedback(model.RunReviewFeedback{
		RunID:         "run-review-1",
		Ticket:        "METAWSM-009",
		Repo:          "metawsm",
		WorkspaceName: "metawsm-009-ws",
		PRNumber:      42,
		PRURL:         "https://github.com/example/metawsm/pull/42",
		SourceType:    model.ReviewFeedbackSourceTypePRReviewComment,
		SourceID:      "9001",
		SourceURL:     "https://github.com/example/metawsm/pull/42#discussion_r9001",
		Author:        "reviewer",
		Body:          "Please add regression coverage for this path.",
		FilePath:      "internal/orchestrator/service.go",
		Line:          1044,
		Status:        model.ReviewFeedbackStatusQueued,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastSeenAt:    now,
	}); err != nil {
		t.Fatalf("upsert run review feedback: %v", err)
	}

	reopened := NewSQLiteStore(dbPath)
	if err := reopened.Init(); err != nil {
		t.Fatalf("re-init reopened store: %v", err)
	}

	rows, err := reopened.ListRunReviewFeedback("run-review-1")
	if err != nil {
		t.Fatalf("list run review feedback: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one run review feedback row, got %d", len(rows))
	}
	row := rows[0]
	if row.SourceType != model.ReviewFeedbackSourceTypePRReviewComment || row.SourceID != "9001" {
		t.Fatalf("unexpected source metadata: %s/%s", row.SourceType, row.SourceID)
	}
	if row.Status != model.ReviewFeedbackStatusQueued {
		t.Fatalf("expected queued status, got %s", row.Status)
	}

	addressedAt := now.Add(10 * time.Minute).Truncate(time.Second)
	if err := reopened.UpdateRunReviewFeedbackStatus(
		"run-review-1",
		"METAWSM-009",
		"metawsm",
		42,
		model.ReviewFeedbackSourceTypePRReviewComment,
		"9001",
		model.ReviewFeedbackStatusAddressed,
		"",
		&addressedAt,
	); err != nil {
		t.Fatalf("update run review feedback status: %v", err)
	}

	addressedRows, err := reopened.ListRunReviewFeedbackByStatus("run-review-1", model.ReviewFeedbackStatusAddressed)
	if err != nil {
		t.Fatalf("list addressed review feedback: %v", err)
	}
	if len(addressedRows) != 1 {
		t.Fatalf("expected one addressed feedback row, got %d", len(addressedRows))
	}
	if addressedRows[0].AddressedAt == nil {
		t.Fatalf("expected addressed_at to be populated")
	}
	if !addressedRows[0].AddressedAt.Equal(addressedAt) {
		t.Fatalf("unexpected addressed_at: got %s want %s", addressedRows[0].AddressedAt.Format(time.RFC3339), addressedAt.Format(time.RFC3339))
	}
}

func TestRunReviewFeedbackUpsertDedupesByCompositeKey(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	firstSeen := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	secondSeen := firstSeen.Add(7 * time.Minute).Truncate(time.Second)
	first := model.RunReviewFeedback{
		RunID:      "run-review-2",
		Ticket:     "METAWSM-009",
		Repo:       "metawsm",
		PRNumber:   99,
		SourceType: model.ReviewFeedbackSourceTypePRReviewComment,
		SourceID:   "r-12345",
		Author:     "reviewer-a",
		Body:       "Original comment body.",
		Status:     model.ReviewFeedbackStatusNew,
		CreatedAt:  firstSeen,
		UpdatedAt:  firstSeen,
		LastSeenAt: firstSeen,
	}
	if err := s.UpsertRunReviewFeedback(first); err != nil {
		t.Fatalf("upsert initial review feedback: %v", err)
	}

	second := first
	second.Author = "reviewer-b"
	second.Body = "Updated comment body after edit."
	second.Status = model.ReviewFeedbackStatusQueued
	second.UpdatedAt = secondSeen
	second.LastSeenAt = secondSeen
	if err := s.UpsertRunReviewFeedback(second); err != nil {
		t.Fatalf("upsert updated review feedback: %v", err)
	}

	rows, err := s.ListRunReviewFeedback("run-review-2")
	if err != nil {
		t.Fatalf("list run review feedback: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one deduped review feedback row, got %d", len(rows))
	}
	row := rows[0]
	if row.Author != "reviewer-b" || row.Body != "Updated comment body after edit." {
		t.Fatalf("expected latest row data to win, got author=%q body=%q", row.Author, row.Body)
	}
	if row.Status != model.ReviewFeedbackStatusQueued {
		t.Fatalf("expected queued status, got %s", row.Status)
	}
	if !row.LastSeenAt.Equal(secondSeen) {
		t.Fatalf("expected last_seen_at %s, got %s", secondSeen.Format(time.RFC3339), row.LastSeenAt.Format(time.RFC3339))
	}
}

func TestForumThreadLifecycleRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	opened, err := s.ForumOpenThread(model.ForumOpenThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-open-1",
			EventType:    "forum.thread.opened",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			RunID:        "run-1",
			Ticket:       "METAWSM-008",
			AgentName:    "agent-a",
			ActorType:    model.ForumActorAgent,
			ActorName:    "agent-a",
		},
		Title:    "Need operator API direction",
		Body:     "Should we keep polling or switch to events?",
		Priority: model.ForumPriorityNormal,
	})
	if err != nil {
		t.Fatalf("open forum thread: %v", err)
	}
	if opened == nil || opened.ThreadID != "thread-1" {
		t.Fatalf("expected opened thread thread-1, got %#v", opened)
	}
	if opened.PostsCount != 1 {
		t.Fatalf("expected posts_count=1 after open, got %d", opened.PostsCount)
	}

	added, err := s.ForumAddPost(model.ForumAddPostCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-post-1",
			EventType:    "forum.post.added",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorOperator,
			ActorName:    "operator-a",
		},
		Body: "Move to event stream from day one.",
	})
	if err != nil {
		t.Fatalf("add forum post: %v", err)
	}
	if added.PostsCount != 2 {
		t.Fatalf("expected posts_count=2 after add, got %d", added.PostsCount)
	}

	duplicatePost, err := s.ForumAddPost(model.ForumAddPostCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-post-1",
			EventType:    "forum.post.added",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorOperator,
			ActorName:    "operator-a",
		},
		Body: "Move to event stream from day one.",
	})
	if err != nil {
		t.Fatalf("duplicate forum post should be idempotent: %v", err)
	}
	if duplicatePost.PostsCount != 2 {
		t.Fatalf("expected posts_count to remain 2 after duplicate event, got %d", duplicatePost.PostsCount)
	}

	if _, err := s.ForumAssignThread(model.ForumAssignThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-assign-1",
			EventType:    "forum.assigned",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorOperator,
			ActorName:    "operator-a",
		},
		AssigneeType: model.ForumActorHuman,
		AssigneeName: "kball",
	}); err != nil {
		t.Fatalf("assign forum thread: %v", err)
	}

	if _, err := s.ForumChangeState(model.ForumChangeStateCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-state-1",
			EventType:    "forum.state.changed",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorOperator,
			ActorName:    "operator-a",
		},
		ToState: model.ForumThreadStateWaitingHuman,
	}); err != nil {
		t.Fatalf("change forum state: %v", err)
	}

	if _, err := s.ForumSetPriority(model.ForumSetPriorityCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-priority-1",
			EventType:    "forum.priority.changed",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorOperator,
			ActorName:    "operator-a",
		},
		Priority: model.ForumPriorityHigh,
	}); err != nil {
		t.Fatalf("set forum priority: %v", err)
	}

	closed, err := s.ForumCloseThread(model.ForumCloseThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-close-1",
			EventType:    "forum.thread.closed",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-1",
			ActorType:    model.ForumActorHuman,
			ActorName:    "kball",
		},
	})
	if err != nil {
		t.Fatalf("close forum thread: %v", err)
	}
	if closed.State != model.ForumThreadStateClosed {
		t.Fatalf("expected closed state, got %s", closed.State)
	}
	if closed.ClosedAt == nil {
		t.Fatalf("expected closed_at to be set")
	}

	threads, err := s.ListForumThreads(model.ForumThreadFilter{Ticket: "METAWSM-008", Limit: 10})
	if err != nil {
		t.Fatalf("list forum threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one forum thread, got %d", len(threads))
	}
	if threads[0].Priority != model.ForumPriorityHigh {
		t.Fatalf("expected high priority thread, got %s", threads[0].Priority)
	}
	if threads[0].AssigneeName != "kball" {
		t.Fatalf("expected assignee kball, got %q", threads[0].AssigneeName)
	}

	stats, err := s.ListForumThreadStats("METAWSM-008", "")
	if err != nil {
		t.Fatalf("list forum stats: %v", err)
	}
	if len(stats) == 0 {
		t.Fatalf("expected forum stats rows")
	}
	if stats[0].Ticket != "METAWSM-008" {
		t.Fatalf("unexpected stats ticket %q", stats[0].Ticket)
	}

	events, err := s.WatchForumEvents("METAWSM-008", 0, 20)
	if err != nil {
		t.Fatalf("watch forum events: %v", err)
	}
	if len(events) != 6 {
		t.Fatalf("expected 6 forum events, got %d", len(events))
	}
	if events[0].Sequence <= 0 {
		t.Fatalf("expected positive event sequence, got %d", events[0].Sequence)
	}
	if events[len(events)-1].Sequence <= events[0].Sequence {
		t.Fatalf("expected monotonic increasing event sequences")
	}
}

func TestForumControlThreadMappingUpsertAndLookup(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if err := s.UpsertForumControlThread(model.ForumControlThread{
		RunID:     "run-ctrl-1",
		AgentName: "agent-a",
		Ticket:    "METAWSM-010",
		ThreadID:  "fctrl-run-ctrl-1-agent-a",
	}); err != nil {
		t.Fatalf("upsert control mapping: %v", err)
	}

	mapping, err := s.GetForumControlThread("run-ctrl-1", "agent-a")
	if err != nil {
		t.Fatalf("get control mapping: %v", err)
	}
	if mapping == nil {
		t.Fatalf("expected control mapping row")
	}
	if mapping.ThreadID != "fctrl-run-ctrl-1-agent-a" {
		t.Fatalf("unexpected thread id %q", mapping.ThreadID)
	}

	if err := s.UpsertForumControlThread(model.ForumControlThread{
		RunID:     "run-ctrl-1",
		AgentName: "agent-a",
		Ticket:    "METAWSM-010",
		ThreadID:  "fctrl-run-ctrl-1-agent-a-v2",
	}); err != nil {
		t.Fatalf("upsert updated control mapping: %v", err)
	}

	updated, err := s.GetForumControlThread("run-ctrl-1", "agent-a")
	if err != nil {
		t.Fatalf("get updated control mapping: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected updated control mapping row")
	}
	if updated.ThreadID != "fctrl-run-ctrl-1-agent-a-v2" {
		t.Fatalf("expected updated thread id, got %q", updated.ThreadID)
	}
}

func TestApplyForumEventProjectionsIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := s.ForumOpenThread(model.ForumOpenThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-open-proj-1",
			EventType:    "forum.thread.opened",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-proj-1",
			RunID:        "run-proj-1",
			Ticket:       "METAWSM-010",
			AgentName:    "agent-a",
			ActorType:    model.ForumActorAgent,
			ActorName:    "agent-a",
		},
		Title:    "Projection test thread",
		Body:     "Initial post",
		Priority: model.ForumPriorityHigh,
	}); err != nil {
		t.Fatalf("open forum thread: %v", err)
	}

	event, err := s.GetForumEvent("evt-open-proj-1")
	if err != nil {
		t.Fatalf("get forum event: %v", err)
	}
	if event == nil {
		t.Fatalf("expected stored forum event")
	}

	if err := s.ApplyForumEventProjections(*event); err != nil {
		t.Fatalf("apply projection first pass: %v", err)
	}
	if err := s.ApplyForumEventProjections(*event); err != nil {
		t.Fatalf("apply projection second pass: %v", err)
	}

	thread, err := s.GetForumThread("thread-proj-1")
	if err != nil {
		t.Fatalf("get projected thread: %v", err)
	}
	if thread == nil {
		t.Fatalf("expected projected thread row")
	}
	if thread.State != model.ForumThreadStateNew {
		t.Fatalf("expected state=new, got %s", thread.State)
	}
	if thread.PostsCount != 1 {
		t.Fatalf("expected posts_count=1, got %d", thread.PostsCount)
	}

	projectionRows, err := s.queryJSON(`SELECT projection_name, event_id FROM forum_projection_events WHERE event_id='evt-open-proj-1' ORDER BY projection_name;`)
	if err != nil {
		t.Fatalf("list projection events: %v", err)
	}
	if len(projectionRows) != 2 {
		t.Fatalf("expected two projection markers (views+stats), got %d", len(projectionRows))
	}
}

func TestForumProjectionReplayRebuildsMissingThreadView(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := s.ForumOpenThread(model.ForumOpenThreadCommand{
		Envelope: model.ForumEnvelope{
			EventID:      "evt-open-proj-replay-1",
			EventType:    "forum.thread.opened",
			EventVersion: 1,
			OccurredAt:   time.Now().UTC(),
			ThreadID:     "thread-proj-replay-1",
			RunID:        "run-proj-replay-1",
			Ticket:       "METAWSM-010",
			AgentName:    "agent-a",
			ActorType:    model.ForumActorAgent,
			ActorName:    "agent-a",
		},
		Title:    "Projection replay thread",
		Body:     "Initial post for replay recovery",
		Priority: model.ForumPriorityNormal,
	}); err != nil {
		t.Fatalf("open forum thread: %v", err)
	}

	event, err := s.GetForumEvent("evt-open-proj-replay-1")
	if err != nil {
		t.Fatalf("get forum event: %v", err)
	}
	if event == nil {
		t.Fatalf("expected stored forum event")
	}

	if err := s.execSQL(`DELETE FROM forum_thread_views WHERE thread_id='thread-proj-replay-1';`); err != nil {
		t.Fatalf("delete projection row: %v", err)
	}
	thread, err := s.GetForumThread("thread-proj-replay-1")
	if err != nil {
		t.Fatalf("get projected thread after delete: %v", err)
	}
	if thread != nil {
		t.Fatalf("expected missing projection row after deletion")
	}

	if err := s.ApplyForumEventProjections(*event); err != nil {
		t.Fatalf("replay projection event: %v", err)
	}
	rebuilt, err := s.GetForumThread("thread-proj-replay-1")
	if err != nil {
		t.Fatalf("get rebuilt thread: %v", err)
	}
	if rebuilt == nil {
		t.Fatalf("expected projection row rebuilt from replay")
	}
	if rebuilt.PostsCount != 1 {
		t.Fatalf("expected rebuilt posts_count=1, got %d", rebuilt.PostsCount)
	}
}

func TestForumOutboxLifecycle(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	s := NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if err := s.EnqueueForumOutbox(model.ForumOutboxMessage{
		MessageID:   "msg-1",
		Topic:       "forum.commands.open_thread",
		MessageKey:  "thread-1",
		PayloadJSON: `{"ok":true}`,
	}); err != nil {
		t.Fatalf("enqueue outbox: %v", err)
	}

	claimed, err := s.ClaimForumOutboxPending(10)
	if err != nil {
		t.Fatalf("claim outbox: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed message, got %d", len(claimed))
	}
	if claimed[0].Status != model.ForumOutboxStatusProcessing {
		t.Fatalf("expected processing status, got %s", claimed[0].Status)
	}

	if err := s.MarkForumOutboxSent("msg-1"); err != nil {
		t.Fatalf("mark outbox sent: %v", err)
	}

	sent, err := s.ListForumOutboxByStatus(model.ForumOutboxStatusSent, 10)
	if err != nil {
		t.Fatalf("list sent outbox: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected one sent outbox row, got %d", len(sent))
	}
	if sent[0].SentAt == nil {
		t.Fatalf("expected sent_at to be set")
	}

	failedCount, err := s.CountForumOutboxByStatus(model.ForumOutboxStatusFailed)
	if err != nil {
		t.Fatalf("count failed outbox: %v", err)
	}
	if failedCount != 0 {
		t.Fatalf("expected no failed rows, got %d", failedCount)
	}

	pendingCount, err := s.CountForumOutboxByStatus(model.ForumOutboxStatusPending)
	if err != nil {
		t.Fatalf("count pending outbox: %v", err)
	}
	if pendingCount != 0 {
		t.Fatalf("expected no pending rows, got %d", pendingCount)
	}

	oldestPending, err := s.OldestForumOutboxCreatedAt(model.ForumOutboxStatusPending)
	if err != nil {
		t.Fatalf("oldest pending outbox: %v", err)
	}
	if oldestPending != nil {
		t.Fatalf("expected nil oldest pending timestamp after send")
	}
}
