package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"metawsm/internal/orchestrator"

	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/layers"
	"github.com/go-go-golems/glazed/pkg/cmds/parameters"
)

const runSelectorLayerSlug = "run-selector"

type runSelectorSettings struct {
	RunID  string `glazed.parameter:"run-id"`
	Ticket string `glazed.parameter:"ticket"`
	DBPath string `glazed.parameter:"db"`
}

func newRunSelectorLayer() (layers.ParameterLayer, error) {
	layer, err := layers.NewParameterLayer(runSelectorLayerSlug, "Run selector")
	if err != nil {
		return nil, err
	}
	layer.AddFlags(
		parameters.NewParameterDefinition(
			"run-id",
			parameters.ParameterTypeString,
			parameters.WithHelp("Run identifier"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"ticket",
			parameters.ParameterTypeString,
			parameters.WithHelp("Ticket identifier (use latest run for this ticket)"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"db",
			parameters.ParameterTypeString,
			parameters.WithHelp("Path to SQLite DB"),
			parameters.WithDefault(".metawsm/metawsm.db"),
		),
	)
	return layer, nil
}

func newRunSelectorCommandDescription(name string, short string, long string, flags ...*parameters.ParameterDefinition) (*cmds.CommandDescription, error) {
	runSelectorLayer, err := newRunSelectorLayer()
	if err != nil {
		return nil, err
	}
	options := []cmds.CommandDescriptionOption{
		cmds.WithShort(short),
		cmds.WithLayersList(runSelectorLayer),
	}
	if strings.TrimSpace(long) != "" {
		options = append(options, cmds.WithLong(long))
	}
	if len(flags) > 0 {
		options = append(options, cmds.WithFlags(flags...))
	}
	return cmds.NewCommandDescription(name, options...), nil
}

func initializeRunSelector(parsedLayers *layers.ParsedLayers) (*runSelectorSettings, error) {
	settings := &runSelectorSettings{}
	if err := parsedLayers.InitializeStruct(runSelectorLayerSlug, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func resolveRunSelectorToRunID(selector *runSelectorSettings) (*orchestrator.Service, string, error) {
	runID, ticket, err := requireRunSelector(selector.RunID, selector.Ticket)
	if err != nil {
		return nil, "", err
	}
	service, err := orchestrator.NewService(selector.DBPath)
	if err != nil {
		return nil, "", err
	}
	runID, err = service.ResolveRunID(runID, ticket)
	if err != nil {
		return nil, "", err
	}
	return service, runID, nil
}

type statusGlazedCommand struct {
	*cmds.CommandDescription
}

func newStatusGlazedCommand() (*statusGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"status",
		"Print run status",
		"Show status for the selected run.",
	)
	if err != nil {
		return nil, err
	}
	return &statusGlazedCommand{CommandDescription: desc}, nil
}

func (c *statusGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	service, runID, err := resolveRunSelectorToRunID(selector)
	if err != nil {
		return err
	}
	status, err := service.Status(ctx, runID)
	if err != nil {
		return err
	}
	fmt.Print(status)
	return nil
}

var _ cmds.BareCommand = &statusGlazedCommand{}

type resumeGlazedCommand struct {
	*cmds.CommandDescription
}

func newResumeGlazedCommand() (*resumeGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"resume",
		"Resume a paused run",
		"Resume the selected run.",
	)
	if err != nil {
		return nil, err
	}
	return &resumeGlazedCommand{CommandDescription: desc}, nil
}

func (c *resumeGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	service, runID, err := resolveRunSelectorToRunID(selector)
	if err != nil {
		return err
	}
	if err := service.Resume(ctx, runID); err != nil {
		return err
	}
	fmt.Printf("Run %s resumed.\n", runID)
	return nil
}

var _ cmds.BareCommand = &resumeGlazedCommand{}

type stopGlazedCommand struct {
	*cmds.CommandDescription
}

func newStopGlazedCommand() (*stopGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"stop",
		"Stop an active run",
		"Stop the selected run.",
	)
	if err != nil {
		return nil, err
	}
	return &stopGlazedCommand{CommandDescription: desc}, nil
}

func (c *stopGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	service, runID, err := resolveRunSelectorToRunID(selector)
	if err != nil {
		return err
	}
	if err := service.Stop(ctx, runID); err != nil {
		return err
	}
	fmt.Printf("Run %s stopped.\n", runID)
	return nil
}

var _ cmds.BareCommand = &stopGlazedCommand{}

type restartGlazedCommand struct {
	*cmds.CommandDescription
}

type restartSettings struct {
	DryRun bool `glazed.parameter:"dry-run"`
}

func newRestartGlazedCommand() (*restartGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"restart",
		"Restart a run",
		"Restart the selected run.",
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview restart actions without executing them"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &restartGlazedCommand{CommandDescription: desc}, nil
}

func (c *restartGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	restart := &restartSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, restart); err != nil {
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
	result, err := service.Restart(ctx, orchestrator.RestartOptions{
		RunID:  runID,
		Ticket: ticket,
		DryRun: restart.DryRun,
	})
	if err != nil {
		return err
	}

	if restart.DryRun {
		fmt.Printf("Restart dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Run %s restarted.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

var _ cmds.BareCommand = &restartGlazedCommand{}

type cleanupGlazedCommand struct {
	*cmds.CommandDescription
}

type cleanupSettings struct {
	DryRun         bool `glazed.parameter:"dry-run"`
	KeepWorkspaces bool `glazed.parameter:"keep-workspaces"`
}

func newCleanupGlazedCommand() (*cleanupGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"cleanup",
		"Clean up run workspaces/sessions",
		"Cleanup the selected run.",
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview cleanup actions without executing them"),
			parameters.WithDefault(false),
		),
		parameters.NewParameterDefinition(
			"keep-workspaces",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Keep workspaces; only stop agent sessions"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &cleanupGlazedCommand{CommandDescription: desc}, nil
}

func (c *cleanupGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	cleanup := &cleanupSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, cleanup); err != nil {
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
	result, err := service.Cleanup(ctx, orchestrator.CleanupOptions{
		RunID:            runID,
		Ticket:           ticket,
		DryRun:           cleanup.DryRun,
		DeleteWorkspaces: !cleanup.KeepWorkspaces,
	})
	if err != nil {
		return err
	}

	if cleanup.DryRun {
		fmt.Printf("Cleanup dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Run %s cleaned up.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

var _ cmds.BareCommand = &cleanupGlazedCommand{}

type mergeGlazedCommand struct {
	*cmds.CommandDescription
}

type mergeSettings struct {
	DryRun bool `glazed.parameter:"dry-run"`
	Human  bool `glazed.parameter:"human"`
}

func newMergeGlazedCommand() (*mergeGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"merge",
		"Merge run pull requests",
		"Merge selected run outputs.",
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview merge actions without executing them"),
			parameters.WithDefault(false),
		),
		parameters.NewParameterDefinition(
			"human",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Required acknowledgement for human-initiated merge execution"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &mergeGlazedCommand{CommandDescription: desc}, nil
}

func (c *mergeGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	merge := &mergeSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, merge); err != nil {
		return err
	}
	if !merge.DryRun && !merge.Human {
		return fmt.Errorf("merge requires --human acknowledgement; automated merge is disabled")
	}

	runID, ticket, err := requireRunSelector(selector.RunID, selector.Ticket)
	if err != nil {
		return err
	}
	service, err := orchestrator.NewService(selector.DBPath)
	if err != nil {
		return err
	}
	result, err := service.Merge(ctx, orchestrator.MergeOptions{
		RunID:  runID,
		Ticket: ticket,
		DryRun: merge.DryRun,
	})
	if err != nil {
		return err
	}

	if merge.DryRun {
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

var _ cmds.BareCommand = &mergeGlazedCommand{}

type commitGlazedCommand struct {
	*cmds.CommandDescription
}

type commitSettings struct {
	Message string `glazed.parameter:"message"`
	Actor   string `glazed.parameter:"actor"`
	DryRun  bool   `glazed.parameter:"dry-run"`
}

func newCommitGlazedCommand() (*commitGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"commit",
		"Commit workspace changes",
		"Commit selected run workspace changes.",
		parameters.NewParameterDefinition(
			"message",
			parameters.ParameterTypeString,
			parameters.WithHelp("Explicit commit message (defaults to ticket + run brief goal)"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"actor",
			parameters.ParameterTypeString,
			parameters.WithHelp("Actor identity to persist with commit metadata"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview commit actions without executing them"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &commitGlazedCommand{CommandDescription: desc}, nil
}

func (c *commitGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	commit := &commitSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, commit); err != nil {
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
	result, err := service.Commit(ctx, orchestrator.CommitOptions{
		RunID:   runID,
		Ticket:  ticket,
		Message: commit.Message,
		Actor:   commit.Actor,
		DryRun:  commit.DryRun,
	})
	if err != nil {
		var inProgress *orchestrator.RunMutationInProgressError
		if errors.As(err, &inProgress) {
			return fmt.Errorf("%w; retry after the active %s operation completes", err, inProgress.Operation)
		}
		return err
	}

	if commit.DryRun {
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
		fmt.Printf("    actor=%s source=%s\n", emptyValue(repo.Actor, "unknown"), emptyValue(repo.ActorSource, "unknown"))
		for _, line := range repo.Preflight {
			fmt.Printf("    preflight: %s\n", line)
		}
		if commit.DryRun {
			for _, action := range repo.Actions {
				fmt.Printf("    dry-run: %s\n", action)
			}
			continue
		}
		fmt.Printf("    commit=%s\n", emptyValue(repo.CommitSHA, "-"))
	}
	return nil
}

var _ cmds.BareCommand = &commitGlazedCommand{}

type prGlazedCommand struct {
	*cmds.CommandDescription
}

type prSettings struct {
	Title  string `glazed.parameter:"title"`
	Body   string `glazed.parameter:"body"`
	Actor  string `glazed.parameter:"actor"`
	DryRun bool   `glazed.parameter:"dry-run"`
}

func newPRGlazedCommand() (*prGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"pr",
		"Open or update pull requests",
		"Open or update pull requests for selected run repositories.",
		parameters.NewParameterDefinition(
			"title",
			parameters.ParameterTypeString,
			parameters.WithHelp("Explicit pull request title"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"body",
			parameters.ParameterTypeString,
			parameters.WithHelp("Explicit pull request body"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"actor",
			parameters.ParameterTypeString,
			parameters.WithHelp("Actor identity to persist with pull request metadata"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview pull request actions without executing them"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &prGlazedCommand{CommandDescription: desc}, nil
}

func (c *prGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	pr := &prSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, pr); err != nil {
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
	result, err := service.OpenPullRequests(ctx, orchestrator.PullRequestOptions{
		RunID:  runID,
		Ticket: ticket,
		Title:  pr.Title,
		Body:   pr.Body,
		Actor:  pr.Actor,
		DryRun: pr.DryRun,
	})
	if err != nil {
		var inProgress *orchestrator.RunMutationInProgressError
		if errors.As(err, &inProgress) {
			return fmt.Errorf("%w; retry after the active %s operation completes", err, inProgress.Operation)
		}
		return err
	}

	if pr.DryRun {
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
		fmt.Printf("    actor=%s source=%s\n", emptyValue(repo.Actor, "unknown"), emptyValue(repo.ActorSource, "unknown"))
		for _, line := range repo.Preflight {
			fmt.Printf("    preflight: %s\n", line)
		}
		if pr.DryRun {
			for _, action := range repo.Actions {
				fmt.Printf("    dry-run: %s\n", action)
			}
			continue
		}
		fmt.Printf("    pr=%s number=%d state=%s\n", emptyValue(repo.PRURL, "-"), repo.PRNumber, emptyValue(string(repo.PRState), "-"))
	}
	return nil
}

var _ cmds.BareCommand = &prGlazedCommand{}

type iterateGlazedCommand struct {
	*cmds.CommandDescription
}

type iterateSettings struct {
	Feedback string `glazed.parameter:"feedback"`
	DryRun   bool   `glazed.parameter:"dry-run"`
}

func newIterateGlazedCommand() (*iterateGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"iterate",
		"Send feedback to continue a run",
		"Send operator feedback to guide another iteration.",
		parameters.NewParameterDefinition(
			"feedback",
			parameters.ParameterTypeString,
			parameters.WithHelp("Operator feedback to guide the next implementation iteration"),
			parameters.WithDefault(""),
		),
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview iterate actions without executing them"),
			parameters.WithDefault(false),
		),
	)
	if err != nil {
		return nil, err
	}
	return &iterateGlazedCommand{CommandDescription: desc}, nil
}

func (c *iterateGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	iterate := &iterateSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, iterate); err != nil {
		return err
	}
	if strings.TrimSpace(iterate.Feedback) == "" {
		return fmt.Errorf("--feedback is required")
	}

	runID, ticket, err := requireRunSelector(selector.RunID, selector.Ticket)
	if err != nil {
		return err
	}
	service, err := orchestrator.NewService(selector.DBPath)
	if err != nil {
		return err
	}
	result, err := service.Iterate(ctx, orchestrator.IterateOptions{
		RunID:    runID,
		Ticket:   ticket,
		Feedback: iterate.Feedback,
		DryRun:   iterate.DryRun,
	})
	if err != nil {
		return err
	}

	if iterate.DryRun {
		fmt.Printf("Iterate dry-run for run %s:\n", result.RunID)
	} else {
		fmt.Printf("Iterate started for run %s.\n", result.RunID)
	}
	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}
	return nil
}

var _ cmds.BareCommand = &iterateGlazedCommand{}

type closeGlazedCommand struct {
	*cmds.CommandDescription
}

type closeSettings struct {
	DryRun         bool   `glazed.parameter:"dry-run"`
	ChangelogEntry string `glazed.parameter:"changelog-entry"`
}

func newCloseGlazedCommand() (*closeGlazedCommand, error) {
	desc, err := newRunSelectorCommandDescription(
		"close",
		"Close a completed run",
		"Close selected run and perform configured close actions.",
		parameters.NewParameterDefinition(
			"dry-run",
			parameters.ParameterTypeBool,
			parameters.WithHelp("Preview close actions"),
			parameters.WithDefault(false),
		),
		parameters.NewParameterDefinition(
			"changelog-entry",
			parameters.ParameterTypeString,
			parameters.WithHelp("Changelog entry for docmgr ticket close"),
			parameters.WithDefault(""),
		),
	)
	if err != nil {
		return nil, err
	}
	return &closeGlazedCommand{CommandDescription: desc}, nil
}

func (c *closeGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	selector, err := initializeRunSelector(parsedLayers)
	if err != nil {
		return err
	}
	closeSettings := &closeSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, closeSettings); err != nil {
		return err
	}

	service, runID, err := resolveRunSelectorToRunID(selector)
	if err != nil {
		return err
	}
	if err := service.Close(ctx, orchestrator.CloseOptions{
		RunID:          runID,
		DryRun:         closeSettings.DryRun,
		ChangelogEntry: closeSettings.ChangelogEntry,
	}); err != nil {
		return err
	}
	if closeSettings.DryRun {
		fmt.Printf("Close dry-run complete for run %s.\n", runID)
	} else {
		fmt.Printf("Run %s closed.\n", runID)
	}
	return nil
}

var _ cmds.BareCommand = &closeGlazedCommand{}
