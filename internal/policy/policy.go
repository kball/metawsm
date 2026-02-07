package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"metawsm/internal/model"
)

const DefaultPolicyPath = ".metawsm/policy.json"

type Config struct {
	Version   int `json:"version"`
	Workspace struct {
		DefaultStrategy string `json:"default_strategy"`
		BranchPrefix    string `json:"branch_prefix"`
	} `json:"workspace"`
	Tmux struct {
		SessionPattern string `json:"session_pattern"`
	} `json:"tmux"`
	Execution struct {
		StepRetries int `json:"step_retries"`
	} `json:"execution"`
	Health struct {
		IdleSeconds            int `json:"idle_seconds"`
		ActivityStalledSeconds int `json:"activity_stalled_seconds"`
		ProgressStalledSeconds int `json:"progress_stalled_seconds"`
	} `json:"health"`
	Close struct {
		RequireCleanGit bool `json:"require_clean_git"`
	} `json:"close"`
	Agents []Agent `json:"agents"`
}

type Agent struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

func Default() Config {
	cfg := Config{
		Version: 1,
	}
	cfg.Workspace.DefaultStrategy = string(model.WorkspaceStrategyCreate)
	cfg.Workspace.BranchPrefix = "task"
	cfg.Tmux.SessionPattern = "{agent}-{workspace}"
	cfg.Execution.StepRetries = 1
	cfg.Health.IdleSeconds = 300
	cfg.Health.ActivityStalledSeconds = 900
	cfg.Health.ProgressStalledSeconds = 1200
	cfg.Close.RequireCleanGit = true
	cfg.Agents = []Agent{
		{
			Name:    "agent",
			Command: "bash",
		},
	}
	return cfg
}

func Load(path string) (Config, string, error) {
	cfg := Default()
	finalPath := path
	if strings.TrimSpace(finalPath) == "" {
		finalPath = DefaultPolicyPath
	}
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return cfg, finalPath, nil
	}

	b, err := os.ReadFile(finalPath)
	if err != nil {
		return cfg, finalPath, fmt.Errorf("read policy %s: %w", finalPath, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, finalPath, fmt.Errorf("parse policy %s: %w", finalPath, err)
	}
	if err := Validate(cfg); err != nil {
		return cfg, finalPath, fmt.Errorf("validate policy %s: %w", finalPath, err)
	}
	return cfg, finalPath, nil
}

func SaveDefault(path string) error {
	cfg := Default()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func Validate(cfg Config) error {
	if cfg.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	strategy := model.WorkspaceStrategy(cfg.Workspace.DefaultStrategy)
	switch strategy {
	case model.WorkspaceStrategyCreate, model.WorkspaceStrategyFork, model.WorkspaceStrategyReuse:
	default:
		return fmt.Errorf("workspace.default_strategy must be create|fork|reuse")
	}
	if strings.TrimSpace(cfg.Tmux.SessionPattern) == "" {
		return fmt.Errorf("tmux.session_pattern cannot be empty")
	}
	if cfg.Health.IdleSeconds <= 0 || cfg.Health.ActivityStalledSeconds <= 0 || cfg.Health.ProgressStalledSeconds <= 0 {
		return fmt.Errorf("health thresholds must be > 0")
	}
	if cfg.Health.ActivityStalledSeconds < cfg.Health.IdleSeconds {
		return fmt.Errorf("activity_stalled_seconds must be >= idle_seconds")
	}
	if cfg.Execution.StepRetries < 0 {
		return fmt.Errorf("execution.step_retries must be >= 0")
	}
	if len(cfg.Agents) == 0 {
		return fmt.Errorf("agents must contain at least one entry")
	}
	for _, agent := range cfg.Agents {
		if strings.TrimSpace(agent.Name) == "" {
			return fmt.Errorf("agent.name cannot be empty")
		}
		if strings.TrimSpace(agent.Command) == "" {
			return fmt.Errorf("agent.command cannot be empty")
		}
	}
	return nil
}

func ResolveAgents(cfg Config, requested []string) ([]model.AgentSpec, error) {
	agentMap := map[string]string{}
	for _, agent := range cfg.Agents {
		agentMap[agent.Name] = agent.Command
	}

	if len(requested) == 0 {
		out := make([]model.AgentSpec, 0, len(cfg.Agents))
		for _, agent := range cfg.Agents {
			out = append(out, model.AgentSpec{Name: agent.Name, Command: agent.Command})
		}
		return out, nil
	}

	out := make([]model.AgentSpec, 0, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		command, ok := agentMap[name]
		if !ok {
			return nil, fmt.Errorf("requested agent %q not found in policy", name)
		}
		out = append(out, model.AgentSpec{Name: name, Command: command})
	}
	return out, nil
}

func RenderSessionName(pattern string, agentName string, workspaceName string) string {
	s := strings.ReplaceAll(pattern, "{agent}", sanitizeToken(agentName))
	s = strings.ReplaceAll(s, "{workspace}", sanitizeToken(workspaceName))
	if strings.TrimSpace(s) == "" {
		return fmt.Sprintf("%s-%s", sanitizeToken(agentName), sanitizeToken(workspaceName))
	}
	return s
}

func sanitizeToken(token string) string {
	token = strings.TrimSpace(strings.ToLower(token))
	token = strings.ReplaceAll(token, " ", "-")
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", ",", "-", ".", "-", "@", "-", "#", "-", "[", "-", "]", "-", "{", "-", "}", "-", "(", "-", ")", "-")
	token = replacer.Replace(token)
	for strings.Contains(token, "--") {
		token = strings.ReplaceAll(token, "--", "-")
	}
	token = strings.Trim(token, "-")
	if token == "" {
		token = "x"
	}
	return token
}
