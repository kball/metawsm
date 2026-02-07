package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicyIsValid(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected default policy to validate: %v", err)
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
	if cfg.Version != 1 {
		t.Fatalf("expected default policy version 1, got %d", cfg.Version)
	}
}
