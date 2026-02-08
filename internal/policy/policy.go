package policy

import (
	"encoding/json"
	"fmt"
	"net/url"
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
		BaseBranch      string `json:"base_branch"`
	} `json:"workspace"`
	Docs struct {
		AuthorityMode       string `json:"authority_mode"`
		SeedMode            string `json:"seed_mode"`
		StaleWarningSeconds int    `json:"stale_warning_seconds"`
		API                 struct {
			WorkspaceEndpoints []DocAPIEndpoint `json:"workspace_endpoints"`
			RepoEndpoints      []DocAPIEndpoint `json:"repo_endpoints"`
			RequestTimeoutSec  int              `json:"request_timeout_seconds"`
		} `json:"api"`
	} `json:"docs"`
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
	Operator struct {
		UnhealthyConfirmations int `json:"unhealthy_confirmations"`
		RestartBudget          int `json:"restart_budget"`
		RestartCooldownSeconds int `json:"restart_cooldown_seconds"`
		StaleRunAgeSeconds     int `json:"stale_run_age_seconds"`
		LLM                    struct {
			Mode           string `json:"mode"`
			Command        string `json:"command"`
			Model          string `json:"model"`
			TimeoutSeconds int    `json:"timeout_seconds"`
			MaxTokens      int    `json:"max_tokens"`
		} `json:"llm"`
	} `json:"operator"`
	GitPR struct {
		Mode             string   `json:"mode"`
		CredentialMode   string   `json:"credential_mode"`
		BranchTemplate   string   `json:"branch_template"`
		RequireAll       bool     `json:"require_all"`
		RequiredChecks   []string `json:"required_checks"`
		AllowedRepos     []string `json:"allowed_repos"`
		DefaultLabels    []string `json:"default_labels"`
		DefaultReviewers []string `json:"default_reviewers"`
	} `json:"git_pr"`
	AgentProfiles []AgentProfile `json:"agent_profiles"`
	Agents        []Agent        `json:"agents"`
}

type AgentProfile struct {
	Name          string        `json:"name"`
	Runner        string        `json:"runner"`
	BasePrompt    string        `json:"base_prompt"`
	Skills        []string      `json:"skills"`
	RunnerOptions RunnerOptions `json:"runner_options"`
}

type RunnerOptions struct {
	FullAuto bool   `json:"full_auto"`
	Command  string `json:"command"`
}

type DocAPIEndpoint struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	WebURL    string `json:"web_url,omitempty"`
	Repo      string `json:"repo"`
	Workspace string `json:"workspace,omitempty"`
}

type Agent struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
}

func Default() Config {
	cfg := Config{
		Version: 2,
	}
	cfg.Workspace.DefaultStrategy = string(model.WorkspaceStrategyCreate)
	cfg.Workspace.BranchPrefix = "task"
	cfg.Workspace.BaseBranch = "main"
	cfg.Docs.AuthorityMode = string(model.DocAuthorityModeWorkspaceActive)
	cfg.Docs.SeedMode = string(model.DocSeedModeCopyFromRepoOnStart)
	cfg.Docs.StaleWarningSeconds = 900
	cfg.Docs.API.RequestTimeoutSec = 3
	cfg.Tmux.SessionPattern = "{agent}-{workspace}"
	cfg.Execution.StepRetries = 1
	cfg.Health.IdleSeconds = 300
	cfg.Health.ActivityStalledSeconds = 900
	cfg.Health.ProgressStalledSeconds = 1200
	cfg.Close.RequireCleanGit = true
	cfg.Operator.UnhealthyConfirmations = 2
	cfg.Operator.RestartBudget = 3
	cfg.Operator.RestartCooldownSeconds = 60
	cfg.Operator.StaleRunAgeSeconds = 3600
	cfg.Operator.LLM.Mode = "assist"
	cfg.Operator.LLM.Command = "codex"
	cfg.Operator.LLM.Model = ""
	cfg.Operator.LLM.TimeoutSeconds = 30
	cfg.Operator.LLM.MaxTokens = 400
	cfg.GitPR.Mode = "assist"
	cfg.GitPR.CredentialMode = "local_user_auth"
	cfg.GitPR.BranchTemplate = "{ticket}/{repo}/{run}"
	cfg.GitPR.RequireAll = true
	cfg.GitPR.RequiredChecks = []string{"tests"}
	cfg.GitPR.AllowedRepos = []string{}
	cfg.GitPR.DefaultLabels = []string{}
	cfg.GitPR.DefaultReviewers = []string{}
	cfg.AgentProfiles = []AgentProfile{
		{
			Name:       "default-shell",
			Runner:     "shell",
			BasePrompt: "",
			Skills:     nil,
			RunnerOptions: RunnerOptions{
				Command: "bash",
			},
		},
	}
	cfg.Agents = []Agent{
		{
			Name:    "agent",
			Profile: "default-shell",
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
	absPath, err := filepath.Abs(finalPath)
	if err == nil {
		finalPath = absPath
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
	authorityMode := strings.TrimSpace(cfg.Docs.AuthorityMode)
	if authorityMode == "" {
		return fmt.Errorf("docs.authority_mode cannot be empty")
	}
	if model.DocAuthorityMode(authorityMode) != model.DocAuthorityModeWorkspaceActive {
		return fmt.Errorf("docs.authority_mode must be %q", model.DocAuthorityModeWorkspaceActive)
	}
	seedMode := strings.TrimSpace(cfg.Docs.SeedMode)
	if seedMode == "" {
		return fmt.Errorf("docs.seed_mode cannot be empty")
	}
	switch model.DocSeedMode(seedMode) {
	case model.DocSeedModeNone, model.DocSeedModeCopyFromRepoOnStart:
	default:
		return fmt.Errorf("docs.seed_mode must be none|copy_from_repo_on_start")
	}
	if cfg.Docs.StaleWarningSeconds <= 0 {
		return fmt.Errorf("docs.stale_warning_seconds must be > 0")
	}
	if cfg.Docs.API.RequestTimeoutSec <= 0 {
		return fmt.Errorf("docs.api.request_timeout_seconds must be > 0")
	}
	seenEndpointNames := map[string]struct{}{}
	if err := validateDocAPIEndpoints("workspace", cfg.Docs.API.WorkspaceEndpoints, seenEndpointNames); err != nil {
		return err
	}
	if err := validateDocAPIEndpoints("repo", cfg.Docs.API.RepoEndpoints, seenEndpointNames); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Workspace.BaseBranch) == "" {
		return fmt.Errorf("workspace.base_branch cannot be empty")
	}
	if cfg.Health.IdleSeconds <= 0 || cfg.Health.ActivityStalledSeconds <= 0 || cfg.Health.ProgressStalledSeconds <= 0 {
		return fmt.Errorf("health thresholds must be > 0")
	}
	if cfg.Health.ActivityStalledSeconds < cfg.Health.IdleSeconds {
		return fmt.Errorf("activity_stalled_seconds must be >= idle_seconds")
	}
	if cfg.Operator.UnhealthyConfirmations <= 0 {
		return fmt.Errorf("operator.unhealthy_confirmations must be > 0")
	}
	if cfg.Operator.RestartBudget <= 0 {
		return fmt.Errorf("operator.restart_budget must be > 0")
	}
	if cfg.Operator.RestartCooldownSeconds <= 0 {
		return fmt.Errorf("operator.restart_cooldown_seconds must be > 0")
	}
	if cfg.Operator.StaleRunAgeSeconds <= 0 {
		return fmt.Errorf("operator.stale_run_age_seconds must be > 0")
	}
	switch strings.TrimSpace(strings.ToLower(cfg.Operator.LLM.Mode)) {
	case "off", "assist", "auto":
	default:
		return fmt.Errorf("operator.llm.mode must be off|assist|auto")
	}
	if strings.TrimSpace(cfg.Operator.LLM.Command) == "" {
		return fmt.Errorf("operator.llm.command cannot be empty")
	}
	if cfg.Operator.LLM.TimeoutSeconds <= 0 {
		return fmt.Errorf("operator.llm.timeout_seconds must be > 0")
	}
	if cfg.Operator.LLM.MaxTokens <= 0 {
		return fmt.Errorf("operator.llm.max_tokens must be > 0")
	}
	switch strings.TrimSpace(strings.ToLower(cfg.GitPR.Mode)) {
	case "off", "assist", "auto":
	default:
		return fmt.Errorf("git_pr.mode must be off|assist|auto")
	}
	switch strings.TrimSpace(strings.ToLower(cfg.GitPR.CredentialMode)) {
	case "local_user_auth":
	default:
		return fmt.Errorf("git_pr.credential_mode must be local_user_auth")
	}
	if strings.TrimSpace(cfg.GitPR.BranchTemplate) == "" {
		return fmt.Errorf("git_pr.branch_template cannot be empty")
	}
	for _, check := range cfg.GitPR.RequiredChecks {
		if strings.TrimSpace(check) == "" {
			return fmt.Errorf("git_pr.required_checks cannot contain empty values")
		}
	}
	for _, repo := range cfg.GitPR.AllowedRepos {
		if strings.TrimSpace(repo) == "" {
			return fmt.Errorf("git_pr.allowed_repos cannot contain empty values")
		}
	}
	for _, label := range cfg.GitPR.DefaultLabels {
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("git_pr.default_labels cannot contain empty values")
		}
	}
	for _, reviewer := range cfg.GitPR.DefaultReviewers {
		if strings.TrimSpace(reviewer) == "" {
			return fmt.Errorf("git_pr.default_reviewers cannot contain empty values")
		}
	}
	if cfg.Execution.StepRetries < 0 {
		return fmt.Errorf("execution.step_retries must be >= 0")
	}
	if len(cfg.AgentProfiles) == 0 {
		return fmt.Errorf("agent_profiles must contain at least one entry")
	}

	profileByName := map[string]AgentProfile{}
	for _, profile := range cfg.AgentProfiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			return fmt.Errorf("agent_profiles.name cannot be empty")
		}
		if _, exists := profileByName[name]; exists {
			return fmt.Errorf("duplicate agent profile %q", name)
		}
		runner := strings.TrimSpace(strings.ToLower(profile.Runner))
		switch runner {
		case "codex":
			if strings.TrimSpace(profile.BasePrompt) == "" {
				return fmt.Errorf("agent profile %q requires non-empty base_prompt for codex runner", name)
			}
		case "shell":
			if strings.TrimSpace(profile.RunnerOptions.Command) == "" {
				return fmt.Errorf("agent profile %q requires runner_options.command for shell runner", name)
			}
		default:
			return fmt.Errorf("agent profile %q has unsupported runner %q", name, profile.Runner)
		}
		for _, skill := range profile.Skills {
			if strings.TrimSpace(skill) == "" {
				return fmt.Errorf("agent profile %q has empty skill name", name)
			}
		}
		profileByName[name] = profile
	}

	if len(cfg.Agents) == 0 {
		return fmt.Errorf("agents must contain at least one entry")
	}
	for _, agent := range cfg.Agents {
		name := strings.TrimSpace(agent.Name)
		if name == "" {
			return fmt.Errorf("agent.name cannot be empty")
		}
		profile := strings.TrimSpace(agent.Profile)
		if profile == "" {
			return fmt.Errorf("agent.profile cannot be empty")
		}
		if _, ok := profileByName[profile]; !ok {
			return fmt.Errorf("agent %q references unknown profile %q", name, profile)
		}
	}
	return nil
}

func validateDocAPIEndpoints(kind string, endpoints []DocAPIEndpoint, seenNames map[string]struct{}) error {
	for _, endpoint := range endpoints {
		name := strings.TrimSpace(endpoint.Name)
		if name == "" {
			return fmt.Errorf("docs.api.%s_endpoints.name cannot be empty", kind)
		}
		if _, exists := seenNames[name]; exists {
			return fmt.Errorf("duplicate docs API endpoint name %q", name)
		}
		seenNames[name] = struct{}{}
		if strings.TrimSpace(endpoint.BaseURL) == "" {
			return fmt.Errorf("docs.api endpoint %q base_url cannot be empty", name)
		}
		parsedBaseURL, err := url.Parse(strings.TrimSpace(endpoint.BaseURL))
		if err != nil || parsedBaseURL.Host == "" || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") {
			return fmt.Errorf("docs.api endpoint %q has invalid base_url %q", name, endpoint.BaseURL)
		}
		if strings.TrimSpace(endpoint.WebURL) != "" {
			parsedWebURL, err := url.Parse(strings.TrimSpace(endpoint.WebURL))
			if err != nil || parsedWebURL.Host == "" || (parsedWebURL.Scheme != "http" && parsedWebURL.Scheme != "https") {
				return fmt.Errorf("docs.api endpoint %q has invalid web_url %q", name, endpoint.WebURL)
			}
		}
		if strings.TrimSpace(endpoint.Repo) == "" {
			return fmt.Errorf("docs.api endpoint %q repo cannot be empty", name)
		}
		if kind == "workspace" && strings.TrimSpace(endpoint.Workspace) == "" {
			return fmt.Errorf("docs.api workspace endpoint %q requires workspace", name)
		}
	}
	return nil
}

func ResolveAgents(cfg Config, requested []string, policyPath string) ([]model.AgentSpec, error) {
	agentByName := map[string]Agent{}
	for _, agent := range cfg.Agents {
		agentByName[agent.Name] = agent
	}

	profileByName := map[string]AgentProfile{}
	for _, profile := range cfg.AgentProfiles {
		profileByName[profile.Name] = profile
	}

	resolver := newSkillResolver(policyPath)
	commandByProfile := map[string]string{}
	specForAgent := func(agent Agent) (model.AgentSpec, error) {
		profile, ok := profileByName[agent.Profile]
		if !ok {
			return model.AgentSpec{}, fmt.Errorf("agent %q references unknown profile %q", agent.Name, agent.Profile)
		}
		command, ok := commandByProfile[profile.Name]
		if !ok {
			var err error
			command, err = compileProfileCommand(profile, resolver)
			if err != nil {
				return model.AgentSpec{}, err
			}
			commandByProfile[profile.Name] = command
		}
		return model.AgentSpec{
			Name:    agent.Name,
			Profile: profile.Name,
			Runner:  profile.Runner,
			Skills:  append([]string(nil), profile.Skills...),
			Command: command,
		}, nil
	}

	if len(requested) == 0 {
		out := make([]model.AgentSpec, 0, len(cfg.Agents))
		for _, agent := range cfg.Agents {
			spec, err := specForAgent(agent)
			if err != nil {
				return nil, err
			}
			out = append(out, spec)
		}
		return out, nil
	}

	out := make([]model.AgentSpec, 0, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		agent, ok := agentByName[name]
		if !ok {
			return nil, fmt.Errorf("requested agent %q not found in policy", name)
		}
		spec, err := specForAgent(agent)
		if err != nil {
			return nil, err
		}
		out = append(out, spec)
	}
	return out, nil
}

func compileProfileCommand(profile AgentProfile, resolver skillResolver) (string, error) {
	runner := strings.TrimSpace(strings.ToLower(profile.Runner))
	switch runner {
	case "shell":
		return strings.TrimSpace(profile.RunnerOptions.Command), nil
	case "codex":
		prompt, err := buildCodexPrompt(profile, resolver)
		if err != nil {
			return "", err
		}
		command := "codex exec"
		if profile.RunnerOptions.FullAuto {
			command += " --full-auto"
		}
		return command + " " + quoteShell(prompt), nil
	default:
		return "", fmt.Errorf("unsupported runner %q", profile.Runner)
	}
}

func buildCodexPrompt(profile AgentProfile, resolver skillResolver) (string, error) {
	basePrompt := strings.TrimSpace(profile.BasePrompt)
	if len(profile.Skills) == 0 {
		return basePrompt, nil
	}

	skillLines := make([]string, 0, len(profile.Skills))
	for _, skill := range profile.Skills {
		skill = strings.TrimSpace(skill)
		path, err := resolver.resolve(skill)
		if err != nil {
			return "", err
		}
		skillLines = append(skillLines, fmt.Sprintf("- %s: %s", skill, path))
	}

	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\nRequired skills (read and apply these before implementation):\n")
	b.WriteString(strings.Join(skillLines, "\n"))
	return b.String(), nil
}

type skillResolver struct {
	roots []string
}

func newSkillResolver(policyPath string) skillResolver {
	roots := []string{}
	seen := map[string]struct{}{}
	addRoot := func(path string) {
		path = filepath.Clean(path)
		if strings.TrimSpace(path) == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		roots = append(roots, path)
	}

	if strings.TrimSpace(policyPath) != "" {
		absPath, err := filepath.Abs(policyPath)
		if err == nil {
			repoRoot := filepath.Dir(filepath.Dir(absPath))
			addRoot(filepath.Join(repoRoot, ".metawsm", "skills"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		addRoot(filepath.Join(home, ".codex", "skills"))
	}
	return skillResolver{roots: roots}
}

func (r skillResolver) resolve(skill string) (string, error) {
	candidates := make([]string, 0, len(r.roots))
	for _, root := range r.roots {
		path := filepath.Join(root, skill, "SKILL.md")
		candidates = append(candidates, path)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("skill %q not found; checked: %s", skill, strings.Join(candidates, ", "))
}

func RenderSessionName(pattern string, agentName string, workspaceName string) string {
	s := strings.ReplaceAll(pattern, "{agent}", sanitizeToken(agentName))
	s = strings.ReplaceAll(s, "{workspace}", sanitizeToken(workspaceName))
	if strings.TrimSpace(s) == "" {
		return fmt.Sprintf("%s-%s", sanitizeToken(agentName), sanitizeToken(workspaceName))
	}
	return s
}

func RenderGitBranch(template string, ticket string, repo string, runID string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = "{ticket}/{repo}/{run}"
	}
	template = strings.ReplaceAll(template, "{ticket}", sanitizeToken(ticket))
	template = strings.ReplaceAll(template, "{repo}", sanitizeToken(repo))
	template = strings.ReplaceAll(template, "{run}", sanitizeToken(runID))

	segments := strings.Split(template, "/")
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		out = append(out, sanitizeToken(segment))
	}
	if len(out) == 0 {
		return fmt.Sprintf("%s/%s/%s", sanitizeToken(ticket), sanitizeToken(repo), sanitizeToken(runID))
	}
	return strings.Join(out, "/")
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

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
