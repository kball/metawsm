package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPolicyIsValid(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected default policy to validate: %v", err)
	}
	if cfg.Workspace.BaseBranch != "main" {
		t.Fatalf("expected default base branch main, got %q", cfg.Workspace.BaseBranch)
	}
	if cfg.Operator.LLM.Command != "codex" {
		t.Fatalf("expected default operator llm command codex, got %q", cfg.Operator.LLM.Command)
	}
	if cfg.Operator.LLM.Mode != "assist" {
		t.Fatalf("expected default operator llm mode assist, got %q", cfg.Operator.LLM.Mode)
	}
	if cfg.GitPR.CredentialMode != "local_user_auth" {
		t.Fatalf("expected default git_pr credential mode local_user_auth, got %q", cfg.GitPR.CredentialMode)
	}
	if cfg.GitPR.Mode != "assist" {
		t.Fatalf("expected default git_pr mode assist, got %q", cfg.GitPR.Mode)
	}
	if !cfg.GitPR.RequireAll {
		t.Fatalf("expected default git_pr require_all=true")
	}
	if len(cfg.GitPR.RequiredChecks) == 0 {
		t.Fatalf("expected default git_pr required checks")
	}
}

func TestLoadPolicyFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "policy.json")
	if err := SaveDefault(path); err != nil {
		t.Fatalf("save default policy: %v", err)
	}

	cfg, loadedPath, err := Load(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if loadedPath != path {
		t.Fatalf("expected loaded path %q, got %q", path, loadedPath)
	}
	if cfg.Workspace.DefaultStrategy == "" {
		t.Fatalf("expected non-empty default strategy")
	}
}

func TestLoadPolicyMissingFileUsesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "missing-policy.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected missing test policy file")
	}

	cfg, loadedPath, err := Load(path)
	if err != nil {
		t.Fatalf("load policy with missing file: %v", err)
	}
	if loadedPath != path {
		t.Fatalf("expected loaded path %q, got %q", path, loadedPath)
	}
	if cfg.Version != 2 {
		t.Fatalf("expected default policy version 2, got %d", cfg.Version)
	}
}

func TestValidateRequiresAgentProfileReference(t *testing.T) {
	cfg := Default()
	cfg.Agents = []Agent{{Name: "agent"}}

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected validation failure for missing agent profile")
	}
	if !strings.Contains(err.Error(), "agent.profile") {
		t.Fatalf("expected agent.profile validation error, got %v", err)
	}
}

func TestResolveAgentsCompilesCodexCommandWithSkills(t *testing.T) {
	root := t.TempDir()
	policyPath := filepath.Join(root, ".metawsm", "policy.json")
	if err := os.MkdirAll(filepath.Join(root, ".metawsm", "skills", "docmgr"), 0o755); err != nil {
		t.Fatalf("mkdir docmgr skill path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".metawsm", "skills", "diary"), 0o755); err != nil {
		t.Fatalf("mkdir diary skill path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metawsm", "skills", "docmgr", "SKILL.md"), []byte("# docmgr\n"), 0o644); err != nil {
		t.Fatalf("write docmgr SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metawsm", "skills", "diary", "SKILL.md"), []byte("# diary\n"), 0o644); err != nil {
		t.Fatalf("write diary SKILL.md: %v", err)
	}

	cfg := Default()
	cfg.AgentProfiles = []AgentProfile{
		{
			Name:       "codex-default",
			Runner:     "codex",
			BasePrompt: "Implement this ticket.",
			Skills:     []string{"docmgr", "diary"},
			RunnerOptions: RunnerOptions{
				FullAuto: true,
			},
		},
	}
	cfg.Agents = []Agent{
		{Name: "agent", Profile: "codex-default"},
	}

	agents, err := ResolveAgents(cfg, nil, policyPath)
	if err != nil {
		t.Fatalf("resolve agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	spec := agents[0]
	if spec.Name != "agent" {
		t.Fatalf("unexpected agent name %q", spec.Name)
	}
	if spec.Profile != "codex-default" {
		t.Fatalf("unexpected profile %q", spec.Profile)
	}
	if spec.Runner != "codex" {
		t.Fatalf("unexpected runner %q", spec.Runner)
	}
	if !strings.Contains(spec.Command, "codex exec --full-auto") {
		t.Fatalf("expected codex full-auto command, got %q", spec.Command)
	}
	if !strings.Contains(spec.Command, ".metawsm/skills/docmgr/SKILL.md") {
		t.Fatalf("expected docmgr skill path in command, got %q", spec.Command)
	}
	if !strings.Contains(spec.Command, ".metawsm/skills/diary/SKILL.md") {
		t.Fatalf("expected diary skill path in command, got %q", spec.Command)
	}
}

func TestResolveAgentsFailsWhenSkillMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	policyPath := filepath.Join(root, ".metawsm", "policy.json")
	cfg := Default()
	cfg.AgentProfiles = []AgentProfile{
		{
			Name:       "codex-default",
			Runner:     "codex",
			BasePrompt: "Implement this ticket.",
			Skills:     []string{"docmgr"},
			RunnerOptions: RunnerOptions{
				FullAuto: true,
			},
		},
	}
	cfg.Agents = []Agent{
		{Name: "agent", Profile: "codex-default"},
	}

	_, err := ResolveAgents(cfg, nil, policyPath)
	if err == nil {
		t.Fatalf("expected missing skill error")
	}
	if !strings.Contains(err.Error(), "skill \"docmgr\" not found") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestValidateDocAPIEndpoints(t *testing.T) {
	cfg := Default()
	cfg.Docs.API.WorkspaceEndpoints = []DocAPIEndpoint{
		{
			Name:      "ws-metawsm",
			BaseURL:   "http://127.0.0.1:8787",
			WebURL:    "http://127.0.0.1:8787",
			Repo:      "metawsm",
			Workspace: "ws-001",
		},
	}
	cfg.Docs.API.RepoEndpoints = []DocAPIEndpoint{
		{
			Name:    "repo-metawsm",
			BaseURL: "http://127.0.0.1:8790",
			WebURL:  "http://127.0.0.1:8790",
			Repo:    "metawsm",
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected docs API endpoint config to validate: %v", err)
	}
}

func TestValidateRejectsInvalidDocAPIEndpoint(t *testing.T) {
	cfg := Default()
	cfg.Docs.API.WorkspaceEndpoints = []DocAPIEndpoint{
		{
			Name:      "bad-endpoint",
			BaseURL:   "://bad-url",
			Repo:      "metawsm",
			Workspace: "ws-001",
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected validation error for invalid docs API endpoint")
	}
	if !strings.Contains(err.Error(), "invalid base_url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsInvalidOperatorLLMMode(t *testing.T) {
	cfg := Default()
	cfg.Operator.LLM.Mode = "maybe"

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected operator llm mode validation error")
	}
	if !strings.Contains(err.Error(), "operator.llm.mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsMissingOperatorCommand(t *testing.T) {
	cfg := Default()
	cfg.Operator.LLM.Command = ""

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected operator llm command validation error")
	}
	if !strings.Contains(err.Error(), "operator.llm.command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsInvalidOperatorBudgets(t *testing.T) {
	cfg := Default()
	cfg.Operator.RestartBudget = 0

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected restart budget validation error")
	}
	if !strings.Contains(err.Error(), "operator.restart_budget") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsInvalidGitPRMode(t *testing.T) {
	cfg := Default()
	cfg.GitPR.Mode = "maybe"

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected git_pr mode validation error")
	}
	if !strings.Contains(err.Error(), "git_pr.mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsInvalidGitPRCredentialMode(t *testing.T) {
	cfg := Default()
	cfg.GitPR.CredentialMode = "token_broker"

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected git_pr credential mode validation error")
	}
	if !strings.Contains(err.Error(), "git_pr.credential_mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsEmptyGitPRBranchTemplate(t *testing.T) {
	cfg := Default()
	cfg.GitPR.BranchTemplate = " "

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected git_pr branch template validation error")
	}
	if !strings.Contains(err.Error(), "git_pr.branch_template") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderGitBranchUsesDefaultTemplate(t *testing.T) {
	branch := RenderGitBranch("", "METAWSM-009", "metawsm/api", "run-20260208-120000")
	if branch != "metawsm-009/metawsm-api/run-20260208-120000" {
		t.Fatalf("unexpected default rendered branch: %q", branch)
	}
}

func TestRenderGitBranchHonorsCustomTemplate(t *testing.T) {
	branch := RenderGitBranch("{repo}/changes/{ticket}", "METAWSM-009", "metawsm", "run-20260208-120000")
	if branch != "metawsm/changes/metawsm-009" {
		t.Fatalf("unexpected custom rendered branch: %q", branch)
	}
}

func TestRenderGitBranchFallsBackWhenTemplateHasNoSegments(t *testing.T) {
	branch := RenderGitBranch("///", "METAWSM-009", "metawsm", "run-20260208-120000")
	if branch != "metawsm-009/metawsm/run-20260208-120000" {
		t.Fatalf("unexpected fallback rendered branch: %q", branch)
	}
}
