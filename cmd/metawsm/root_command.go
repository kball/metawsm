package main

import (
	"fmt"

	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/layers"
	"github.com/spf13/cobra"
)

type legacyPassthroughSpec struct {
	Use     string
	Short   string
	Aliases []string
	Run     func(args []string) error
}

func executeCLI(args []string) error {
	rootCmd, err := newRootCommand()
	if err != nil {
		return err
	}
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func newRootCommand() (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Use:           "metawsm",
		Short:         "orchestrate multi-ticket multi-workspace agent runs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			printUsage()
			return fmt.Errorf("command is required")
		},
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	defaultHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			printUsage()
			return
		}
		defaultHelpFunc(cmd, args)
	})

	migrated := []cmds.Command{}
	statusCmd, err := newStatusGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, statusCmd)

	resumeCmd, err := newResumeGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, resumeCmd)

	stopCmd, err := newStopGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, stopCmd)

	restartCmd, err := newRestartGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, restartCmd)

	cleanupCmd, err := newCleanupGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, cleanupCmd)

	mergeCmd, err := newMergeGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, mergeCmd)

	commitCmd, err := newCommitGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, commitCmd)

	prCmd, err := newPRGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, prCmd)

	iterateCmd, err := newIterateGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, iterateCmd)

	closeCmd, err := newCloseGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, closeCmd)

	policyInitCmd, err := newPolicyInitGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, policyInitCmd)

	docsCmd, err := newDocsGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, docsCmd)

	serveCmd, err := newServeGlazedCommand()
	if err != nil {
		return nil, err
	}
	migrated = append(migrated, serveCmd)

	for _, command := range migrated {
		cobraCommand, err := buildGlazedCobraCommand(command)
		if err != nil {
			return nil, err
		}
		rootCmd.AddCommand(cobraCommand)
	}

	legacySpecs := []legacyPassthroughSpec{
		{Use: "run", Short: "Start a multi-ticket run", Run: runCommand},
		{Use: "bootstrap", Short: "Bootstrap a ticket run interactively", Run: bootstrapCommand},
		{Use: "auth", Short: "Auth subcommands", Run: authCommand},
		{Use: "review", Short: "Review subcommands", Run: reviewCommand},
		{Use: "watch", Short: "Watch run status and alerts", Run: watchCommand},
		{Use: "operator", Short: "Operator loop for run supervision", Run: operatorCommand},
		{Use: "forum", Short: "Forum subcommands", Run: forumCommand},
		{Use: "tui", Short: "Terminal UI monitor", Run: tuiCommand},
	}

	for _, spec := range legacySpecs {
		addLegacyPassthroughCommand(rootCmd, spec)
	}

	return rootCmd, nil
}

func buildGlazedCobraCommand(command cmds.Command) (*cobra.Command, error) {
	return cli.BuildCobraCommand(
		command,
		cli.WithParserConfig(cli.CobraParserConfig{
			ShortHelpLayers: []string{layers.DefaultSlug},
			MiddlewaresFunc: cli.CobraCommandDefaultMiddlewares,
		}),
		cli.WithCobraMiddlewaresFunc(cli.CobraCommandDefaultMiddlewares),
		cli.WithCobraShortHelpLayers(layers.DefaultSlug),
	)
}

func addLegacyPassthroughCommand(rootCmd *cobra.Command, spec legacyPassthroughSpec) {
	cmd := &cobra.Command{
		Use:                spec.Use,
		Short:              spec.Short,
		Aliases:            spec.Aliases,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Run(args)
		},
	}
	rootCmd.AddCommand(cmd)
}
