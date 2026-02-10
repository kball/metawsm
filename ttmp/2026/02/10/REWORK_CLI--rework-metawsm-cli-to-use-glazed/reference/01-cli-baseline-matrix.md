---
Title: CLI baseline matrix
Ticket: REWORK_CLI
Status: active
Topics:
    - cli
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: |-
        Current stdlib flag-based CLI implementation and usage surface
        Source of command matrix and parser baseline
    - Path: cmd/metawsm/main_test.go
      Note: |-
        Baseline behavior tests for selector and subcommand validation
        Baseline usage coverage test
ExternalSources: []
Summary: Baseline command matrix and parser complexity before glazed migration
LastUpdated: 2026-02-10T09:20:00-08:00
WhatFor: Capture exact current CLI command families and parser complexity to validate migration parity.
WhenToUse: Before and during migration phases to compare command coverage and help text parity.
---


# CLI baseline matrix

## Goal

Document the current `metawsm` CLI shape and parser footprint before moving to glazed/cobra command registration.

## Context

This baseline comes from `cmd/metawsm/main.go` usage output plus command parser inventory counts.

## Quick Reference

### Command families from `printUsage`

| Family | Commands |
| --- | --- |
| Run lifecycle | `run`, `bootstrap`, `status`, `resume`, `stop`, `restart`, `cleanup`, `iterate`, `close` |
| Approval/review | `auth check`, `review sync` |
| Operator loops | `watch`, `operator`, `tui` |
| Forum | `forum ask`, `forum answer`, `forum assign`, `forum state`, `forum priority`, `forum close`, `forum list`, `forum thread`, `forum watch`, `forum signal`, `forum debug` |
| Delivery | `commit`, `pr`, `merge` |
| Utility | `policy-init`, `docs`, `serve` |

### Parser complexity snapshot (pre-migration)

- `flag.NewFlagSet(...)` callsites: `31`
- Flag definition callsites (`StringVar/BoolVar/IntVar/DurationVar/Var`): `185`
- Usage command lines: `21`

### Usage-line source of truth (task-1 baseline)

- `usageCommandLines` in `cmd/metawsm/main.go`

## Usage Examples

- Validate baseline tests:

```bash
go test ./cmd/metawsm -count=1
```

- Recompute parser counts during migration:

```bash
rg -c "flag.NewFlagSet\(" cmd/metawsm/main.go
rg -c "\.StringVar\(|\.BoolVar\(|\.IntVar\(|\.DurationVar\(|\.Var\(" cmd/metawsm/main.go
```

## Related

- `ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/design-doc/01-plan-migrate-metawsm-cli-to-glazed.md`
- `ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/reference/01-diary.md`
