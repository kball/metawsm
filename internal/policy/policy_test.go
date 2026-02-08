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
