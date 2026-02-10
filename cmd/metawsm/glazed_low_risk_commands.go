package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"metawsm/internal/docfederation"
	"metawsm/internal/orchestrator"
	"metawsm/internal/policy"
	"metawsm/internal/server"

	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/layers"
	"github.com/go-go-golems/glazed/pkg/cmds/parameters"
)

type policyInitGlazedCommand struct {
	*cmds.CommandDescription
}

type policyInitSettings struct {
	Path string `glazed.parameter:"path"`
}

func newPolicyInitGlazedCommand() (*policyInitGlazedCommand, error) {
	return &policyInitGlazedCommand{
		CommandDescription: cmds.NewCommandDescription(
			"policy-init",
			cmds.WithShort("Write a default policy file"),
			cmds.WithLong("Create a default metawsm policy file at the target path."),
			cmds.WithFlags(
				parameters.NewParameterDefinition(
					"path",
					parameters.ParameterTypeString,
					parameters.WithHelp("Path to policy file"),
					parameters.WithDefault(policy.DefaultPolicyPath),
				),
			),
		),
	}, nil
}

func (c *policyInitGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	_ = ctx
	settings := &policyInitSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}
	if err := policy.SaveDefault(settings.Path); err != nil {
		return err
	}
	fmt.Printf("Wrote default policy to %s\n", settings.Path)
	return nil
}

var _ cmds.BareCommand = &policyInitGlazedCommand{}

type docsGlazedCommand struct {
	*cmds.CommandDescription
}

type docsSettings struct {
	DBPath        string   `glazed.parameter:"db"`
	PolicyPath    string   `glazed.parameter:"policy"`
	Refresh       bool     `glazed.parameter:"refresh"`
	Ticket        string   `glazed.parameter:"ticket"`
	EndpointNames []string `glazed.parameter:"endpoint"`
}

func newDocsGlazedCommand() (*docsGlazedCommand, error) {
	return &docsGlazedCommand{
		CommandDescription: cmds.NewCommandDescription(
			"docs",
			cmds.WithShort("Show federated docs health and ticket index"),
			cmds.WithLong("Read docs API endpoints from policy and print merged workspace-first ticket views."),
			cmds.WithFlags(
				parameters.NewParameterDefinition(
					"db",
					parameters.ParameterTypeString,
					parameters.WithHelp("Path to SQLite DB"),
					parameters.WithDefault(".metawsm/metawsm.db"),
				),
				parameters.NewParameterDefinition(
					"policy",
					parameters.ParameterTypeString,
					parameters.WithHelp("Path to policy file (defaults to .metawsm/policy.json)"),
					parameters.WithDefault(""),
				),
				parameters.NewParameterDefinition(
					"refresh",
					parameters.ParameterTypeBool,
					parameters.WithHelp("Call /api/v1/index/refresh before aggregation"),
					parameters.WithDefault(false),
				),
				parameters.NewParameterDefinition(
					"ticket",
					parameters.ParameterTypeString,
					parameters.WithHelp("Optional ticket filter"),
					parameters.WithDefault(""),
				),
				parameters.NewParameterDefinition(
					"endpoint",
					parameters.ParameterTypeStringList,
					parameters.WithHelp("Endpoint names for --refresh selection (repeatable, or comma-separated)"),
					parameters.WithDefault([]string{}),
				),
			),
		),
	}, nil
}

func (c *docsGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	settings := &docsSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}

	service, err := orchestrator.NewService(settings.DBPath)
	if err != nil {
		return err
	}
	cfg, _, err := policy.Load(settings.PolicyPath)
	if err != nil {
		return err
	}
	endpoints := federationEndpointsFromPolicy(cfg)
	if len(endpoints) == 0 {
		return fmt.Errorf("no docs.api endpoints configured in policy")
	}

	timeout := time.Duration(cfg.Docs.API.RequestTimeoutSec) * time.Second
	client := docfederation.NewClient(timeout)

	if settings.Refresh {
		selected := selectFederationEndpoints(endpoints, normalizeInputTokens(settings.EndpointNames))
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
	ticketFilter := strings.TrimSpace(settings.Ticket)

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

var _ cmds.BareCommand = &docsGlazedCommand{}

type serveGlazedCommand struct {
	*cmds.CommandDescription
}

type serveSettings struct {
	Addr            string `glazed.parameter:"addr"`
	DBPath          string `glazed.parameter:"db"`
	WorkerInterval  string `glazed.parameter:"worker-interval"`
	WorkerBatchSize int    `glazed.parameter:"worker-batch-size"`
	WorkerLogPeriod string `glazed.parameter:"worker-log-period"`
	ShutdownTimeout string `glazed.parameter:"shutdown-timeout"`
}

func newServeGlazedCommand() (*serveGlazedCommand, error) {
	return &serveGlazedCommand{
		CommandDescription: cmds.NewCommandDescription(
			"serve",
			cmds.WithShort("Run forum/API server"),
			cmds.WithLong("Start the metawsm API server and forum worker loop."),
			cmds.WithFlags(
				parameters.NewParameterDefinition(
					"addr",
					parameters.ParameterTypeString,
					parameters.WithHelp("HTTP listen address"),
					parameters.WithDefault(":3001"),
				),
				parameters.NewParameterDefinition(
					"db",
					parameters.ParameterTypeString,
					parameters.WithHelp("Path to SQLite DB"),
					parameters.WithDefault(".metawsm/metawsm.db"),
				),
				parameters.NewParameterDefinition(
					"worker-interval",
					parameters.ParameterTypeString,
					parameters.WithHelp("Forum worker loop interval"),
					parameters.WithDefault("500ms"),
				),
				parameters.NewParameterDefinition(
					"worker-batch-size",
					parameters.ParameterTypeInteger,
					parameters.WithHelp("Forum worker ProcessOnce batch size"),
					parameters.WithDefault(100),
				),
				parameters.NewParameterDefinition(
					"worker-log-period",
					parameters.ParameterTypeString,
					parameters.WithHelp("Forum worker summary log period"),
					parameters.WithDefault("15s"),
				),
				parameters.NewParameterDefinition(
					"shutdown-timeout",
					parameters.ParameterTypeString,
					parameters.WithHelp("Graceful shutdown timeout"),
					parameters.WithDefault("5s"),
				),
			),
		),
	}, nil
}

func parseDurationSetting(flagName string, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid --%s duration %q: %w", flagName, value, err)
	}
	return duration, nil
}

func (c *serveGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	settings := &serveSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}

	workerInterval, err := parseDurationSetting("worker-interval", settings.WorkerInterval)
	if err != nil {
		return err
	}
	workerLogPeriod, err := parseDurationSetting("worker-log-period", settings.WorkerLogPeriod)
	if err != nil {
		return err
	}
	shutdownTimeout, err := parseDurationSetting("shutdown-timeout", settings.ShutdownTimeout)
	if err != nil {
		return err
	}

	runtime, err := server.NewRuntime(server.Options{
		Addr:            settings.Addr,
		DBPath:          settings.DBPath,
		WorkerInterval:  workerInterval,
		WorkerBatchSize: settings.WorkerBatchSize,
		WorkerLogPeriod: workerLogPeriod,
		ShutdownTimeout: shutdownTimeout,
	})
	if err != nil {
		return err
	}

	fmt.Printf("metawsm serve listening on %s\n", settings.Addr)
	return runtime.Run(ctx)
}

var _ cmds.BareCommand = &serveGlazedCommand{}
