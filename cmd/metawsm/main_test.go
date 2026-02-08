package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"metawsm/internal/docfederation"
	"metawsm/internal/model"
	"metawsm/internal/policy"
)

func TestCollectBootstrapBriefNonInteractiveRequiresAllFields(t *testing.T) {
	_, err := collectBootstrapBrief(strings.NewReader(""), &bytes.Buffer{}, false, "METAWSM-002", model.RunBrief{
		Ticket: "METAWSM-002",
		Goal:   "Implement bootstrap",
	})
	if err == nil {
		t.Fatalf("expected error for missing non-interactive fields")
	}
	if !strings.Contains(err.Error(), "missing required bootstrap intake field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectBootstrapBriefInteractivePrompts(t *testing.T) {
	input := strings.NewReader("Goal answer\nScope answer\nDone answer\nConstraints answer\ndefault\n")
	var output bytes.Buffer
	brief, err := collectBootstrapBrief(input, &output, true, "METAWSM-002", model.RunBrief{Ticket: "METAWSM-002"})
	if err != nil {
		t.Fatalf("collect bootstrap brief: %v", err)
	}
	if brief.Goal != "Goal answer" {
		t.Fatalf("expected goal answer, got %q", brief.Goal)
	}
	if brief.Scope != "Scope answer" {
		t.Fatalf("expected scope answer, got %q", brief.Scope)
	}
	if brief.DoneCriteria != "Done answer" {
		t.Fatalf("expected done criteria answer, got %q", brief.DoneCriteria)
	}
	if brief.Constraints != "Constraints answer" {
		t.Fatalf("expected constraints answer, got %q", brief.Constraints)
	}
	if brief.MergeIntent != "default" {
		t.Fatalf("expected merge intent default, got %q", brief.MergeIntent)
	}
	if len(brief.QA) != 5 {
		t.Fatalf("expected 5 QA entries, got %d", len(brief.QA))
	}
}

func TestCollectBootstrapBriefNonInteractiveWithSeed(t *testing.T) {
	brief, err := collectBootstrapBrief(strings.NewReader(""), &bytes.Buffer{}, false, "METAWSM-002", model.RunBrief{
		Ticket:       "METAWSM-002",
		Goal:         "Goal",
		Scope:        "Scope",
		DoneCriteria: "Done",
		Constraints:  "Constraints",
		MergeIntent:  "default",
	})
	if err != nil {
		t.Fatalf("collect bootstrap brief with seed: %v", err)
	}
	if len(brief.QA) != 5 {
		t.Fatalf("expected 5 QA entries, got %d", len(brief.QA))
	}
}

func TestExtractJSONArray(t *testing.T) {
	payload := []map[string]any{
		{"ticket": "METAWSM-001"},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	out := []byte(`{"level":"debug"}\n` + string(b) + "\n")
	extracted, ok := extractJSONArray(out)
	if !ok {
		t.Fatalf("expected to extract json array")
	}
	var parsed []map[string]any
	if err := json.Unmarshal(extracted, &parsed); err != nil {
		t.Fatalf("unmarshal extracted json: %v", err)
	}
	if len(parsed) != 1 || parsed[0]["ticket"] != "METAWSM-001" {
		t.Fatalf("unexpected parsed payload: %#v", parsed)
	}
}

func TestRequireRunSelector(t *testing.T) {
	_, _, err := requireRunSelector("", "")
	if err == nil {
		t.Fatalf("expected selector error")
	}

	runID, ticket, err := requireRunSelector(" run-1 ", "")
	if err != nil {
		t.Fatalf("selector with run id: %v", err)
	}
	if runID != "run-1" || ticket != "" {
		t.Fatalf("unexpected selector result run=%q ticket=%q", runID, ticket)
	}

	runID, ticket, err = requireRunSelector("", " METAWSM-003 ")
	if err != nil {
		t.Fatalf("selector with ticket: %v", err)
	}
	if runID != "" || ticket != "METAWSM-003" {
		t.Fatalf("unexpected selector result run=%q ticket=%q", runID, ticket)
	}
}

func TestFederationEndpointsFromPolicyWorkspaceFirst(t *testing.T) {
	cfg := policy.Default()
	cfg.Docs.API.WorkspaceEndpoints = []policy.DocAPIEndpoint{
		{
			Name:      "ws-metawsm",
			BaseURL:   "http://127.0.0.1:8787",
			WebURL:    "http://127.0.0.1:8787",
			Repo:      "metawsm",
			Workspace: "ws-001",
		},
	}
	cfg.Docs.API.RepoEndpoints = []policy.DocAPIEndpoint{
		{
			Name:    "repo-metawsm",
			BaseURL: "http://127.0.0.1:8790",
			WebURL:  "http://127.0.0.1:8790",
			Repo:    "metawsm",
		},
	}
	endpoints := federationEndpointsFromPolicy(cfg)
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].Kind != docfederation.EndpointKindWorkspace {
		t.Fatalf("expected workspace endpoint first, got %s", endpoints[0].Kind)
	}
	if endpoints[1].Kind != docfederation.EndpointKindRepo {
		t.Fatalf("expected repo endpoint second, got %s", endpoints[1].Kind)
	}
}

func TestSelectFederationEndpointsByName(t *testing.T) {
	endpoints := []docfederation.Endpoint{
		{Name: "repo-z", Kind: docfederation.EndpointKindRepo},
		{Name: "workspace-a", Kind: docfederation.EndpointKindWorkspace},
		{Name: "repo-b", Kind: docfederation.EndpointKindRepo},
	}
	selected := selectFederationEndpoints(endpoints, []string{"repo-b", "workspace-a"})
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected endpoints, got %d", len(selected))
	}
	if selected[0].Name != "repo-b" {
		t.Fatalf("expected sorted selection starting with repo-b, got %q", selected[0].Name)
	}
	if selected[1].Name != "workspace-a" {
		t.Fatalf("expected sorted selection ending with workspace-a, got %q", selected[1].Name)
	}
}

func TestParseWatchSnapshot(t *testing.T) {
	status := `Run: run-123
Status: awaiting_guidance
Mode: bootstrap
Tickets: METAWSM-006
Guidance:
  - id=7 agent@ws question=Need operator decision
Agents:
  - agent@ws session=s1 status=stalled health=stalled last_activity=2026-02-08T00:00:00Z activity_age=1h last_progress=2026-02-08T00:00:00Z progress_age=1h
`
	snapshot := parseWatchSnapshot(status)
	if snapshot.RunID != "run-123" {
		t.Fatalf("expected run id run-123, got %q", snapshot.RunID)
	}
	if snapshot.RunStatus != "awaiting_guidance" {
		t.Fatalf("expected awaiting_guidance, got %q", snapshot.RunStatus)
	}
	if snapshot.Tickets != "METAWSM-006" {
		t.Fatalf("expected tickets METAWSM-006, got %q", snapshot.Tickets)
	}
	if !snapshot.HasGuidance {
		t.Fatalf("expected guidance=true")
	}
	if !snapshot.HasUnhealthyAgents {
		t.Fatalf("expected unhealthy agents=true")
	}
	if len(snapshot.GuidanceItems) != 1 {
		t.Fatalf("expected one guidance item, got %d", len(snapshot.GuidanceItems))
	}
	if len(snapshot.UnhealthyAgents) != 1 {
		t.Fatalf("expected one unhealthy agent, got %d", len(snapshot.UnhealthyAgents))
	}
	if !strings.Contains(snapshot.UnhealthyAgents[0].Reason, "no recent activity/progress") {
		t.Fatalf("expected stalled reason, got %q", snapshot.UnhealthyAgents[0].Reason)
	}
	if snapshot.UnhealthyAgents[0].Session != "s1" {
		t.Fatalf("expected unhealthy session s1, got %q", snapshot.UnhealthyAgents[0].Session)
	}
}

func TestClassifyWatchEventPrioritizesGuidance(t *testing.T) {
	event, message, terminal := classifyWatchEvent(watchSnapshot{
		RunStatus:          string(model.RunStatusRunning),
		HasGuidance:        true,
		HasUnhealthyAgents: true,
	})
	if event != "guidance_needed" {
		t.Fatalf("expected guidance_needed event, got %q", event)
	}
	if message == "" {
		t.Fatalf("expected non-empty alert message")
	}
	if !terminal {
		t.Fatalf("expected guidance alert to be terminal")
	}
}

func TestClassifyWatchEventDone(t *testing.T) {
	event, _, terminal := classifyWatchEvent(watchSnapshot{
		RunStatus: string(model.RunStatusComplete),
	})
	if event != "run_done" {
		t.Fatalf("expected run_done event, got %q", event)
	}
	if !terminal {
		t.Fatalf("expected run_done to be terminal")
	}
}

func TestClassifyWatchEventUnhealthyNonTerminal(t *testing.T) {
	event, _, terminal := classifyWatchEvent(watchSnapshot{
		RunStatus:          string(model.RunStatusRunning),
		HasUnhealthyAgents: true,
	})
	if event != "agent_unhealthy" {
		t.Fatalf("expected agent_unhealthy event, got %q", event)
	}
	if terminal {
		t.Fatalf("expected agent_unhealthy to be non-terminal")
	}
}

func TestClassifyWatchEventSuppressesUnhealthyForPausedRuns(t *testing.T) {
	event, _, terminal := classifyWatchEvent(watchSnapshot{
		RunStatus:          string(model.RunStatusPaused),
		HasUnhealthyAgents: true,
	})
	if event != "" {
		t.Fatalf("expected no unhealthy alert for paused run, got %q", event)
	}
	if terminal {
		t.Fatalf("expected no terminal alert for paused unhealthy state")
	}
}

func TestBuildWatchDirectionHintsIncludesLikelyCause(t *testing.T) {
	snapshot := watchSnapshot{
		RunID: "run-123",
		UnhealthyAgents: []watchAgentIssue{
			{
				Agent:  "agent@ws",
				Reason: "agent session is not running",
			},
		},
	}
	hints := buildWatchDirectionHints(snapshot, "agent_unhealthy")
	joined := strings.Join(hints, "\n")
	if !strings.Contains(joined, "Likely cause: agent session is not running (agent@ws)") {
		t.Fatalf("expected likely-cause hint, got:\n%s", joined)
	}
	if !strings.Contains(joined, "metawsm restart --run-id run-123") {
		t.Fatalf("expected restart direction hint, got:\n%s", joined)
	}
}

func TestResolveWatchMode(t *testing.T) {
	mode, err := resolveWatchMode("", "", false)
	if err != nil {
		t.Fatalf("resolve watch mode default all: %v", err)
	}
	if mode != watchModeAllActiveRuns {
		t.Fatalf("expected all-active mode, got %v", mode)
	}

	mode, err = resolveWatchMode("", "", true)
	if err != nil {
		t.Fatalf("resolve watch mode explicit all: %v", err)
	}
	if mode != watchModeAllActiveRuns {
		t.Fatalf("expected all-active mode, got %v", mode)
	}

	mode, err = resolveWatchMode("run-1", "", false)
	if err != nil {
		t.Fatalf("resolve watch mode single run: %v", err)
	}
	if mode != watchModeSingleRun {
		t.Fatalf("expected single mode, got %v", mode)
	}

	mode, err = resolveWatchMode("", "METAWSM-006", false)
	if err != nil {
		t.Fatalf("resolve watch mode ticket: %v", err)
	}
	if mode != watchModeSingleRun {
		t.Fatalf("expected single mode, got %v", mode)
	}

	_, err = resolveWatchMode("run-1", "", true)
	if err == nil {
		t.Fatalf("expected selector conflict error when --all is combined with --run-id")
	}
}

func TestIsTerminalRunStatus(t *testing.T) {
	if !isTerminalRunStatus(string(model.RunStatusComplete)) {
		t.Fatalf("expected completed to be terminal")
	}
	if !isTerminalRunStatus(string(model.RunStatusClosed)) {
		t.Fatalf("expected closed to be terminal")
	}
	if !isTerminalRunStatus(string(model.RunStatusFailed)) {
		t.Fatalf("expected failed to be terminal")
	}
	if !isTerminalRunStatus(string(model.RunStatusStopped)) {
		t.Fatalf("expected stopped to be terminal")
	}
	if isTerminalRunStatus(string(model.RunStatusRunning)) {
		t.Fatalf("expected running to be non-terminal")
	}
}

func TestResolveOperatorLLMMode(t *testing.T) {
	mode, err := resolveOperatorLLMMode("", "assist")
	if err != nil {
		t.Fatalf("resolve from policy: %v", err)
	}
	if mode != "assist" {
		t.Fatalf("expected assist mode, got %q", mode)
	}

	mode, err = resolveOperatorLLMMode("AUTO", "assist")
	if err != nil {
		t.Fatalf("resolve from flag: %v", err)
	}
	if mode != "auto" {
		t.Fatalf("expected auto mode, got %q", mode)
	}

	if _, err := resolveOperatorLLMMode("invalid", "assist"); err == nil {
		t.Fatalf("expected invalid mode error")
	}
}

func TestAuthCommandRequiresCheckSubcommand(t *testing.T) {
	err := authCommand([]string{})
	if err == nil {
		t.Fatalf("expected auth usage error")
	}
	if !strings.Contains(err.Error(), "usage: metawsm auth check") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommitCommandRequiresRunSelector(t *testing.T) {
	err := commitCommand([]string{})
	if err == nil {
		t.Fatalf("expected commit selector error")
	}
	if !strings.Contains(err.Error(), "one of --run-id or --ticket is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPRCommandRequiresRunSelector(t *testing.T) {
	err := prCommand([]string{})
	if err == nil {
		t.Fatalf("expected pr selector error")
	}
	if !strings.Contains(err.Error(), "one of --run-id or --ticket is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceRepoPathPrefersNestedRepoDir(t *testing.T) {
	workspacePath := t.TempDir()
	repoPath := filepath.Join(workspacePath, "metawsm")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	resolved, err := resolveWorkspaceRepoPath(workspacePath, "metawsm", 2)
	if err != nil {
		t.Fatalf("resolve workspace repo path: %v", err)
	}
	if resolved != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, resolved)
	}
}

func TestResolveWorkspaceRepoPathFallsBackForSingleRepoWorkspaceRoot(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	resolved, err := resolveWorkspaceRepoPath(workspacePath, "metawsm", 1)
	if err != nil {
		t.Fatalf("resolve workspace repo path fallback: %v", err)
	}
	if resolved != workspacePath {
		t.Fatalf("expected fallback workspace path %q, got %q", workspacePath, resolved)
	}
}

func TestResolveWorkspaceRepoPathErrorsWhenRepoMissing(t *testing.T) {
	workspacePath := t.TempDir()
	_, err := resolveWorkspaceRepoPath(workspacePath, "metawsm", 2)
	if err == nil {
		t.Fatalf("expected missing repo path error")
	}
	if !strings.Contains(err.Error(), "repo path not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyStaleRunCandidate(t *testing.T) {
	now := time.Now()
	run := model.RunRecord{
		RunID:     "run-1",
		Status:    model.RunStatusRunning,
		UpdatedAt: now.Add(-2 * time.Hour),
	}
	snapshot := watchSnapshot{
		RunID:              "run-1",
		RunStatus:          string(model.RunStatusRunning),
		HasUnhealthyAgents: true,
	}

	stale, reason := classifyStaleRunCandidate(snapshot, run, now, time.Hour)
	if !stale {
		t.Fatalf("expected stale candidate")
	}
	if !strings.Contains(reason, "run appears stale") {
		t.Fatalf("expected stale reason, got %q", reason)
	}
}

func TestVerifyStaleRuntimeEvidenceRejectsActiveSession(t *testing.T) {
	now := time.Now()
	snapshot := watchSnapshot{
		UnhealthyAgents: []watchAgentIssue{
			{Agent: "agent@ws", Session: "session-a"},
		},
	}
	probe := func(ctx context.Context, session string) (operatorSessionEvidence, error) {
		lastActivity := now.Add(-30 * time.Second)
		return operatorSessionEvidence{
			Session:      session,
			HasSession:   true,
			LastActivity: &lastActivity,
		}, nil
	}

	verified, reason, err := verifyStaleRuntimeEvidence(context.Background(), snapshot, now, 2*time.Minute, probe)
	if err != nil {
		t.Fatalf("verify stale runtime evidence: %v", err)
	}
	if verified {
		t.Fatalf("expected stale verification rejection for active session")
	}
	if !strings.Contains(reason, "recent activity") {
		t.Fatalf("expected recent activity reason, got %q", reason)
	}
}

func TestVerifyStaleRuntimeEvidenceAcceptsExitedSessions(t *testing.T) {
	now := time.Now()
	snapshot := watchSnapshot{
		UnhealthyAgents: []watchAgentIssue{
			{Agent: "agent@ws", Session: "session-a"},
		},
	}
	probe := func(ctx context.Context, session string) (operatorSessionEvidence, error) {
		code := 1
		return operatorSessionEvidence{
			Session:    session,
			HasSession: true,
			ExitCode:   &code,
		}, nil
	}

	verified, reason, err := verifyStaleRuntimeEvidence(context.Background(), snapshot, now, 2*time.Minute, probe)
	if err != nil {
		t.Fatalf("verify stale runtime evidence: %v", err)
	}
	if !verified {
		t.Fatalf("expected stale verification success")
	}
	if !strings.Contains(reason, "no active tmux sessions") {
		t.Fatalf("expected no-active-session reason, got %q", reason)
	}
}
