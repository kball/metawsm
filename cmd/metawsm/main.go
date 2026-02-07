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
	"strings"
	"syscall"
	"time"

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
	case "close":
		err = closeCommand(args)
	case "policy-init":
		err = policyInitCommand(args)
	case "tui":
		err = tuiCommand(args)
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
	var baseBranch string
	var policyPath string
	var dbPath string
	var dryRun bool

	fs.Var(&tickets, "ticket", "Ticket identifier (repeatable, or comma-separated)")
	fs.Var(&repos, "repos", "Repositories list (repeatable, or comma-separated)")
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
	var dbPath string
	var answer string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.StringVar(&answer, "answer", "", "Guidance answer for the pending question")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("--run-id is required")
	}
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("--answer is required")
	}

	service, err := orchestrator.NewService(dbPath)
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
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("--run-id is required")
	}

	service, err := orchestrator.NewService(dbPath)
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

func resumeCommand(args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	var runID string
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("--run-id is required")
	}

	service, err := orchestrator.NewService(dbPath)
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
	var dbPath string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("--run-id is required")
	}

	service, err := orchestrator.NewService(dbPath)
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

func closeCommand(args []string) error {
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	var runID string
	var dbPath string
	var dryRun bool
	var changelogEntry string
	fs.StringVar(&runID, "run-id", "", "Run identifier")
	fs.StringVar(&dbPath, "db", ".metawsm/metawsm.db", "Path to SQLite DB")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview close actions")
	fs.StringVar(&changelogEntry, "changelog-entry", "", "Changelog entry for docmgr ticket close")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("--run-id is required")
	}

	service, err := orchestrator.NewService(dbPath)
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
	var dbPath string
	var intervalSeconds int
	fs.StringVar(&runID, "run-id", "", "Specific run to monitor (optional)")
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

func printUsage() {
	fmt.Println("metawsm - orchestrate multi-ticket multi-workspace agent runs")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  metawsm run --ticket T1 --ticket T2 --repos repo1,repo2 [--agent planner --agent coder] [--base-branch main]")
	fmt.Println("  metawsm bootstrap --ticket T1 --repos repo1,repo2 [--agent planner] [--base-branch main]")
	fmt.Println("  metawsm status --run-id RUN_ID")
	fmt.Println("  metawsm guide --run-id RUN_ID --answer \"...\"")
	fmt.Println("  metawsm resume --run-id RUN_ID")
	fmt.Println("  metawsm stop --run-id RUN_ID")
	fmt.Println("  metawsm restart [--run-id RUN_ID | --ticket T1] [--dry-run]")
	fmt.Println("  metawsm cleanup [--run-id RUN_ID | --ticket T1] [--keep-workspaces] [--dry-run]")
	fmt.Println("  metawsm close --run-id RUN_ID [--dry-run]")
	fmt.Println("  metawsm policy-init")
	fmt.Println("  metawsm tui [--run-id RUN_ID] [--interval 2]")
}
