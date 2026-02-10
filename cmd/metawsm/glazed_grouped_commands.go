package main

import (
	"context"
	"fmt"
	"strings"

	"metawsm/internal/orchestrator"
	"metawsm/internal/policy"

	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/layers"
	"github.com/go-go-golems/glazed/pkg/cmds/parameters"
	"github.com/spf13/cobra"
)

type authCheckGlazedCommand struct {
	*cmds.CommandDescription
}

type authCheckSettings struct {
	PolicyPath string `glazed.parameter:"policy"`
}

func newAuthCheckGlazedCommand() (*authCheckGlazedCommand, error) {
	runSelectorLayer, err := newRunSelectorLayer()
	if err != nil {
		return nil, err
	}
	desc := cmds.NewCommandDescription(
		"check",
		cmds.WithShort("Check local auth readiness for push/PR"),
		cmds.WithLong("Validate GitHub CLI auth and optional run workspace repo credentials."),
		cmds.WithLayersList(runSelectorLayer),
		cmds.WithFlags(
			parameters.NewParameterDefinition(
				"policy",
				parameters.ParameterTypeString,
				parameters.WithHelp("Path to policy file (defaults to .metawsm/policy.json)"),
				parameters.WithDefault(""),
			),
		),
	)
	return &authCheckGlazedCommand{CommandDescription: desc}, nil
}

func (c *authCheckGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	settings := &authCheckSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}

	cfg, _, err := policy.Load(settings.PolicyPath)
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

	ghInstalled, ghAuthed, ghActor, ghDetail := checkGitHubLocalAuth(ctx)
	effectiveRunID := ""
	repoChecks := []authRepoCheck{}
	if strings.TrimSpace(selector.RunID) != "" || strings.TrimSpace(selector.Ticket) != "" {
		runID, ticket, err := requireRunSelector(selector.RunID, selector.Ticket)
		if err != nil {
			return err
		}
		service, err := orchestrator.NewService(selector.DBPath)
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

var _ cmds.BareCommand = &authCheckGlazedCommand{}

type reviewSyncGlazedCommand struct {
	*cmds.CommandDescription
}

type reviewSyncSettings struct {
	MaxItems int  `glazed.parameter:"max-items"`
	Dispatch bool `glazed.parameter:"dispatch"`
	DryRun   bool `glazed.parameter:"dry-run"`
}

func newReviewSyncGlazedCommand() (*reviewSyncGlazedCommand, error) {
	runSelectorLayer, err := newRunSelectorLayer()
	if err != nil {
		return nil, err
	}
	desc := cmds.NewCommandDescription(
		"sync",
		cmds.WithShort("Sync review feedback for a run"),
		cmds.WithLong("Sync review feedback and optionally dispatch queued feedback via iterate."),
		cmds.WithLayersList(runSelectorLayer),
		cmds.WithFlags(
			parameters.NewParameterDefinition(
				"max-items",
				parameters.ParameterTypeInteger,
				parameters.WithHelp("Optional cap for number of review comments to sync/dispatch"),
				parameters.WithDefault(0),
			),
			parameters.NewParameterDefinition(
				"dispatch",
				parameters.ParameterTypeBool,
				parameters.WithHelp("Dispatch queued feedback via iterate flow after sync"),
				parameters.WithDefault(false),
			),
			parameters.NewParameterDefinition(
				"dry-run",
				parameters.ParameterTypeBool,
				parameters.WithHelp("Preview review sync/dispatch actions without persisting changes"),
				parameters.WithDefault(false),
			),
		),
	)
	return &reviewSyncGlazedCommand{CommandDescription: desc}, nil
}

func (c *reviewSyncGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	settings := &reviewSyncSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}

	runID, ticket, err := requireRunSelector(selector.RunID, selector.Ticket)
	if err != nil {
		return err
	}
	service, err := orchestrator.NewService(selector.DBPath)
	if err != nil {
		return err
	}
	result, err := service.SyncReviewFeedback(ctx, orchestrator.ReviewFeedbackSyncOptions{
		RunID:    runID,
		Ticket:   ticket,
		MaxItems: settings.MaxItems,
		DryRun:   settings.DryRun,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Review sync for run %s:\n", result.RunID)
	for _, repo := range result.Repos {
		fmt.Printf("  - %s/%s pr=%d fetched=%d added=%d updated=%d\n",
			repo.Ticket, repo.Repo, repo.PRNumber, repo.Fetched, repo.Added, repo.Updated)
		if strings.TrimSpace(repo.SkippedReason) != "" {
			fmt.Printf("    skipped=%s\n", repo.SkippedReason)
		}
		for _, action := range repo.Actions {
			if settings.DryRun {
				fmt.Printf("    dry-run: %s\n", action)
			} else {
				fmt.Printf("    action: %s\n", action)
			}
		}
	}
	fmt.Printf("Totals: added=%d updated=%d\n", result.Added, result.Updated)

	if !settings.Dispatch {
		return nil
	}

	dispatchResult, err := service.DispatchQueuedReviewFeedback(ctx, orchestrator.ReviewFeedbackDispatchOptions{
		RunID:    runID,
		Ticket:   ticket,
		MaxItems: settings.MaxItems,
		DryRun:   settings.DryRun,
	})
	if err != nil {
		return err
	}
	if settings.DryRun {
		fmt.Printf("Review dispatch dry-run for run %s queued=%d:\n", dispatchResult.RunID, dispatchResult.QueuedCount)
	} else {
		fmt.Printf("Review dispatch started for run %s queued=%d:\n", dispatchResult.RunID, dispatchResult.QueuedCount)
	}
	for _, action := range dispatchResult.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

var _ cmds.BareCommand = &reviewSyncGlazedCommand{}

func addGroupedCommandTrees(rootCmd *cobra.Command) error {
	authRoot := &cobra.Command{
		Use:   "auth",
		Short: "Auth subcommands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return authCommand(args)
		},
	}
	authCheckCmd, err := newAuthCheckGlazedCommand()
	if err != nil {
		return err
	}
	authCheckCobraCmd, err := buildGlazedCobraCommand(authCheckCmd)
	if err != nil {
		return err
	}
	authRoot.AddCommand(authCheckCobraCmd)
	rootCmd.AddCommand(authRoot)

	reviewRoot := &cobra.Command{
		Use:   "review",
		Short: "Review subcommands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reviewCommand(args)
		},
	}
	reviewSyncCmd, err := newReviewSyncGlazedCommand()
	if err != nil {
		return err
	}
	reviewSyncCobraCmd, err := buildGlazedCobraCommand(reviewSyncCmd)
	if err != nil {
		return err
	}
	reviewRoot.AddCommand(reviewSyncCobraCmd)
	rootCmd.AddCommand(reviewRoot)

	forumRoot := &cobra.Command{
		Use:   "forum",
		Short: "Forum subcommands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return forumCommand(args)
		},
	}

	forumSubcommands := []struct {
		name  string
		short string
	}{
		{name: "ask", short: "Create a forum question"},
		{name: "answer", short: "Add an answer to a thread"},
		{name: "assign", short: "Assign a thread"},
		{name: "state", short: "Update thread state"},
		{name: "priority", short: "Update thread priority"},
		{name: "close", short: "Close a thread"},
		{name: "list", short: "List threads"},
		{name: "thread", short: "Show thread details"},
		{name: "watch", short: "Watch thread activity"},
		{name: "signal", short: "Signal run status to forum"},
		{name: "debug", short: "Forum debug helpers"},
	}
	for _, sub := range forumSubcommands {
		subName := sub.name
		forumRoot.AddCommand(&cobra.Command{
			Use:                subName,
			Short:              sub.short,
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return forumCommand(append([]string{subName}, args...))
			},
		})
	}
	rootCmd.AddCommand(forumRoot)

	return nil
}
