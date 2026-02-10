package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/layers"
	"github.com/go-go-golems/glazed/pkg/cmds/parameters"
)

func appendStringFlag(args []string, name string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return args
	}
	return append(args, "--"+name, value)
}

func appendIntFlag(args []string, name string, value int, defaultValue int) []string {
	if value == defaultValue {
		return args
	}
	return append(args, fmt.Sprintf("--%s=%d", name, value))
}

func appendBoolFlag(args []string, name string, value bool, defaultValue bool) []string {
	if value == defaultValue {
		return args
	}
	return append(args, fmt.Sprintf("--%s=%t", name, value))
}

type watchGlazedCommand struct {
	*cmds.CommandDescription
}

type watchSettings struct {
	RunID     string `glazed.parameter:"run-id"`
	Ticket    string `glazed.parameter:"ticket"`
	DBPath    string `glazed.parameter:"db"`
	Interval  int    `glazed.parameter:"interval"`
	NotifyCmd string `glazed.parameter:"notify-cmd"`
	Bell      bool   `glazed.parameter:"bell"`
	All       bool   `glazed.parameter:"all"`
}

func newWatchGlazedCommand() (*watchGlazedCommand, error) {
	desc := cmds.NewCommandDescription(
		"watch",
		cmds.WithShort("Watch run status and alerts"),
		cmds.WithLong("Watch run status and emit operator-facing alerts."),
		cmds.WithFlags(
			parameters.NewParameterDefinition("run-id", parameters.ParameterTypeString, parameters.WithHelp("Run identifier"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("ticket", parameters.ParameterTypeString, parameters.WithHelp("Ticket identifier (operate on latest run for this ticket)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("db", parameters.ParameterTypeString, parameters.WithHelp("Path to SQLite DB"), parameters.WithDefault(".metawsm/metawsm.db")),
			parameters.NewParameterDefinition("interval", parameters.ParameterTypeInteger, parameters.WithHelp("Heartbeat interval in seconds"), parameters.WithDefault(15)),
			parameters.NewParameterDefinition("notify-cmd", parameters.ParameterTypeString, parameters.WithHelp("Optional shell command to run on alert"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("bell", parameters.ParameterTypeBool, parameters.WithHelp("Emit terminal bell on alert"), parameters.WithDefault(true)),
			parameters.NewParameterDefinition("all", parameters.ParameterTypeBool, parameters.WithHelp("Watch all active runs/tickets/agents"), parameters.WithDefault(false)),
		),
	)
	return &watchGlazedCommand{CommandDescription: desc}, nil
}

func (c *watchGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	_ = ctx
	settings := &watchSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}
	args := []string{}
	args = appendStringFlag(args, "run-id", settings.RunID)
	args = appendStringFlag(args, "ticket", settings.Ticket)
	args = appendStringFlag(args, "db", settings.DBPath)
	args = appendIntFlag(args, "interval", settings.Interval, 15)
	args = appendStringFlag(args, "notify-cmd", settings.NotifyCmd)
	args = appendBoolFlag(args, "bell", settings.Bell, true)
	args = appendBoolFlag(args, "all", settings.All, false)
	return watchCommand(args)
}

var _ cmds.BareCommand = &watchGlazedCommand{}

type operatorGlazedCommand struct {
	*cmds.CommandDescription
}

type operatorSettings struct {
	RunID     string `glazed.parameter:"run-id"`
	Ticket    string `glazed.parameter:"ticket"`
	DBPath    string `glazed.parameter:"db"`
	Policy    string `glazed.parameter:"policy"`
	LLMMode   string `glazed.parameter:"llm-mode"`
	Interval  int    `glazed.parameter:"interval"`
	NotifyCmd string `glazed.parameter:"notify-cmd"`
	Bell      bool   `glazed.parameter:"bell"`
	All       bool   `glazed.parameter:"all"`
	DryRun    bool   `glazed.parameter:"dry-run"`
}

func newOperatorGlazedCommand() (*operatorGlazedCommand, error) {
	desc := cmds.NewCommandDescription(
		"operator",
		cmds.WithShort("Operator loop for run supervision"),
		cmds.WithLong("Supervise run state and optionally trigger deterministic actions."),
		cmds.WithFlags(
			parameters.NewParameterDefinition("run-id", parameters.ParameterTypeString, parameters.WithHelp("Run identifier"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("ticket", parameters.ParameterTypeString, parameters.WithHelp("Ticket identifier (operate on latest run for this ticket)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("db", parameters.ParameterTypeString, parameters.WithHelp("Path to SQLite DB"), parameters.WithDefault(".metawsm/metawsm.db")),
			parameters.NewParameterDefinition("policy", parameters.ParameterTypeString, parameters.WithHelp("Path to policy file (defaults to .metawsm/policy.json)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("llm-mode", parameters.ParameterTypeString, parameters.WithHelp("Operator LLM mode override (off|assist|auto)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("interval", parameters.ParameterTypeInteger, parameters.WithHelp("Heartbeat interval in seconds"), parameters.WithDefault(15)),
			parameters.NewParameterDefinition("notify-cmd", parameters.ParameterTypeString, parameters.WithHelp("Optional shell command to run on alert"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("bell", parameters.ParameterTypeBool, parameters.WithHelp("Emit terminal bell on alert"), parameters.WithDefault(true)),
			parameters.NewParameterDefinition("all", parameters.ParameterTypeBool, parameters.WithHelp("Operate on all active runs/tickets/agents"), parameters.WithDefault(false)),
			parameters.NewParameterDefinition("dry-run", parameters.ParameterTypeBool, parameters.WithHelp("Observe only; do not execute actions"), parameters.WithDefault(false)),
		),
	)
	return &operatorGlazedCommand{CommandDescription: desc}, nil
}

func (c *operatorGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	_ = ctx
	settings := &operatorSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}
	args := []string{}
	args = appendStringFlag(args, "run-id", settings.RunID)
	args = appendStringFlag(args, "ticket", settings.Ticket)
	args = appendStringFlag(args, "db", settings.DBPath)
	args = appendStringFlag(args, "policy", settings.Policy)
	args = appendStringFlag(args, "llm-mode", settings.LLMMode)
	args = appendIntFlag(args, "interval", settings.Interval, 15)
	args = appendStringFlag(args, "notify-cmd", settings.NotifyCmd)
	args = appendBoolFlag(args, "bell", settings.Bell, true)
	args = appendBoolFlag(args, "all", settings.All, false)
	args = appendBoolFlag(args, "dry-run", settings.DryRun, false)
	return operatorCommand(args)
}

var _ cmds.BareCommand = &operatorGlazedCommand{}

type tuiGlazedCommand struct {
	*cmds.CommandDescription
}

type tuiSettings struct {
	RunID    string `glazed.parameter:"run-id"`
	Ticket   string `glazed.parameter:"ticket"`
	DBPath   string `glazed.parameter:"db"`
	Interval int    `glazed.parameter:"interval"`
}

func newTUIGlazedCommand() (*tuiGlazedCommand, error) {
	desc := cmds.NewCommandDescription(
		"tui",
		cmds.WithShort("Terminal UI monitor"),
		cmds.WithLong("Render a periodic terminal status view for selected or active runs."),
		cmds.WithFlags(
			parameters.NewParameterDefinition("run-id", parameters.ParameterTypeString, parameters.WithHelp("Specific run to monitor (optional)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("ticket", parameters.ParameterTypeString, parameters.WithHelp("Specific ticket to monitor (optional; resolves latest run)"), parameters.WithDefault("")),
			parameters.NewParameterDefinition("db", parameters.ParameterTypeString, parameters.WithHelp("Path to SQLite DB"), parameters.WithDefault(".metawsm/metawsm.db")),
			parameters.NewParameterDefinition("interval", parameters.ParameterTypeInteger, parameters.WithHelp("Refresh interval in seconds"), parameters.WithDefault(2)),
		),
	)
	return &tuiGlazedCommand{CommandDescription: desc}, nil
}

func (c *tuiGlazedCommand) Run(ctx context.Context, parsedLayers *layers.ParsedLayers) error {
	_ = ctx
	settings := &tuiSettings{}
	if err := parsedLayers.InitializeStruct(layers.DefaultSlug, settings); err != nil {
		return err
	}
	args := []string{}
	args = appendStringFlag(args, "run-id", settings.RunID)
	args = appendStringFlag(args, "ticket", settings.Ticket)
	args = appendStringFlag(args, "db", settings.DBPath)
	args = appendIntFlag(args, "interval", settings.Interval, 2)
	return tuiCommand(args)
}

var _ cmds.BareCommand = &tuiGlazedCommand{}
