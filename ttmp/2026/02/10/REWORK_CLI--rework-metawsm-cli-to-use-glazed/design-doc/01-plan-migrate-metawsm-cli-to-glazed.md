---
Title: 'Plan: migrate metawsm CLI to glazed'
Ticket: REWORK_CLI
Status: active
Topics:
    - cli
    - core
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: ../../../../../../../docmgr/cmd/docmgr/cmds/common/common.go
      Note: Reference wrapper for building glazed commands with shared parser defaults
    - Path: ../../../../../../../glazed/pkg/cli/cobra.go
      Note: Cobra integration and BuildCobraCommand wiring
    - Path: ../../../../../../../glazed/pkg/cmds/cmds.go
      Note: Glazed command description and layered flag definition model
    - Path: ../../../../../../../glazed/pkg/doc/topics/13-layers-and-parsed-layers.md
      Note: Layer model for reusable parameter groups
    - Path: ../../../../../../../glazed/pkg/doc/tutorials/build-first-command.md
      Note: Canonical example for creating glazed commands
    - Path: cmd/metawsm/main.go
      Note: Current CLI command dispatch and all flag definitions
    - Path: cmd/metawsm/main_test.go
      Note: Current CLI parsing and usage behavior tests
ExternalSources: []
Summary: Plan and phased migration strategy for moving metawsm CLI parsing from stdlib flag sets in main.go to glazed/cobra command descriptions while preserving current command behavior.
LastUpdated: 2026-02-10T07:15:31-08:00
WhatFor: Rework the metawsm CLI architecture to improve maintainability, reduce flag duplication, and support richer command metadata/help using glazed.
WhenToUse: Before implementing REWORK_CLI code changes that restructure CLI command parsing and registration.
---


# Plan: migrate metawsm CLI to glazed

## Executive Summary

`metawsm` currently defines its entire CLI surface in `cmd/metawsm/main.go` with manual `flag.NewFlagSet` usage and manual command/subcommand dispatch. This works, but it is expensive to extend safely because parsing, validation, usage messaging, and command execution are tightly coupled and repeated across commands.

This plan migrates the CLI to `glazed` in phases, keeping command behavior and flag names stable while introducing:
- `glazed` command descriptions (`cmds.CommandDescription`) for flag definitions,
- `cobra` command tree registration via `cli.BuildCobraCommand`,
- reusable parameter/layer patterns for shared flags (`--run-id`, `--ticket`, `--db`, `--policy`, etc.),
- improved long-term extensibility without a high-risk big-bang rewrite.

## Problem Statement

Current CLI structure has grown to the point where change risk is high:

1. Parsing and business logic are coupled in one file.
- `cmd/metawsm/main.go` is the command router and all command implementations.
- Each command constructs its own `flag.FlagSet` and parses inline.

2. Flag definitions are duplicated heavily.
- There are currently 30 `flag.NewFlagSet(...)` callsites and ~180 flag-definition callsites (`StringVar`/`BoolVar`/`IntVar`/`DurationVar`/`Var`) in `cmd/metawsm/main.go`.
- Shared selectors (`--run-id`, `--ticket`, `--db`) are redefined repeatedly.

3. Subcommand structure is manually simulated.
- `auth` requires trailing `check` by inspecting `fs.Args()`.
- `review` requires trailing `sync` similarly.
- `forum` dispatches to subcommands by hand.

4. Help/usage consistency is manual and brittle.
- `printUsage()` is hardcoded and can drift from actual parser behavior.
- Usage errors are manually formatted in command functions.

5. Testing validates behavior, but structure still impedes change.
- `cmd/metawsm/main_test.go` covers many validation helpers and error cases, but parser wiring is not modular, so new commands still require repetitive boilerplate.

## Proposed Solution

Introduce a glazed-first command architecture while preserving external CLI behavior.

### 1) Create a command package and cobra root
- Add a dedicated CLI package (for example `cmd/metawsm/cmds`) with:
  - root command constructor,
  - one constructor per command group (`run`, `bootstrap`, `forum`, `auth`, etc.),
  - small wrappers that call existing service functions.
- Replace manual `main()` switch dispatch with `rootCmd.ExecuteContext(...)`.

### 2) Port commands to glazed command descriptions
- For each command, define `cmds.NewCommandDescription(...)` with `cmds.WithFlags(...)`.
- Use typed settings structs with `glazed.parameter` tags and `parsedLayers.InitializeStruct(...)`.
- Register commands using `cli.BuildCobraCommand(...)` (same pattern used in `docmgr`).

### 3) Introduce shared layers/helpers for repeated flags
- Extract reusable selectors and common flags:
  - run selector layer (`run-id`, `ticket`),
  - persistence/config layer (`db`, `policy`),
  - watch/operator runtime layer (`interval`, `notify-cmd`, `bell`, `all`, `dry-run`),
  - forum actor/server layer (`server`, `actor-type`, `actor-name`).
- Keep existing flag names and defaults to avoid user-visible breakage.

### 4) Normalize hierarchy and semantics
- Encode actual hierarchy in cobra/glazed instead of manual argument checks:
  - `metawsm auth check`
  - `metawsm review sync`
  - `metawsm forum ask|answer|...`
- Keep error messages close to existing wording where tests/operators depend on them.

### 5) Preserve output contracts first, add structured output later
- First migration target: behavior-preserving refactor.
- Commands continue as `BareCommand`/`WriterCommand` equivalents where appropriate.
- Optional follow-up: promote selected read commands (`status`, `docs`, maybe `forum list`) to true `GlazeCommand` output modes.

## Design Decisions

1. Use glazed over raw cobra.
- Rationale: glazed gives typed parameter definitions, layer composition, and common parsing patterns while still building on cobra command trees.

2. Preserve flag names/defaults and command spellings in phase 1.
- Rationale: this is an architectural refactor, not a UX redesign.

3. Migrate incrementally by command group.
- Rationale: reduces regression surface and allows per-group testing/rollback.

4. Keep existing domain/service logic unchanged during CLI port.
- Rationale: isolate parser/framework migration from orchestration/runtime behavior.

5. Start with bare output behavior, not immediate structured output conversion.
- Rationale: avoids mixing parser migration with output contract changes.

## Alternatives Considered

1. Keep stdlib `flag` and only split `main.go` into files.
- Rejected: lowers file size but not repetition; does not provide shared parameter model or command metadata.

2. Migrate to plain cobra without glazed.
- Rejected: better than stdlib `flag`, but still requires more custom parameter plumbing that glazed already provides.

3. Big-bang rewrite of all commands at once.
- Rejected: too much behavioral risk across a broad CLI surface.

4. Immediate structured-output redesign for all commands.
- Rejected: mixes two large changes (framework + output contract), increasing failure risk.

## Implementation Plan

1. Baseline and guardrails.
- Snapshot current command/flag matrix and expected usage text.
- Add focused tests for command parsing and selector validation where coverage is thin.

2. Add glazed dependencies and root scaffolding.
- Add `github.com/go-go-golems/glazed` to `metawsm/go.mod`.
- Introduce cobra/glazed root command wiring and keep `main.go` as thin bootstrap.

3. Port low-risk standalone commands first.
- `policy-init`, `serve`, `docs`.
- Verify no behavior change in flags, defaults, and output.

4. Port run-selector command family.
- `status`, `resume`, `stop`, `restart`, `cleanup`, `merge`, `commit`, `pr`, `iterate`, `close`.
- Centralize shared selector parsing helpers in glazed layers/settings structs.

5. Port grouped commands.
- `auth check` and `review sync` as true subcommands.
- `forum` subcommand tree (`ask|answer|assign|state|priority|close|list|thread|watch|signal`).

6. Port long-loop operator surfaces.
- `watch`, `operator`, `tui` with same runtime/control behavior and signal handling.

7. Remove legacy parser path and stabilize.
- Delete old `flag.NewFlagSet` command plumbing.
- Keep compatibility aliases where needed.
- Update README/docs examples if any help text changed.

8. Validation gates per phase.
- `go test ./cmd/metawsm -count=1`
- `go test ./...`
- targeted manual smoke checks for key command families.

## Open Questions

1. Should we expose `--with-glaze-output` on selected read commands during the same ticket, or defer to a follow-up ticket?
2. Do we want carapace completion wiring in this migration, or strictly parser/framework parity?
3. Should command code live under `cmd/metawsm/cmds` (CLI-first) or `internal/cli` (package-first)?
4. Do we preserve every current wording in usage errors, or allow minor text drift if semantics are unchanged?

## References

- `cmd/metawsm/main.go`
- `cmd/metawsm/main_test.go`
- `glazed/pkg/cmds/cmds.go`
- `glazed/pkg/cli/cobra.go`
- `glazed/pkg/doc/tutorials/build-first-command.md`
- `glazed/pkg/doc/topics/13-layers-and-parsed-layers.md`
- `docmgr/cmd/docmgr/cmds/common/common.go`
- `docmgr/pkg/commands/create_ticket.go`
