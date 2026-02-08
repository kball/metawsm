package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"metawsm/internal/docfederation"
	"metawsm/internal/model"
	"metawsm/internal/orchestrator"
	"metawsm/internal/policy"
)

type multiValueFlag []string

func (f *multiValueFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *multiValueFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "run":
		err = runCommand(args)
	case "bootstrap":
		err = bootstrapCommand(args)
	case "status":
		err = statusCommand(args)
	case "auth":
		err = authCommand(args)
	case "watch":
		err = watchCommand(args)
	case "operator":
		err = operatorCommand(args)
	case "guide":
		err = guideCommand(args)
	case "resume":
		err = resumeCommand(args)
	case "stop":
		err = stopCommand(args)
	case "restart":
		err = restartCommand(args)
	case "cleanup":
		err = cleanupCommand(args)
	case "commit":
		err = commitCommand(args)
	case "pr":
		err = prCommand(args)
	case "merge":
		err = mergeCommand(args)
	case "iterate":
		err = iterateCommand(args)
	case "close":
		err = closeCommand(args)
	case "policy-init":
		err = policyInitCommand(args)
	case "tui":
		err = tuiCommand(args)
	case "docs":
		err = docsCommand(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		err = fmt.Errorf("unknown command %q", command)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var tickets multiValueFlag
	var repos multiValueFlag
	var agents multiValueFlag
	var runID string
	var strategy string
	var docHomeRepo string
	var docRepo string
	var docAuthorityMode string
	var docSeedMode string
	var baseBranch string
	var policyPath string
	var dbPath string
	var dryRun bool

	fs.Var(&tickets, "ticket", "Ticket identifier (repeatable, or comma-separated)")
	fs.Var(&repos, "repos", "Repositories list (repeatable, or comma-separated)")
	fs.StringVar(&docHomeRepo, "doc-home-repo", "", "Canonical repository for ticket docs in the run (defaults to first --repos entry)")
	fs.StringVar(&docRepo, "doc-repo", "", "Deprecated alias for --doc-home-repo")
	fs.StringVar(&docAuthorityMode, "doc-authority-mode", "", "Doc authority mode (workspace_active)")
	fs.StringVar(&docSeedMode, "doc-seed-mode", "", "Doc seed mode (none|copy_from_repo_on_start)")
	fs.Var(&agents, "agent", "Agent name from policy (repeatable, or comma-separated)")
	fs.StringVar(&runID, "run-id", "", "Run identifier (optional)")
	fs.StringVar(&strategy, "workspace-strategy", "", "Workspace strategy: create|fork|reuse")
	fs.StringVar(&baseBranch, "base-branch", "", "Branch to use as workspace start point (default from policy, usually main)")
	fs.StringVar(&policyPath, "policy", "", "Path to policy file (defaults to .metawsm/policy.json)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Plan only; do not execute steps")
	if err := fs.Parse(args); err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Run(context.Background(), orchestrator.RunOptions{
		RunID:             runID,
		Tickets:           tickets,
		Repos:             repos,
		DocRepo:           docRepo,
		DocHomeRepo:       docHomeRepo,
		DocAuthorityMode:  docAuthorityMode,
		DocSeedMode:       docSeedMode,
		BaseBranch:        baseBranch,
		AgentNames:        agents,
		WorkspaceStrategy: model.WorkspaceStrategy(strings.TrimSpace(strategy)),
		PolicyPath:        policyPath,
		DryRun:            dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Run ID: %s\n", result.RunID)
	for _, step := range result.Steps {
		fmt.Printf("  [%02d] %s (%s) status=%s\n", step.Index, step.Name, step.Kind, step.Status)
		if strings.TrimSpace(step.Command) != "" {
			fmt.Printf("       %s\n", step.Command)
		}
	}
	if dryRun {
		fmt.Println("Run planned in dry-run mode.")
	}
	return nil
}

func bootstrapCommand(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	var ticket string
	var repos multiValueFlag
	var agents multiValueFlag
	var runID string
	var strategy string
	var docHomeRepo string
	var docRepo string
	var docAuthorityMode string
	var docSeedMode string
	var baseBranch string
	var policyPath string
	var dbPath string
	var dryRun bool
	var goal string
	var scope string
	var doneCriteria string
	var constraints string
	var mergeIntent string

	fs.StringVar(&ticket, "ticket", "", "Ticket identifier")
	fs.Var(&repos, "repos", "Repositories list (repeatable, or comma-separated) [required]")
	fs.StringVar(&docHomeRepo, "doc-home-repo", "", "Canonical repository for ticket docs in the run (defaults to first --repos entry)")
	fs.StringVar(&docRepo, "doc-repo", "", "Deprecated alias for --doc-home-repo")
	fs.StringVar(&docAuthorityMode, "doc-authority-mode", "", "Doc authority mode (workspace_active)")
	fs.StringVar(&docSeedMode, "doc-seed-mode", "", "Doc seed mode (none|copy_from_repo_on_start)")
	fs.Var(&agents, "agent", "Agent name from policy (repeatable, or comma-separated)")
	fs.StringVar(&runID, "run-id", "", "Run identifier (optional)")
	fs.StringVar(&strategy, "workspace-strategy", "", "Workspace strategy: create|fork|reuse")
	fs.StringVar(&baseBranch, "base-branch", "", "Branch to use as workspace start point (default from policy, usually main)")
	fs.StringVar(&policyPath, "policy", "", "Path to policy file (defaults to .metawsm/policy.json)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Plan only; do not execute setup steps")
	fs.StringVar(&goal, "goal", "", "Goal for what should be built")
	fs.StringVar(&scope, "scope", "", "Scope (areas/files expected to change)")
	fs.StringVar(&doneCriteria, "done-criteria", "", "Done criteria (tests/checks/acceptance)")
	fs.StringVar(&constraints, "constraints", "", "Constraints, non-goals, or risk boundaries")
	fs.StringVar(&mergeIntent, "merge-intent", "", "Merge intent (or 'default')")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return fmt.Errorf("--ticket is required")
	}
	repoTokens := normalizeInputTokens(repos)
	if len(repoTokens) == 0 {
		return fmt.Errorf("--repos is required for bootstrap")
	}

	interactive := isInteractiveStdin()
	brief, err := collectBootstrapBrief(os.Stdin, os.Stdout, interactive, ticket, model.RunBrief{
		Ticket:       ticket,
		Goal:         goal,
		Scope:        scope,
		DoneCriteria: doneCriteria,
		Constraints:  constraints,
		MergeIntent:  mergeIntent,
	})
	if err != nil {
		return err
	}
	if err := ensureTicketExists(context.Background(), ticket, brief.Goal); err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Run(context.Background(), orchestrator.RunOptions{
		RunID:             runID,
		Tickets:           []string{ticket},
		Repos:             repoTokens,
		DocRepo:           docRepo,
		DocHomeRepo:       docHomeRepo,
		DocAuthorityMode:  docAuthorityMode,
		DocSeedMode:       docSeedMode,
		BaseBranch:        baseBranch,
		AgentNames:        agents,
		WorkspaceStrategy: model.WorkspaceStrategy(strings.TrimSpace(strategy)),
		PolicyPath:        policyPath,
		DryRun:            dryRun,
		Mode:              model.RunModeBootstrap,
		RunBrief:          &brief,
	})
	if err != nil {
		return err
	}
	if err := createBootstrapBriefDoc(context.Background(), ticket, result.RunID, brief); err != nil {
		return err
	}

	fmt.Printf("Bootstrap Run ID: %s\n", result.RunID)
	fmt.Printf("Ticket: %s\n", ticket)
	fmt.Printf("Repos: %s\n", strings.Join(repoTokens, ","))
	for _, step := range result.Steps {
		fmt.Printf("  [%02d] %s (%s) status=%s\n", step.Index, step.Name, step.Kind, step.Status)
		if strings.TrimSpace(step.Command) != "" {
			fmt.Printf("       %s\n", step.Command)
		}
	}
	if dryRun {
		fmt.Println("Bootstrap planned in dry-run mode.")
	} else {
		fmt.Println("Bootstrap setup complete. Use `metawsm status --run-id` to monitor guidance/completion.")
	}
	return nil
}

func guideCommand(args []string) error {
	fs := flag.NewFlagSet("guide", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var answer string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (guide latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&answer, "answer", "", "Guidance answer for the pending question")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("--answer is required")
	}
	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return err
	}
	result, err := service.Guide(context.Background(), runID, answer)
	if err != nil {
		return err
	}
	fmt.Printf("Guidance answered for run %s (id=%d %s@%s).\n", result.RunID, result.GuidanceID, result.AgentName, result.WorkspaceName)
	return nil
}

func statusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (status latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return err
	}
	status, err := service.Status(context.Background(), runID)
	if err != nil {
		return err
	}
	fmt.Print(status)
	return nil
}

type authRepoCheck struct {
	WorkspaceName string
	Repo          string
	RepoPath      string
	GitUserName   string
	GitUserEmail  string
	RemoteOrigin  string
	Ready         bool
	Error         string
}

func authCommand(args []string) error {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var policyPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (check latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&policyPath, "policy", "", "Path to policy file (defaults to .metawsm/policy.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || !strings.EqualFold(strings.TrimSpace(rest[0]), "check") {
		return fmt.Errorf("usage: metawsm auth check [--run-id RUN_ID | --ticket TICKET] [--policy PATH]")
	}

	cfg, _, err := policy.Load(policyPath)
	if err != nil {
		return err
	}
	credentialMode := strings.TrimSpace(strings.ToLower(cfg.GitPR.CredentialMode))
	if credentialMode == "" {
		credentialMode = "local_user_auth"
	}

	if credentialMode != "local_user_auth" {
		return fmt.Errorf("unsupported git_pr.credential_mode %q (expected local_user_auth)", cfg.GitPR.CredentialMode)
	}

	ctx := context.Background()
	ghInstalled, ghAuthed, ghActor, ghDetail := checkGitHubLocalAuth(ctx)

	effectiveRunID := ""
	repoChecks := []authRepoCheck{}
	if strings.TrimSpace(runID) != "" || strings.TrimSpace(ticket) != "" {
		service, err := orchestrator.NewService(dbPath)
		if err != nil {
			return err
		}
		runID, ticket, err = requireRunSelector(runID, ticket)
		if err != nil {
			return err
		}
		effectiveRunID, err = service.ResolveRunID(runID, ticket)
		if err != nil {
			return err
		}
		runCtx, err := service.OperatorRunContext(effectiveRunID)
		if err != nil {
			return err
		}
		repoChecks, err = checkRunGitCredentials(ctx, runCtx)
		if err != nil {
			return err
		}
	}

	allReposReady := true
	for _, check := range repoChecks {
		if !check.Ready {
			allReposReady = false
			break
		}
	}
	pushReady := ghInstalled && ghAuthed && allReposReady
	prReady := pushReady

	fmt.Printf("Credential mode: %s\n", credentialMode)
	if effectiveRunID != "" {
		fmt.Printf("Run: %s\n", effectiveRunID)
	}
	fmt.Printf("GitHub CLI: installed=%t authed=%t actor=%s\n", ghInstalled, ghAuthed, emptyValue(ghActor, "unknown"))
	if strings.TrimSpace(ghDetail) != "" {
		fmt.Printf("  detail=%s\n", ghDetail)
	}
	if len(repoChecks) > 0 {
		fmt.Println("Repository checks:")
		for _, check := range repoChecks {
			fmt.Printf("  - %s/%s ready=%t path=%s\n", check.WorkspaceName, check.Repo, check.Ready, emptyValue(check.RepoPath, "n/a"))
			if check.Ready {
				fmt.Printf("    git_user=%s <%s>\n", check.GitUserName, check.GitUserEmail)
				fmt.Printf("    origin=%s\n", check.RemoteOrigin)
			} else if strings.TrimSpace(check.Error) != "" {
				fmt.Printf("    error=%s\n", check.Error)
			}
		}
	}
	fmt.Printf("Push ready: %t\n", pushReady)
	fmt.Printf("PR ready: %t\n", prReady)

	if !pushReady {
		return fmt.Errorf("auth check failed: push/pr not ready")
	}
	return nil
}

func checkGitHubLocalAuth(ctx context.Context) (installed bool, authed bool, actor string, detail string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return false, false, "", "gh CLI not found on PATH"
	}

	statusCmd := exec.CommandContext(ctx, "gh", "auth", "status", "-h", "github.com")
	statusOut, statusErr := statusCmd.CombinedOutput()
	if statusErr != nil {
		return true, false, "", strings.TrimSpace(string(statusOut))
	}

	actorCmd := exec.CommandContext(ctx, "gh", "api", "user", "--jq", ".login")
	actorOut, actorErr := actorCmd.CombinedOutput()
	if actorErr != nil {
		return true, true, "", strings.TrimSpace(string(actorOut))
	}

	return true, true, strings.TrimSpace(string(actorOut)), strings.TrimSpace(string(statusOut))
}

func checkRunGitCredentials(ctx context.Context, runCtx orchestrator.OperatorRunContext) ([]authRepoCheck, error) {
	workspaceSet := map[string]struct{}{}
	for _, agent := range runCtx.Agents {
		workspaceName := strings.TrimSpace(agent.WorkspaceName)
		if workspaceName == "" {
			continue
		}
		workspaceSet[workspaceName] = struct{}{}
	}

	repos := normalizeInputTokens(runCtx.Repos)
	if len(repos) == 0 && strings.TrimSpace(runCtx.DocHomeRepo) != "" {
		repos = []string{strings.TrimSpace(runCtx.DocHomeRepo)}
	}

	workspaceNames := make([]string, 0, len(workspaceSet))
	for workspaceName := range workspaceSet {
		workspaceNames = append(workspaceNames, workspaceName)
	}
	sort.Strings(workspaceNames)

	estimated := len(repos)
	if estimated == 0 {
		estimated = 1
	}
	checks := make([]authRepoCheck, 0, len(workspaceNames)*estimated)
	for _, workspaceName := range workspaceNames {
		workspacePath, err := operatorResolveWorkspacePath(workspaceName)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			check := authRepoCheck{
				WorkspaceName: workspaceName,
				Repo:          repo,
			}
			repoPath, err := resolveWorkspaceRepoPath(workspacePath, repo, len(repos))
			if err != nil {
				check.Error = err.Error()
				checks = append(checks, check)
				continue
			}
			check.RepoPath = repoPath
			userName, err := gitConfigValue(ctx, repoPath, "user.name")
			if err != nil {
				check.Error = err.Error()
				checks = append(checks, check)
				continue
			}
			userEmail, err := gitConfigValue(ctx, repoPath, "user.email")
			if err != nil {
				check.Error = err.Error()
				checks = append(checks, check)
				continue
			}
			originURL, err := gitRemoteOrigin(ctx, repoPath)
			if err != nil {
				check.Error = err.Error()
				checks = append(checks, check)
				continue
			}
			check.GitUserName = userName
			check.GitUserEmail = userEmail
			check.RemoteOrigin = originURL
			check.Ready = true
			checks = append(checks, check)
		}
	}
	return checks, nil
}

func resolveWorkspaceRepoPath(workspacePath string, repo string, repoCount int) (string, error) {
	workspacePath = filepath.Clean(strings.TrimSpace(workspacePath))
	repo = strings.TrimSpace(repo)
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path is empty")
	}
	if repo == "" {
		return "", fmt.Errorf("repo name is empty")
	}

	repoPath := filepath.Join(workspacePath, repo)
	if info, err := os.Stat(repoPath); err == nil && info.IsDir() {
		return repoPath, nil
	}
	if repoCount == 1 && operatorIsGitRepo(workspacePath) {
		return workspacePath, nil
	}
	return "", fmt.Errorf("repo path not found for %s in workspace %s", repo, workspacePath)
}

func gitConfigValue(ctx context.Context, repoPath string, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--get", key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git config %s failed: %s", key, strings.TrimSpace(string(out)))
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git config %s is empty", key)
	}
	return value, nil
}

func gitRemoteOrigin(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin failed: %s", strings.TrimSpace(string(out)))
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git remote origin is empty")
	}
	return value, nil
}

type watchSnapshot struct {
	RunID              string
	RunStatus          string
	Tickets            string
	HasGuidance        bool
	HasUnhealthyAgents bool
	HasDirtyDiffs      bool
	DraftPullRequests  int
	OpenPullRequests   int
	GuidanceItems      []string
	UnhealthyAgents    []watchAgentIssue
}

type watchAgentIssue struct {
	Agent        string
	Session      string
	Status       string
	Health       string
	LastActivity string
	LastProgress string
	ActivityAge  string
	ProgressAge  string
	Reason       string
}

type operatorSessionEvidence struct {
	Session      string
	HasSession   bool
	LastActivity *time.Time
	ExitCode     *int
}

type operatorSessionProbe func(ctx context.Context, session string) (operatorSessionEvidence, error)

var operatorAgentExitRegex = regexp.MustCompile(`\[metawsm\] agent command exited with status ([0-9]+)`)
var operatorDocmgrDocsRootRegex = regexp.MustCompile("Docs root:\\s+`([^`]+)`")
var operatorDocmgrTicketPathRegex = regexp.MustCompile("Path:\\s+`([^`]+)`")

type watchMode int

const (
	watchModeSingleRun watchMode = iota
	watchModeAllActiveRuns
)

func watchCommand(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var intervalSeconds int
	var notifyCmd string
	var bell bool
	var all bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (watch latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.IntVar(&intervalSeconds, "interval", 15, "Heartbeat interval in seconds")
	fs.StringVar(&notifyCmd, "notify-cmd", "", "Optional shell command to run on alert (receives METAWSM_* env vars)")
	fs.BoolVar(&bell, "bell", true, "Emit terminal bell on alert")
	fs.BoolVar(&all, "all", false, "Watch all active runs/tickets/agents")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if intervalSeconds <= 0 {
		return fmt.Errorf("--interval must be > 0")
	}
	mode, err := resolveWatchMode(runID, ticket, all)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	selectedRunID := ""
	selectedTicket := strings.TrimSpace(ticket)
	if mode == watchModeSingleRun {
		runID, ticket, err = requireRunSelector(runID, ticket)
		if err != nil {
			return err
		}
		selectedRunID, err = service.ResolveRunID(runID, ticket)
		if err != nil {
			return err
		}
		selectedTicket = strings.TrimSpace(ticket)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if mode == watchModeAllActiveRuns {
		fmt.Printf("Watching all active runs (interval=%ds).\n", intervalSeconds)
	} else {
		fmt.Printf("Watching run %s (interval=%ds).\n", selectedRunID, intervalSeconds)
	}
	fmt.Println("Alerts: guidance needed, run done/failed/stopped, agent unhealthy.")

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	lastAlertByRun := map[string]string{}
	trackedRuns := map[string]struct{}{}
	if mode == watchModeSingleRun {
		trackedRuns[selectedRunID] = struct{}{}
	}

	for {
		snapshots := []watchSnapshot{}
		if mode == watchModeSingleRun {
			snapshot, err := loadWatchSnapshot(ctx, service, selectedRunID, selectedTicket)
			if err != nil {
				if isWatchStopError(ctx, err) {
					fmt.Println("\nWatch stopped.")
					return nil
				}
				return err
			}
			snapshots = append(snapshots, snapshot)
		} else {
			var err error
			snapshots, err = loadWatchSnapshotsAll(ctx, service, trackedRuns)
			if err != nil {
				if isWatchStopError(ctx, err) {
					fmt.Println("\nWatch stopped.")
					return nil
				}
				return err
			}
		}
		sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].RunID < snapshots[j].RunID })

		now := time.Now().Format(time.RFC3339)
		if len(snapshots) == 0 {
			fmt.Printf("[%s] heartbeat active_runs=0\n", now)
		}

		for _, snapshot := range snapshots {
			fmt.Printf("[%s] heartbeat run=%s status=%s guidance=%t unhealthy_agents=%t\n",
				now,
				snapshot.RunID,
				emptyValue(snapshot.RunStatus, "unknown"),
				snapshot.HasGuidance,
				snapshot.HasUnhealthyAgents,
			)

			event, message, terminal := classifyWatchEvent(snapshot)
			lastEvent := lastAlertByRun[snapshot.RunID]
			if event == "" {
				delete(lastAlertByRun, snapshot.RunID)
				continue
			}
			if lastEvent != event || terminal {
				lastAlertByRun[snapshot.RunID] = event
				fmt.Printf("[%s] ALERT %s: %s\n", now, event, message)
				for _, line := range buildWatchDirectionHints(snapshot, event) {
					fmt.Printf("  %s\n", line)
				}
				if bell {
					fmt.Print("\a")
				}
				if err := runWatchNotifyCommand(ctx, notifyCmd, event, message, snapshot); err != nil {
					fmt.Fprintf(os.Stderr, "warning: notify command failed: %v\n", err)
				}
			}
			if mode == watchModeSingleRun && terminal {
				return nil
			}
			if mode == watchModeAllActiveRuns && isTerminalRunStatus(snapshot.RunStatus) {
				delete(trackedRuns, snapshot.RunID)
				delete(lastAlertByRun, snapshot.RunID)
			}
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nWatch stopped.")
			return nil
		case <-ticker.C:
		}
	}
}

func operatorCommand(args []string) error {
	fs := flag.NewFlagSet("operator", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var policyPath string
	var llmMode string
	var intervalSeconds int
	var notifyCmd string
	var bell bool
	var all bool
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (operate on latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&policyPath, "policy", "", "Path to policy file (defaults to .metawsm/policy.json)")
	fs.StringVar(&llmMode, "llm-mode", "", "Operator LLM mode override (off|assist|auto)")
	fs.IntVar(&intervalSeconds, "interval", 15, "Heartbeat interval in seconds")
	fs.StringVar(&notifyCmd, "notify-cmd", "", "Optional shell command to run on alert (receives METAWSM_* env vars)")
	fs.BoolVar(&bell, "bell", true, "Emit terminal bell on alert")
	fs.BoolVar(&all, "all", false, "Operate on all active runs/tickets/agents")
	fs.BoolVar(&dryRun, "dry-run", false, "Observe only; do not execute actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if intervalSeconds <= 0 {
		return fmt.Errorf("--interval must be > 0")
	}

	mode, err := resolveWatchMode(runID, ticket, all)
	if err != nil {
		return err
	}

	cfg, _, err := policy.Load(policyPath)
	if err != nil {
		return err
	}
	effectiveLLMMode, err := resolveOperatorLLMMode(llmMode, cfg.Operator.LLM.Mode)
	if err != nil {
		return err
	}
	staleAge := time.Duration(cfg.Operator.StaleRunAgeSeconds) * time.Second
	runtimeRecentWindow := time.Duration(cfg.Health.ActivityStalledSeconds) * time.Second
	restartCooldown := time.Duration(cfg.Operator.RestartCooldownSeconds) * time.Second

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	var llmAdapter operatorLLMAdapter
	if effectiveLLMMode != "off" {
		llmAdapter = newCodexCLIAdapter(
			cfg.Operator.LLM.Command,
			cfg.Operator.LLM.Model,
			cfg.Operator.LLM.MaxTokens,
			time.Duration(cfg.Operator.LLM.TimeoutSeconds)*time.Second,
			nil,
		)
	}

	selectedRunID := ""
	selectedTicket := strings.TrimSpace(ticket)
	if mode == watchModeSingleRun {
		runID, ticket, err = requireRunSelector(runID, ticket)
		if err != nil {
			return err
		}
		selectedRunID, err = service.ResolveRunID(runID, ticket)
		if err != nil {
			return err
		}
		selectedTicket = strings.TrimSpace(ticket)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if mode == watchModeAllActiveRuns {
		fmt.Printf("Operator supervising all active runs (interval=%ds llm_mode=%s dry_run=%t).\n", intervalSeconds, effectiveLLMMode, dryRun)
	} else {
		fmt.Printf("Operator supervising run %s (interval=%ds llm_mode=%s dry_run=%t).\n", selectedRunID, intervalSeconds, effectiveLLMMode, dryRun)
	}
	fmt.Println("Operator signals: guidance-needed, stale-candidate-verified, stale-candidate-rejected, commit-ready, pr-ready.")

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	lastAlertByRun := map[string]string{}
	trackedRuns := map[string]struct{}{}
	consecutiveUnhealthyByRun := map[string]int{}
	if mode == watchModeSingleRun {
		trackedRuns[selectedRunID] = struct{}{}
	}

	for {
		activeRuns, err := service.ActiveRuns()
		if err != nil {
			return err
		}
		activeByRunID := map[string]model.RunRecord{}
		for _, run := range activeRuns {
			activeByRunID[run.RunID] = run
		}

		snapshots := []watchSnapshot{}
		if mode == watchModeSingleRun {
			snapshot, err := loadWatchSnapshot(ctx, service, selectedRunID, selectedTicket)
			if err != nil {
				if isWatchStopError(ctx, err) {
					fmt.Println("\nOperator stopped.")
					return nil
				}
				return err
			}
			snapshots = append(snapshots, snapshot)
		} else {
			snapshots, err = loadWatchSnapshotsAll(ctx, service, trackedRuns)
			if err != nil {
				if isWatchStopError(ctx, err) {
					fmt.Println("\nOperator stopped.")
					return nil
				}
				return err
			}
		}
		sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].RunID < snapshots[j].RunID })

		now := time.Now()
		if len(snapshots) == 0 {
			fmt.Printf("[%s] operator heartbeat active_runs=0\n", now.Format(time.RFC3339))
		}

		for _, snapshot := range snapshots {
			fmt.Printf("[%s] operator heartbeat run=%s status=%s guidance=%t unhealthy_agents=%t\n",
				now.Format(time.RFC3339),
				snapshot.RunID,
				emptyValue(snapshot.RunStatus, "unknown"),
				snapshot.HasGuidance,
				snapshot.HasUnhealthyAgents,
			)

			runRecord, ok := activeByRunID[snapshot.RunID]
			if !ok {
				delete(lastAlertByRun, snapshot.RunID)
				delete(consecutiveUnhealthyByRun, snapshot.RunID)
				continue
			}

			if snapshot.HasUnhealthyAgents && strings.EqualFold(snapshot.RunStatus, string(model.RunStatusRunning)) {
				consecutiveUnhealthyByRun[snapshot.RunID]++
			} else {
				consecutiveUnhealthyByRun[snapshot.RunID] = 0
			}

			ruleDecision, err := buildOperatorRuleDecision(
				ctx,
				service,
				snapshot,
				runRecord,
				now,
				staleAge,
				runtimeRecentWindow,
				cfg.Operator.UnhealthyConfirmations,
				cfg.Operator.RestartBudget,
				consecutiveUnhealthyByRun[snapshot.RunID],
				probeOperatorSessionEvidence,
				cfg.GitPR.Mode,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: operator rule evaluation failed for run %s: %v\n", snapshot.RunID, err)
				continue
			}

			var llmReply *operatorLLMResponse
			if llmAdapter != nil {
				reply, err := llmAdapter.Propose(ctx, operatorLLMRequest{
					RunID:           snapshot.RunID,
					RunStatus:       snapshot.RunStatus,
					Tickets:         snapshot.Tickets,
					HasGuidance:     snapshot.HasGuidance,
					HasUnhealthy:    snapshot.HasUnhealthyAgents,
					RuleIntent:      ruleDecision.Intent,
					RuleReason:      ruleDecision.Reason,
					UnhealthyAgents: snapshot.UnhealthyAgents,
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: llm proposal failed for run %s: %v\n", snapshot.RunID, err)
				} else {
					llmReply = &reply
				}
			}

			merged := mergeOperatorDecisions(effectiveLLMMode, ruleDecision, llmReply)
			if merged.Intent == operatorIntentNoop {
				delete(lastAlertByRun, snapshot.RunID)
				continue
			}
			event, message := operatorEventMessage(snapshot, merged)
			if lastAlertByRun[snapshot.RunID] == event {
				continue
			}
			lastAlertByRun[snapshot.RunID] = event
			fmt.Printf("[%s] ALERT %s: %s\n", now.Format(time.RFC3339), event, message)
			fmt.Printf("  decision_source=%s llm_mode=%s intent=%s\n", merged.Source, effectiveLLMMode, merged.Intent)
			if llmReply != nil {
				fmt.Printf("  llm intent=%s confidence=%.2f reason=%s\n", llmReply.Intent, llmReply.Confidence, llmReply.Reason)
			}

			shouldExecute := merged.Execute && !dryRun
			if shouldExecute {
				if err := executeOperatorAction(ctx, service, snapshot.RunID, merged.Intent); err != nil {
					fmt.Fprintf(os.Stderr, "warning: operator action failed for run %s intent=%s: %v\n", snapshot.RunID, merged.Intent, err)
				} else if merged.Intent == operatorIntentAutoRestart {
					current, err := service.GetOperatorRunState(snapshot.RunID)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: read operator restart state for run %s failed: %v\n", snapshot.RunID, err)
					} else {
						state := model.OperatorRunState{RunID: snapshot.RunID}
						if current != nil {
							state = *current
						}
						nowCopy := now
						cooldownUntil := nowCopy.Add(restartCooldown)
						state.RestartAttempts++
						state.LastRestartAt = &nowCopy
						state.CooldownUntil = &cooldownUntil
						state.UpdatedAt = nowCopy
						if err := service.UpsertOperatorRunState(state); err != nil {
							fmt.Fprintf(os.Stderr, "warning: persist operator restart state for run %s failed: %v\n", snapshot.RunID, err)
						}
					}
				}
			} else if merged.Intent == operatorIntentAutoStopStale || merged.Intent == operatorIntentAutoRestart || merged.Intent == operatorIntentCommitReady || merged.Intent == operatorIntentPRReady {
				fmt.Printf("  action not executed (dry_run=%t llm_mode=%s)\n", dryRun, effectiveLLMMode)
			}

			if merged.Intent == operatorIntentEscalateGuidance || merged.Intent == operatorIntentEscalateBlocked {
				if err := appendOperatorEscalationSummary(ctx, service, snapshot.RunID, merged.Intent, message); err != nil {
					fmt.Fprintf(os.Stderr, "warning: escalation summary write failed for run %s: %v\n", snapshot.RunID, err)
				}
			}
			if bell {
				fmt.Print("\a")
			}
			if err := runWatchNotifyCommand(ctx, notifyCmd, event, message, snapshot); err != nil {
				fmt.Fprintf(os.Stderr, "warning: notify command failed: %v\n", err)
			}
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nOperator stopped.")
			return nil
		case <-ticker.C:
		}
	}
}

func buildOperatorRuleDecision(
	ctx context.Context,
	service *orchestrator.Service,
	snapshot watchSnapshot,
	runRecord model.RunRecord,
	now time.Time,
	staleAge time.Duration,
	runtimeRecentWindow time.Duration,
	unhealthyConfirmations int,
	restartBudget int,
	consecutiveUnhealthy int,
	probe operatorSessionProbe,
	gitPRMode string,
) (operatorRuleDecision, error) {
	if snapshot.HasGuidance {
		return operatorRuleDecision{
			Intent:  operatorIntentEscalateGuidance,
			Reason:  "operator input is required",
			Execute: false,
		}, nil
	}

	isStale, staleReason := classifyStaleRunCandidate(snapshot, runRecord, now, staleAge)
	if isStale {
		verified, verifyReason, err := verifyStaleRuntimeEvidence(ctx, snapshot, now, runtimeRecentWindow, probe)
		if err != nil {
			return operatorRuleDecision{}, err
		}
		if verified {
			return operatorRuleDecision{
				Intent:  operatorIntentAutoStopStale,
				Reason:  staleReason + "; " + verifyReason,
				Execute: true,
			}, nil
		}
		return operatorRuleDecision{
			Intent:  operatorIntentNoop,
			Reason:  staleReason + "; " + verifyReason,
			Execute: false,
		}, nil
	}

	if snapshot.HasUnhealthyAgents && strings.EqualFold(snapshot.RunStatus, string(model.RunStatusRunning)) {
		if consecutiveUnhealthy < unhealthyConfirmations {
			return operatorRuleDecision{
				Intent:  operatorIntentNoop,
				Reason:  fmt.Sprintf("awaiting corroboration (%d/%d unhealthy intervals)", consecutiveUnhealthy, unhealthyConfirmations),
				Execute: false,
			}, nil
		}
		state, err := service.GetOperatorRunState(snapshot.RunID)
		if err != nil {
			return operatorRuleDecision{}, err
		}
		if state != nil && state.RestartAttempts >= restartBudget {
			return operatorRuleDecision{
				Intent:  operatorIntentEscalateBlocked,
				Reason:  fmt.Sprintf("restart budget exhausted (%d/%d)", state.RestartAttempts, restartBudget),
				Execute: false,
			}, nil
		}
		if state != nil && state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
			return operatorRuleDecision{
				Intent:  operatorIntentNoop,
				Reason:  fmt.Sprintf("restart cooldown active until %s", state.CooldownUntil.Format(time.RFC3339)),
				Execute: false,
			}, nil
		}
		return operatorRuleDecision{
			Intent:  operatorIntentAutoRestart,
			Reason:  "unhealthy state corroborated and restart budget available",
			Execute: true,
		}, nil
	}

	mode := strings.TrimSpace(strings.ToLower(gitPRMode))
	if mode == "" {
		mode = "assist"
	}
	if strings.EqualFold(snapshot.RunStatus, string(model.RunStatusComplete)) && mode != "off" {
		if snapshot.HasDirtyDiffs {
			return operatorRuleDecision{
				Intent:  operatorIntentCommitReady,
				Reason:  "run completed with dirty repository diffs; commit workflow is ready",
				Execute: mode == "auto",
			}, nil
		}
		if snapshot.DraftPullRequests > 0 {
			return operatorRuleDecision{
				Intent:  operatorIntentPRReady,
				Reason:  fmt.Sprintf("run has %d draft pull request record(s); PR creation is ready", snapshot.DraftPullRequests),
				Execute: mode == "auto",
			}, nil
		}
	}

	return operatorRuleDecision{
		Intent:  operatorIntentNoop,
		Reason:  "no deterministic action required",
		Execute: false,
	}, nil
}

func operatorEventMessage(snapshot watchSnapshot, decision operatorMergedDecision) (string, string) {
	switch decision.Intent {
	case operatorIntentEscalateGuidance:
		return "guidance_needed", decision.Reason
	case operatorIntentAutoStopStale:
		return "stale_candidate_verified", decision.Reason
	case operatorIntentAutoRestart:
		return "auto_restart_candidate", decision.Reason
	case operatorIntentEscalateBlocked:
		return "escalation_blocked", decision.Reason
	case operatorIntentCommitReady:
		return "commit_ready", decision.Reason
	case operatorIntentPRReady:
		return "pr_ready", decision.Reason
	default:
		return "operator_noop", decision.Reason
	}
}

func executeOperatorAction(ctx context.Context, service *orchestrator.Service, runID string, intent operatorIntent) error {
	switch intent {
	case operatorIntentAutoRestart:
		_, err := service.Restart(ctx, orchestrator.RestartOptions{RunID: runID, DryRun: false})
		return err
	case operatorIntentAutoStopStale:
		return service.Stop(ctx, runID)
	case operatorIntentCommitReady:
		_, err := service.Commit(ctx, orchestrator.CommitOptions{RunID: runID, Actor: "operator"})
		return err
	case operatorIntentPRReady:
		_, err := service.OpenPullRequests(ctx, orchestrator.PullRequestOptions{RunID: runID, Actor: "operator"})
		return err
	default:
		return nil
	}
}

func appendOperatorEscalationSummary(ctx context.Context, service *orchestrator.Service, runID string, intent operatorIntent, summary string) error {
	runCtx, err := service.OperatorRunContext(runID)
	if err != nil {
		return err
	}
	if len(runCtx.Tickets) == 0 {
		return nil
	}
	workspaceSet := map[string]struct{}{}
	for _, agent := range runCtx.Agents {
		workspaceName := strings.TrimSpace(agent.WorkspaceName)
		if workspaceName == "" {
			continue
		}
		workspaceSet[workspaceName] = struct{}{}
	}
	if len(workspaceSet) == 0 {
		return nil
	}

	for workspaceName := range workspaceSet {
		workspacePath, err := operatorResolveWorkspacePath(workspaceName)
		if err != nil {
			return err
		}
		docRepoPath, err := operatorResolveDocRepoPath(workspacePath, runCtx.DocHomeRepo, runCtx.Repos)
		if err != nil {
			return err
		}
		for _, ticket := range runCtx.Tickets {
			relativePath, err := operatorResolveTicketRelativePath(ctx, ticket)
			if err != nil {
				return err
			}
			changelogPath := filepath.Join(docRepoPath, "ttmp", relativePath, "changelog.md")
			entry := "\n\n## " + time.Now().Format(time.RFC3339) + "\n\n" +
				"- Operator escalation for run `" + runID + "`\n" +
				"- Intent: `" + string(intent) + "`\n" +
				"- Summary: " + summary + "\n" +
				"- Requested decision: review `metawsm status --run-id " + runID + "` and provide guidance.\n"
			if err := appendTextFile(changelogPath, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func operatorResolveWorkspacePath(workspaceName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "workspace-manager", "workspaces", workspaceName+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read workspace config %s: %w", path, err)
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", fmt.Errorf("parse workspace config %s: %w", path, err)
	}
	if strings.TrimSpace(payload.Path) == "" {
		return "", fmt.Errorf("workspace config %s missing path", path)
	}
	return payload.Path, nil
}

func operatorResolveDocRepoPath(workspacePath string, docHomeRepo string, repos []string) (string, error) {
	docHomeRepo = strings.TrimSpace(docHomeRepo)
	if docHomeRepo == "" {
		if len(repos) > 0 {
			docHomeRepo = strings.TrimSpace(repos[0])
		}
	}
	if docHomeRepo == "" {
		return workspacePath, nil
	}
	candidate := filepath.Join(workspacePath, docHomeRepo)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	if len(repos) == 1 && operatorIsGitRepo(workspacePath) {
		return workspacePath, nil
	}
	return "", fmt.Errorf("doc repo path not found in workspace: %s", candidate)
}

func operatorIsGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func operatorResolveTicketRelativePath(ctx context.Context, ticket string) (string, error) {
	cmd := exec.CommandContext(ctx, "docmgr", "ticket", "list", "--ticket", ticket)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docmgr ticket list --ticket %s failed: %w: %s", ticket, err, strings.TrimSpace(string(out)))
	}
	text := string(out)
	docsRootMatch := operatorDocmgrDocsRootRegex.FindStringSubmatch(text)
	ticketPathMatch := operatorDocmgrTicketPathRegex.FindStringSubmatch(text)
	if len(docsRootMatch) < 2 || len(ticketPathMatch) < 2 {
		return "", fmt.Errorf("unable to parse ticket path from docmgr output")
	}
	docsRoot := filepath.Clean(strings.TrimSpace(docsRootMatch[1]))
	relativePath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ticketPathMatch[1])))
	if relativePath == "." || relativePath == "" || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe ticket relative path parsed from docmgr output: %s", relativePath)
	}
	if _, err := os.Stat(filepath.Join(docsRoot, relativePath)); err != nil {
		return "", fmt.Errorf("ticket path %s: %w", filepath.Join(docsRoot, relativePath), err)
	}
	return relativePath, nil
}

func appendTextFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func resolveOperatorLLMMode(flagValue string, policyValue string) (string, error) {
	mode := strings.TrimSpace(strings.ToLower(flagValue))
	if mode == "" {
		mode = strings.TrimSpace(strings.ToLower(policyValue))
	}
	if mode == "" {
		mode = "assist"
	}
	switch mode {
	case "off", "assist", "auto":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid --llm-mode %q (expected off|assist|auto)", flagValue)
	}
}

func classifyStaleRunCandidate(snapshot watchSnapshot, run model.RunRecord, now time.Time, staleAge time.Duration) (bool, string) {
	if staleAge <= 0 {
		return false, ""
	}
	if snapshot.HasGuidance {
		return false, ""
	}
	if !snapshot.HasUnhealthyAgents {
		return false, ""
	}
	age := now.Sub(run.UpdatedAt)
	if age < staleAge {
		return false, ""
	}
	return true, fmt.Sprintf("run appears stale (updated %s ago, threshold=%s)", age.Truncate(time.Second), staleAge)
}

func verifyStaleRuntimeEvidence(ctx context.Context, snapshot watchSnapshot, now time.Time, recentWindow time.Duration, probe operatorSessionProbe) (bool, string, error) {
	if probe == nil {
		return false, "runtime probe is unavailable", nil
	}
	if recentWindow <= 0 {
		recentWindow = 5 * time.Minute
	}
	if len(snapshot.UnhealthyAgents) == 0 {
		return false, "no unhealthy agents available to verify", nil
	}

	evidenceCount := 0
	for _, issue := range snapshot.UnhealthyAgents {
		session := strings.TrimSpace(issue.Session)
		if session == "" {
			continue
		}
		evidenceCount++
		evidence, err := probe(ctx, session)
		if err != nil {
			return false, "", err
		}
		if evidence.HasSession {
			if evidence.LastActivity != nil && now.Sub(*evidence.LastActivity) <= recentWindow {
				return false, fmt.Sprintf("session %s has recent activity within %s", session, recentWindow), nil
			}
			if evidence.ExitCode == nil || *evidence.ExitCode == 0 {
				return false, fmt.Sprintf("session %s still appears running", session), nil
			}
		}
	}
	if evidenceCount == 0 {
		return false, "no agent sessions available for runtime verification", nil
	}
	return true, "no active tmux sessions or recent activity detected", nil
}

func probeOperatorSessionEvidence(ctx context.Context, session string) (operatorSessionEvidence, error) {
	evidence := operatorSessionEvidence{Session: session}
	if !operatorTmuxHasSession(ctx, session) {
		return evidence, nil
	}
	evidence.HasSession = true
	if epoch := operatorFetchSessionActivity(ctx, session); epoch > 0 {
		t := time.Unix(epoch, 0)
		evidence.LastActivity = &t
	}
	if exitCode, ok := operatorReadAgentExitCode(ctx, session); ok {
		exitCopy := exitCode
		evidence.ExitCode = &exitCopy
	}
	return evidence, nil
}

func operatorTmuxHasSession(ctx context.Context, session string) bool {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux has-session -t %s", cmdShellQuote(session)))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	return cmd.Run() == nil
}

func operatorFetchSessionActivity(ctx context.Context, session string) int64 {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux display-message -p -t %s '#{session_activity}'", cmdShellQuote(session)))
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	value := strings.TrimSpace(string(out))
	epoch, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return epoch
}

func operatorReadAgentExitCode(ctx context.Context, session string) (int, bool) {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux capture-pane -p -t %s:0 | tail -n 200", cmdShellQuote(session)))
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	return operatorParseAgentExitCode(string(out))
}

func operatorParseAgentExitCode(output string) (int, bool) {
	matches := operatorAgentExitRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(last[1]))
	if err != nil {
		return 0, false
	}
	return code, true
}

func resolveWatchMode(runID string, ticket string, all bool) (watchMode, error) {
	hasSelector := strings.TrimSpace(runID) != "" || strings.TrimSpace(ticket) != ""
	if all && hasSelector {
		return watchModeSingleRun, fmt.Errorf("--all cannot be combined with --run-id or --ticket")
	}
	if all || !hasSelector {
		return watchModeAllActiveRuns, nil
	}
	return watchModeSingleRun, nil
}

func statusWithRetry(ctx context.Context, service *orchestrator.Service, runID string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		status, err := service.Status(ctx, runID)
		if err == nil {
			return status, nil
		}
		lastErr = err
		if !strings.Contains(strings.ToLower(err.Error()), "database is locked") {
			return "", err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", lastErr
}

func loadWatchSnapshot(ctx context.Context, service *orchestrator.Service, runID string, ticketFallback string) (watchSnapshot, error) {
	statusText, err := statusWithRetry(ctx, service, runID)
	if err != nil {
		return watchSnapshot{}, err
	}
	snapshot := parseWatchSnapshot(statusText)
	if strings.TrimSpace(snapshot.RunID) == "" {
		snapshot.RunID = runID
	}
	if strings.TrimSpace(snapshot.Tickets) == "" {
		snapshot.Tickets = strings.TrimSpace(ticketFallback)
	}
	return snapshot, nil
}

func loadWatchSnapshotsAll(ctx context.Context, service *orchestrator.Service, trackedRuns map[string]struct{}) ([]watchSnapshot, error) {
	activeRuns, err := service.ActiveRuns()
	if err != nil {
		return nil, err
	}
	activeSet := map[string]struct{}{}
	snapshots := []watchSnapshot{}

	for _, run := range activeRuns {
		runID := strings.TrimSpace(run.RunID)
		if runID == "" {
			continue
		}
		activeSet[runID] = struct{}{}
		trackedRuns[runID] = struct{}{}
		snapshot, err := loadWatchSnapshot(ctx, service, runID, "")
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}

	for runID := range trackedRuns {
		if _, active := activeSet[runID]; active {
			continue
		}
		snapshot, err := loadWatchSnapshot(ctx, service, runID, "")
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				delete(trackedRuns, runID)
				continue
			}
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func parseWatchSnapshot(statusText string) watchSnapshot {
	snapshot := watchSnapshot{}
	inAgents := false
	inGuidance := false
	inDiffs := false
	inPullRequests := false
	scanner := bufio.NewScanner(strings.NewReader(statusText))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Run:"):
			snapshot.RunID = strings.TrimSpace(strings.TrimPrefix(line, "Run:"))
			inAgents = false
			inGuidance = false
			inDiffs = false
			inPullRequests = false
		case strings.HasPrefix(line, "Status:"):
			snapshot.RunStatus = strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
			inAgents = false
			inGuidance = false
			inDiffs = false
			inPullRequests = false
		case strings.HasPrefix(line, "Tickets:"):
			snapshot.Tickets = strings.TrimSpace(strings.TrimPrefix(line, "Tickets:"))
			inAgents = false
			inGuidance = false
			inDiffs = false
			inPullRequests = false
		case strings.HasPrefix(line, "Guidance:"):
			snapshot.HasGuidance = true
			inAgents = false
			inGuidance = true
			inDiffs = false
			inPullRequests = false
		case strings.HasPrefix(line, "Diffs:"):
			inAgents = false
			inGuidance = false
			inDiffs = true
			inPullRequests = false
		case strings.HasPrefix(line, "Pull Requests:"):
			inAgents = false
			inGuidance = false
			inDiffs = false
			inPullRequests = true
		case strings.HasPrefix(line, "Agents:"):
			inAgents = true
			inGuidance = false
			inDiffs = false
			inPullRequests = false
		default:
			if inGuidance {
				if !strings.HasPrefix(line, "  ") {
					inGuidance = false
				} else {
					item := strings.TrimSpace(strings.TrimPrefix(line, "- "))
					if item != "" {
						snapshot.GuidanceItems = append(snapshot.GuidanceItems, item)
					}
					continue
				}
			}
			if inDiffs {
				if !strings.HasPrefix(line, "  ") {
					inDiffs = false
				} else {
					if strings.Contains(strings.TrimSpace(line), " dirty files=") {
						snapshot.HasDirtyDiffs = true
					}
					continue
				}
			}
			if inPullRequests {
				if !strings.HasPrefix(line, "  ") {
					inPullRequests = false
				} else {
					state := strings.TrimSpace(strings.ToLower(parseWatchField(line, "state")))
					switch state {
					case "draft":
						snapshot.DraftPullRequests++
					case "open":
						snapshot.OpenPullRequests++
					}
					continue
				}
			}
			if !inAgents {
				continue
			}
			if !strings.HasPrefix(line, "  ") {
				inAgents = false
				continue
			}
			agentIssue, unhealthy := parseWatchAgentLine(line)
			if unhealthy {
				snapshot.HasUnhealthyAgents = true
				snapshot.UnhealthyAgents = append(snapshot.UnhealthyAgents, agentIssue)
			}
		}
	}
	if strings.EqualFold(snapshot.RunStatus, string(model.RunStatusAwaitingGuidance)) {
		snapshot.HasGuidance = true
	}
	return snapshot
}

func parseWatchField(line string, key string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	prefix := strings.TrimSpace(key) + "="
	for _, field := range fields {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimPrefix(field, prefix)
		}
	}
	return ""
}

func classifyWatchEvent(snapshot watchSnapshot) (event string, message string, terminal bool) {
	status := strings.TrimSpace(snapshot.RunStatus)
	switch status {
	case string(model.RunStatusFailed):
		return "run_failed", "run entered failed state", true
	case string(model.RunStatusStopped):
		return "run_stopped", "run entered stopped state", true
	}
	if snapshot.HasGuidance {
		return "guidance_needed", "operator input is required", true
	}
	switch status {
	case string(model.RunStatusComplete), string(model.RunStatusClosed):
		return "run_done", fmt.Sprintf("run reached %s", status), true
	}
	if snapshot.HasUnhealthyAgents && strings.EqualFold(status, string(model.RunStatusRunning)) {
		return "agent_unhealthy", summarizeUnhealthyAgents(snapshot), false
	}
	return "", "", false
}

func isTerminalRunStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case string(model.RunStatusComplete), string(model.RunStatusClosed), string(model.RunStatusFailed), string(model.RunStatusStopped):
		return true
	default:
		return false
	}
}

func runWatchNotifyCommand(ctx context.Context, notifyCmd string, event string, message string, snapshot watchSnapshot) error {
	notifyCmd = strings.TrimSpace(notifyCmd)
	if notifyCmd == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "zsh", "-lc", notifyCmd)
	cmd.Env = append(os.Environ(),
		"METAWSM_EVENT="+event,
		"METAWSM_MESSAGE="+message,
		"METAWSM_RUN_ID="+snapshot.RunID,
		"METAWSM_RUN_STATUS="+snapshot.RunStatus,
		"METAWSM_TICKETS="+snapshot.Tickets,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isWatchStopError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "signal: interrupt") || strings.Contains(lower, "context canceled")
}

func parseWatchAgentLine(line string) (watchAgentIssue, bool) {
	issue := watchAgentIssue{Agent: strings.TrimSpace(line)}
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	if trimmed == "" {
		return issue, false
	}
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		issue.Agent = fields[0]
	}
	values := map[string]string{}
	for _, field := range fields[1:] {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	issue.Status = values["status"]
	issue.Health = values["health"]
	issue.Session = values["session"]
	issue.LastActivity = values["last_activity"]
	issue.LastProgress = values["last_progress"]
	issue.ActivityAge = values["activity_age"]
	issue.ProgressAge = values["progress_age"]
	issue.Reason = describeUnhealthyReason(issue)
	unhealthy := issue.Health == "dead" || issue.Health == "stalled" || issue.Status == "failed" || issue.Status == "dead"
	return issue, unhealthy
}

func describeUnhealthyReason(issue watchAgentIssue) string {
	switch {
	case issue.Status == "failed":
		return "agent process exited with failure"
	case issue.Health == "dead" || issue.Status == "dead":
		return "agent session is not running"
	case issue.Health == "stalled" || issue.Status == "stalled":
		return fmt.Sprintf("no recent activity/progress (activity_age=%s progress_age=%s)", emptyValue(issue.ActivityAge, "?"), emptyValue(issue.ProgressAge, "?"))
	default:
		return "agent reported unhealthy state"
	}
}

func summarizeUnhealthyAgents(snapshot watchSnapshot) string {
	if len(snapshot.UnhealthyAgents) == 0 {
		return "one or more agents are stalled/dead/failed"
	}
	first := snapshot.UnhealthyAgents[0]
	if len(snapshot.UnhealthyAgents) == 1 {
		return fmt.Sprintf("%s: %s", first.Agent, first.Reason)
	}
	return fmt.Sprintf("%s: %s (+%d more unhealthy agent(s))", first.Agent, first.Reason, len(snapshot.UnhealthyAgents)-1)
}

func buildWatchDirectionHints(snapshot watchSnapshot, event string) []string {
	hints := []string{
		fmt.Sprintf("Direction needed for run %s.", emptyValue(snapshot.RunID, "unknown")),
	}
	switch event {
	case "guidance_needed":
		if len(snapshot.GuidanceItems) > 0 {
			hints = append(hints, fmt.Sprintf("Pending question: %s", snapshot.GuidanceItems[0]))
		}
		hints = append(hints, fmt.Sprintf("Answer with: metawsm guide --run-id %s --answer \"<decision>\"", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Or inspect first: metawsm status --run-id %s", snapshot.RunID))
	case "agent_unhealthy":
		if len(snapshot.UnhealthyAgents) > 0 {
			issue := snapshot.UnhealthyAgents[0]
			hints = append(hints, fmt.Sprintf("Likely cause: %s (%s)", issue.Reason, issue.Agent))
		}
		hints = append(hints, fmt.Sprintf("If you want to re-run the agent: metawsm restart --run-id %s", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("If this needs operator guidance: metawsm guide --run-id %s --answer \"<direction>\"", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Inspect details: metawsm status --run-id %s", snapshot.RunID))
	case "run_failed":
		hints = append(hints, fmt.Sprintf("Inspect failure context: metawsm status --run-id %s", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Then decide to restart or stop: metawsm restart --run-id %s", snapshot.RunID))
	case "run_stopped":
		hints = append(hints, fmt.Sprintf("Resume if desired: metawsm resume --run-id %s", snapshot.RunID))
	case "run_done":
		hints = append(hints, fmt.Sprintf("Review and merge: metawsm merge --run-id %s --dry-run", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Close when ready: metawsm close --run-id %s", snapshot.RunID))
	case "commit_ready":
		hints = append(hints, fmt.Sprintf("Preview commit actions: metawsm commit --run-id %s --dry-run", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Create commits: metawsm commit --run-id %s", snapshot.RunID))
	case "pr_ready":
		hints = append(hints, fmt.Sprintf("Preview PR actions: metawsm pr --run-id %s --dry-run", snapshot.RunID))
		hints = append(hints, fmt.Sprintf("Create pull requests: metawsm pr --run-id %s", snapshot.RunID))
	}
	return hints
}

func resumeCommand(args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (resume latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return err
	}
	if err := service.Resume(context.Background(), runID); err != nil {
		return err
	}
	fmt.Printf("Run %s resumed.\n", runID)
	return nil
}

func stopCommand(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (stop latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return err
	}
	if err := service.Stop(context.Background(), runID); err != nil {
		return err
	}
	fmt.Printf("Run %s stopped.\n", runID)
	return nil
}

func restartCommand(args []string) error {
	fs := flag.NewFlagSet("restart", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (restart latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview restart actions without executing them")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Restart(context.Background(), orchestrator.RestartOptions{
		RunID:  runID,
		Ticket: ticket,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Restart dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Run %s restarted.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

func cleanupCommand(args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var dryRun bool
	var keepWorkspaces bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (cleanup latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview cleanup actions without executing them")
	fs.BoolVar(&keepWorkspaces, "keep-workspaces", false, "Keep workspaces; only stop agent sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Cleanup(context.Background(), orchestrator.CleanupOptions{
		RunID:            runID,
		Ticket:           ticket,
		DryRun:           dryRun,
		DeleteWorkspaces: !keepWorkspaces,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Cleanup dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Run %s cleaned up.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

func mergeCommand(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (merge latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview merge actions without executing them")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Merge(context.Background(), orchestrator.MergeOptions{
		RunID:  runID,
		Ticket: ticket,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Merge dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Merge completed for run %s.\n", result.RunID)
	}
	if len(result.Actions) == 0 {
		fmt.Println("  - no dirty workspace changes to merge")
		return nil
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

func commitCommand(args []string) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var message string
	var actor string
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (commit latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&message, "message", "", "Explicit commit message (defaults to ticket + run brief goal)")
	fs.StringVar(&actor, "actor", "", "Actor identity to persist with commit metadata")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview commit actions without executing them")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Commit(context.Background(), orchestrator.CommitOptions{
		RunID:   runID,
		Ticket:  ticket,
		Message: message,
		Actor:   actor,
		DryRun:  dryRun,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Commit dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Commit completed for run %s.\n", result.RunID)
	}
	if len(result.Repos) == 0 {
		fmt.Println("  - no repository commit targets found")
		return nil
	}
	for _, repo := range result.Repos {
		fmt.Printf("  - %s/%s workspace=%s dirty=%t\n", repo.Ticket, repo.Repo, repo.WorkspaceName, repo.Dirty)
		if strings.TrimSpace(repo.SkippedReason) != "" {
			fmt.Printf("    skipped=%s\n", repo.SkippedReason)
			continue
		}
		fmt.Printf("    branch=%s base=%s (%s)\n", repo.Branch, repo.BaseBranch, repo.BaseRef)
		fmt.Printf("    message=%s\n", repo.CommitMessage)
		if dryRun {
			for _, action := range repo.Actions {
				fmt.Printf("    dry-run: %s\n", action)
			}
			continue
		}
		fmt.Printf("    commit=%s\n", emptyValue(repo.CommitSHA, "-"))
	}
	return nil
}

func prCommand(args []string) error {
	fs := flag.NewFlagSet("pr", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var title string
	var body string
	var actor string
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (open PRs for the latest run on this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&title, "title", "", "Explicit pull request title")
	fs.StringVar(&body, "body", "", "Explicit pull request body")
	fs.StringVar(&actor, "actor", "", "Actor identity to persist with pull request metadata")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview pull request actions without executing them")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.OpenPullRequests(context.Background(), orchestrator.PullRequestOptions{
		RunID:  runID,
		Ticket: ticket,
		Title:  title,
		Body:   body,
		Actor:  actor,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("PR dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("PR creation completed for run %s.\n", result.RunID)
	}
	if len(result.Repos) == 0 {
		fmt.Println("  - no pull request targets found")
		return nil
	}
	for _, repo := range result.Repos {
		fmt.Printf("  - %s/%s workspace=%s\n", repo.Ticket, repo.Repo, repo.WorkspaceName)
		if strings.TrimSpace(repo.SkippedReason) != "" {
			fmt.Printf("    skipped=%s\n", repo.SkippedReason)
			continue
		}
		fmt.Printf("    head=%s base=%s\n", repo.HeadBranch, repo.BaseBranch)
		fmt.Printf("    title=%s\n", repo.Title)
		if dryRun {
			for _, action := range repo.Actions {
				fmt.Printf("    dry-run: %s\n", action)
			}
			continue
		}
		fmt.Printf("    pr=%s number=%d state=%s\n", emptyValue(repo.PRURL, "-"), repo.PRNumber, emptyValue(string(repo.PRState), "-"))
	}
	return nil
}

func iterateCommand(args []string) error {
	fs := flag.NewFlagSet("iterate", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var feedback string
	var dryRun bool
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (iterate latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&feedback, "feedback", "", "Operator feedback to guide the next implementation iteration")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview iterate actions without executing them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(feedback) == "" {
		return fmt.Errorf("--feedback is required")
	}

	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	result, err := service.Iterate(context.Background(), orchestrator.IterateOptions{
		RunID:    runID,
		Ticket:   ticket,
		Feedback: feedback,
		DryRun:   dryRun,
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Iterate dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Iterate started for run %s.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

func closeCommand(args []string) error {
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var dryRun bool
	var changelogEntry string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&ticket, "ticket", "", "Ticket identifier (close latest run for this ticket)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview close actions")
	fs.StringVar(&changelogEntry, "changelog-entry", "", "Changelog entry for docmgr ticket close")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID, ticket, err := requireRunSelector(runID, ticket)
	if err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return err
	}
	if err := service.Close(context.Background(), orchestrator.CloseOptions{
		RunID:          runID,
		DryRun:         dryRun,
		ChangelogEntry: changelogEntry,
	}); err != nil {
		return err
	}
	if dryRun {
		fmt.Printf("Close dry-run complete for run %s.\n", runID)
	} else {
		fmt.Printf("Run %s closed.\n", runID)
	}
	return nil
}

func policyInitCommand(args []string) error {
	fs := flag.NewFlagSet("policy-init", flag.ContinueOnError)
	var path string
	fs.StringVar(&path, "path", policy.DefaultPolicyPath, "Path to policy file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := policy.SaveDefault(path); err != nil {
		return err
	}
	fmt.Printf("Wrote default policy to %s\n", path)
	return nil
}

func tuiCommand(args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	var runID string
	var ticket string
	var dbPath string
	var intervalSeconds int
	fs.StringVar(&runID, "run-id", "", "Specific run to monitor (optional)")
	fs.StringVar(&ticket, "ticket", "", "Specific ticket to monitor (optional; resolves latest run)")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.IntVar(&intervalSeconds, "interval", 2, "Refresh interval in seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if intervalSeconds <= 0 {
		return fmt.Errorf("--interval must be > 0")
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(runID) != "" || strings.TrimSpace(ticket) != "" {
		runID, ticket, err = requireRunSelector(runID, ticket)
		if err != nil {
			return err
		}
		runID, err = service.ResolveRunID(runID, ticket)
		if err != nil {
			return err
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		printTUIFrameHeader(runID, intervalSeconds)
		if strings.TrimSpace(runID) != "" {
			status, err := service.Status(ctx, runID)
			if err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println(status)
			}
		} else {
			runs, err := service.ActiveRuns()
			if err != nil {
				fmt.Printf("error: %v\n", err)
			} else if len(runs) == 0 {
				fmt.Println("No active runs.")
				fmt.Println("Tip: start one with `metawsm run ...`.")
			} else {
				for i, run := range runs {
					if i > 0 {
						fmt.Println(strings.Repeat("-", 72))
					}
					status, err := service.Status(ctx, run.RunID)
					if err != nil {
						fmt.Printf("run=%s error=%v\n", run.RunID, err)
						continue
					}
					fmt.Println(status)
				}
			}
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nTUI monitor stopped.")
			return nil
		case <-ticker.C:
		}
	}
}

func docsCommand(args []string) error {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	var dbPath string
	var policyPath string
	var refresh bool
	var ticket string
	var endpointNames multiValueFlag
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&policyPath, "policy", "", "Path to policy file (defaults to .metawsm/policy.json)")
	fs.BoolVar(&refresh, "refresh", false, "Call /api/v1/index/refresh before aggregation")
	fs.StringVar(&ticket, "ticket", "", "Optional ticket filter")
	fs.Var(&endpointNames, "endpoint", "Endpoint names for --refresh selection (repeatable, or comma-separated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return err
	}
	cfg, _, err := policy.Load(policyPath)
	if err != nil {
		return err
	}
	endpoints := federationEndpointsFromPolicy(cfg)
	if len(endpoints) == 0 {
		return fmt.Errorf("no docs.api endpoints configured in policy")
	}

	timeout := time.Duration(cfg.Docs.API.RequestTimeoutSec) * time.Second
	client := docfederation.NewClient(timeout)
	ctx := context.Background()

	if refresh {
		selected := selectFederationEndpoints(endpoints, normalizeInputTokens(endpointNames))
		refreshResults := client.RefreshIndexes(ctx, selected)
		fmt.Println("Refresh:")
		for _, result := range refreshResults {
			if result.Err != nil {
				fmt.Printf("  - %s (%s) error=%v\n", result.Endpoint.Name, result.Endpoint.Kind, result.Err)
				continue
			}
			fmt.Printf("  - %s (%s) refreshed=%t indexed_at=%s docs=%d\n",
				result.Endpoint.Name, result.Endpoint.Kind, result.Refreshed, result.IndexedAt, result.DocsCount)
		}
		fmt.Println("")
	}

	snapshots := client.CollectSnapshots(ctx, endpoints)
	activeContexts, err := service.ActiveDocContexts()
	if err != nil {
		return err
	}
	contexts := make([]docfederation.ActiveContext, 0, len(activeContexts))
	for _, item := range activeContexts {
		contexts = append(contexts, docfederation.ActiveContext{
			Ticket:      item.Ticket,
			DocHomeRepo: item.DocHomeRepo,
		})
	}
	merged := docfederation.MergeWorkspaceFirst(snapshots, contexts)
	ticketFilter := strings.TrimSpace(ticket)

	fmt.Println("Doc Federation")
	fmt.Println("Endpoints:")
	for _, health := range merged.Health {
		if health.Reachable {
			fmt.Printf("  - %s kind=%s repo=%s workspace=%s indexed_at=%s web=%s\n",
				health.Endpoint.Name,
				health.Endpoint.Kind,
				health.Endpoint.Repo,
				emptyValue(health.Endpoint.Workspace, "-"),
				emptyValue(health.IndexedAt, "unknown"),
				emptyValue(webURLOrEndpointBase(health.Endpoint), health.Endpoint.BaseURL),
			)
		} else {
			fmt.Printf("  - %s kind=%s repo=%s workspace=%s error=%s\n",
				health.Endpoint.Name,
				health.Endpoint.Kind,
				health.Endpoint.Repo,
				emptyValue(health.Endpoint.Workspace, "-"),
				health.ErrorText,
			)
		}
	}

	fmt.Println("Tickets:")
	seen := 0
	for _, item := range merged.Tickets {
		if ticketFilter != "" && !strings.EqualFold(ticketFilter, item.Ticket) {
			continue
		}
		seen++
		fmt.Printf("  - %s status=%s home_repo=%s active=%t source=%s/%s workspace=%s link=%s\n",
			item.Ticket,
			emptyValue(item.Status, "unknown"),
			emptyValue(item.DocHomeRepo, "unknown"),
			item.Active,
			item.SourceKind,
			item.SourceName,
			emptyValue(item.SourceWS, "-"),
			emptyValue(item.SourceWebURL, item.SourceURL),
		)
	}
	if seen == 0 {
		fmt.Println("  - none")
	}
	return nil
}

func printTUIFrameHeader(runID string, intervalSeconds int) {
	fmt.Print("\033[H\033[2J")
	fmt.Printf("metawsm tui monitor  now=%s  interval=%ds\n", time.Now().Format(time.RFC3339), intervalSeconds)
	if strings.TrimSpace(runID) != "" {
		fmt.Printf("scope: run=%s\n", runID)
	} else {
		fmt.Println("scope: active runs")
	}
	fmt.Println(strings.Repeat("=", 72))
}

func normalizeInputTokens(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			out = append(out, token)
		}
	}
	return out
}

func requireRunSelector(runID string, ticket string) (string, string, error) {
	runID = strings.TrimSpace(runID)
	ticket = strings.TrimSpace(ticket)
	if runID == "" && ticket == "" {
		return "", "", fmt.Errorf("one of --run-id or --ticket is required")
	}
	return runID, ticket, nil
}

func isInteractiveStdin() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func collectBootstrapBrief(in io.Reader, out io.Writer, interactive bool, ticket string, seed model.RunBrief) (model.RunBrief, error) {
	brief := seed
	brief.Ticket = ticket
	if strings.TrimSpace(brief.MergeIntent) == "" {
		brief.MergeIntent = "default"
	}
	qa := []model.IntakeQA{}
	reader := bufio.NewReader(in)

	type prompt struct {
		label    string
		question string
		field    *string
	}
	prompts := []prompt{
		{label: "Goal", question: fmt.Sprintf("Ticket %s goal: what should be built/changed?", ticket), field: &brief.Goal},
		{label: "Scope", question: "Scope: which areas/files are in scope?", field: &brief.Scope},
		{label: "Done", question: "Done criteria: which tests/checks define complete?", field: &brief.DoneCriteria},
		{label: "Constraints", question: "Constraints/non-goals/risk boundaries?", field: &brief.Constraints},
		{label: "Merge", question: "Merge intent? (type 'default' for normal close flow)", field: &brief.MergeIntent},
	}
	for _, item := range prompts {
		value := strings.TrimSpace(*item.field)
		if value != "" {
			qa = append(qa, model.IntakeQA{Question: item.question, Answer: value})
			continue
		}
		if !interactive {
			return model.RunBrief{}, fmt.Errorf("missing required bootstrap intake field %q; provide all fields via flags in non-interactive mode", item.label)
		}
		for {
			fmt.Fprintf(out, "%s\n> ", item.question)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return model.RunBrief{}, err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				fmt.Fprintln(out, "Please provide a non-empty answer.")
				if err == io.EOF {
					return model.RunBrief{}, fmt.Errorf("incomplete intake")
				}
				continue
			}
			*item.field = line
			qa = append(qa, model.IntakeQA{Question: item.question, Answer: line})
			break
		}
	}
	brief.QA = qa
	now := time.Now()
	brief.CreatedAt = now
	brief.UpdatedAt = now
	return brief, nil
}

func ensureTicketExists(ctx context.Context, ticket string, goal string) error {
	exists, err := docmgrTicketExists(ctx, ticket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	title := titleFromGoal(goal)
	return runDocmgrCommand(ctx, "ticket", "create-ticket", "--ticket", ticket, "--title", title, "--topics", "core,cli")
}

func titleFromGoal(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "Bootstrap work"
	}
	goal = strings.TrimSuffix(goal, ".")
	runes := []rune(goal)
	if len(runes) > 72 {
		goal = string(runes[:72])
	}
	return goal
}

func runDocmgrCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docmgr", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runDocmgrCommandQuiet(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docmgr", args...)
	return cmd.Run()
}

func docmgrTicketExists(ctx context.Context, ticket string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docmgr", "list", "tickets", "--with-glaze-output", "--output", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("list tickets: %w: %s", err, strings.TrimSpace(string(out)))
	}
	payload, ok := extractJSONArray(out)
	if !ok {
		return false, fmt.Errorf("unable to parse ticket list output")
	}
	var rows []map[string]any
	if err := json.Unmarshal(payload, &rows); err != nil {
		return false, fmt.Errorf("parse ticket list json: %w", err)
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(row["ticket"])), strings.TrimSpace(ticket)) {
			return true, nil
		}
	}
	return false, nil
}

func extractJSONArray(out []byte) ([]byte, bool) {
	text := string(out)
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end < 0 || end <= start {
		return nil, false
	}
	return []byte(text[start : end+1]), true
}

var docPathRegex = regexp.MustCompile("Path:\\s+`([^`]+)`")

func createBootstrapBriefDoc(ctx context.Context, ticket string, runID string, brief model.RunBrief) error {
	cmd := exec.CommandContext(ctx, "docmgr", "doc", "add", "--ticket", ticket, "--doc-type", "reference", "--title", fmt.Sprintf("Bootstrap brief %s", runID))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create bootstrap brief doc: %w: %s", err, strings.TrimSpace(string(output)))
	}
	match := docPathRegex.FindStringSubmatch(string(output))
	if len(match) < 2 {
		return fmt.Errorf("unable to find created doc path in docmgr output")
	}
	docPath := filepath.Join("ttmp", filepath.FromSlash(match[1]))
	content, err := os.ReadFile(docPath)
	if err != nil {
		return err
	}
	frontmatter, err := splitFrontmatter(content)
	if err != nil {
		return err
	}

	var body strings.Builder
	body.WriteString("# Bootstrap Brief\n\n")
	body.WriteString(fmt.Sprintf("Run ID: `%s`\n\n", runID))
	body.WriteString("## Goal\n\n")
	body.WriteString(brief.Goal + "\n\n")
	body.WriteString("## Scope\n\n")
	body.WriteString(brief.Scope + "\n\n")
	body.WriteString("## Done Criteria\n\n")
	body.WriteString(brief.DoneCriteria + "\n\n")
	body.WriteString("## Constraints\n\n")
	body.WriteString(brief.Constraints + "\n\n")
	body.WriteString("## Merge Intent\n\n")
	body.WriteString(brief.MergeIntent + "\n\n")
	body.WriteString("## Intake Q/A\n\n")
	for i, qa := range brief.QA {
		body.WriteString(fmt.Sprintf("%d. **Q:** %s\n", i+1, qa.Question))
		body.WriteString(fmt.Sprintf("   **A:** %s\n", qa.Answer))
	}
	body.WriteString("\n")

	final := append([]byte(frontmatter), []byte(body.String())...)
	return os.WriteFile(docPath, final, 0o644)
}

func splitFrontmatter(content []byte) (string, error) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("document missing frontmatter")
	}
	rest := text[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", fmt.Errorf("document frontmatter terminator not found")
	}
	return text[:len("---\n")+idx+len("\n---\n")] + "\n", nil
}

func cmdShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func printUsage() {
	fmt.Println("metawsm - orchestrate multi-ticket multi-workspace agent runs")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  metawsm run --ticket T1 --ticket T2 --repos repo1,repo2 [--doc-home-repo repo1] [--doc-authority-mode workspace_active] [--doc-seed-mode copy_from_repo_on_start] [--agent planner --agent coder] [--base-branch main]")
	fmt.Println("  metawsm bootstrap --ticket T1 --repos repo1,repo2 [--doc-home-repo repo1] [--doc-authority-mode workspace_active] [--doc-seed-mode copy_from_repo_on_start] [--agent planner] [--base-branch main]")
	fmt.Println("  metawsm status [--run-id RUN_ID | --ticket T1]")
	fmt.Println("  metawsm auth check [--run-id RUN_ID | --ticket T1] [--policy PATH]")
	fmt.Println("  metawsm watch [--run-id RUN_ID | --ticket T1 | --all] [--interval 15] [--notify-cmd \"...\"] [--bell=true]")
	fmt.Println("  metawsm operator [--run-id RUN_ID | --ticket T1 | --all] [--interval 15] [--llm-mode off|assist|auto] [--dry-run]")
	fmt.Println("  metawsm guide [--run-id RUN_ID | --ticket T1] --answer \"...\"")
	fmt.Println("  metawsm resume [--run-id RUN_ID | --ticket T1]")
	fmt.Println("  metawsm stop [--run-id RUN_ID | --ticket T1]")
	fmt.Println("  metawsm restart [--run-id RUN_ID | --ticket T1] [--dry-run]")
	fmt.Println("  metawsm cleanup [--run-id RUN_ID | --ticket T1] [--keep-workspaces] [--dry-run]")
	fmt.Println("  metawsm commit [--run-id RUN_ID | --ticket T1] [--message \"...\"] [--actor USER] [--dry-run]")
	fmt.Println("  metawsm pr [--run-id RUN_ID | --ticket T1] [--title \"...\"] [--body \"...\"] [--actor USER] [--dry-run]")
	fmt.Println("  metawsm merge [--run-id RUN_ID | --ticket T1] [--dry-run]")
	fmt.Println("  metawsm iterate [--run-id RUN_ID | --ticket T1] --feedback \"...\" [--dry-run]")
	fmt.Println("  metawsm close [--run-id RUN_ID | --ticket T1] [--dry-run]")
	fmt.Println("  metawsm policy-init")
	fmt.Println("  metawsm tui [--run-id RUN_ID | --ticket T1] [--interval 2]")
	fmt.Println("  metawsm docs [--policy PATH] [--refresh] [--endpoint NAME] [--ticket T1]")
}

func federationEndpointsFromPolicy(cfg policy.Config) []docfederation.Endpoint {
	endpoints := []docfederation.Endpoint{}
	for _, endpoint := range cfg.Docs.API.WorkspaceEndpoints {
		endpoints = append(endpoints, docfederation.Endpoint{
			Name:      strings.TrimSpace(endpoint.Name),
			Kind:      docfederation.EndpointKindWorkspace,
			BaseURL:   strings.TrimSpace(endpoint.BaseURL),
			WebURL:    strings.TrimSpace(endpoint.WebURL),
			Repo:      strings.TrimSpace(endpoint.Repo),
			Workspace: strings.TrimSpace(endpoint.Workspace),
		})
	}
	for _, endpoint := range cfg.Docs.API.RepoEndpoints {
		endpoints = append(endpoints, docfederation.Endpoint{
			Name:      strings.TrimSpace(endpoint.Name),
			Kind:      docfederation.EndpointKindRepo,
			BaseURL:   strings.TrimSpace(endpoint.BaseURL),
			WebURL:    strings.TrimSpace(endpoint.WebURL),
			Repo:      strings.TrimSpace(endpoint.Repo),
			Workspace: strings.TrimSpace(endpoint.Workspace),
		})
	}
	return endpoints
}

func selectFederationEndpoints(endpoints []docfederation.Endpoint, names []string) []docfederation.Endpoint {
	if len(names) == 0 {
		return endpoints
	}
	nameSet := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}
	selected := []docfederation.Endpoint{}
	for _, endpoint := range endpoints {
		if _, ok := nameSet[endpoint.Name]; ok {
			selected = append(selected, endpoint)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Name < selected[j].Name
	})
	return selected
}

func webURLOrEndpointBase(endpoint docfederation.Endpoint) string {
	if strings.TrimSpace(endpoint.WebURL) != "" {
		return endpoint.WebURL
	}
	return endpoint.BaseURL
}

func emptyValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
