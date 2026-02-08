---
Title: Credential strategies for commit and PR automation
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service.go
      Note: Credentials are consumed by push and PR creation service actions
    - Path: internal/policy/policy.go
      Note: Policy selects allowed credential mode and fallback behavior
    - Path: cmd/metawsm/main.go
      Note: CLI exposes credential mode and dry-run diagnostics
ExternalSources: []
Summary: 'Credential strategy options for enabling metawsm to push commits and open GitHub PRs safely across local and sandboxed environments.'
LastUpdated: 2026-02-08T11:02:06-08:00
WhatFor: 'Provide concrete credential architecture proposals with tradeoffs and rollout guidance.'
WhenToUse: 'When deciding how metawsm should authenticate git push and gh pr create operations.'
---

# Credential strategies for commit and PR automation

## Goal

Choose a safe and practical way for `metawsm` to authenticate `git push` and `gh pr create` while preserving auditability and least privilege.

## Context

`metawsm` needs write-capable credentials for:
1. pushing branch commits,
2. creating/updating pull requests,
3. optionally adding labels/reviewers.

Environments vary: local dev machines, sandboxed runners, and possibly CI-hosted automation.

## Proposals

### Proposal A: Reuse local user auth (gh + git credential helper)

How it works:
1. `metawsm` runs `gh auth status` and uses the logged-in user token.
2. `git push` uses the machine's configured credential helper/SSH key.
3. If auth missing, command fails with deterministic remediation guidance.

Pros:
1. Fastest implementation.
2. Minimal new infrastructure.
3. Works naturally for operator-driven local workflows.

Cons:
1. Depends on local machine state.
2. Harder to standardize across environments.
3. Token scope may be broader than required.

Best for:
- V1 local rollout.

### Proposal B: Fine-grained GitHub App installation token broker

How it works:
1. `metawsm` requests short-lived installation tokens from an internal broker.
2. Token is scoped to specific repos and permissions.
3. `metawsm` uses token for push/PR and discards it after use.

Pros:
1. Strong least-privilege story.
2. Centralized revocation/audit.
3. Better for hosted/sandbox automation.

Cons:
1. Highest implementation complexity.
2. Requires operating a secure token broker/service.
3. Additional operational dependencies.

Best for:
- long-term production-grade automation.

### Proposal C: Per-run ephemeral PAT injected by operator/CI

How it works:
1. Operator or CI injects short-lived token as env var (for example `METAWSM_GITHUB_TOKEN`).
2. `metawsm` configures `gh`/git for the run scope only.
3. Token is removed after run completion.

Pros:
1. Simpler than GitHub App broker.
2. Better reproducibility than local desktop auth.
3. Works in constrained sandboxes if env injection is allowed.

Cons:
1. Token distribution still needs secure handling.
2. Manual friction unless integrated with CI/secret manager.
3. Revocation/auditing weaker than app-based broker.

Best for:
- intermediate phase between local auth and full broker.

## Recommended rollout

1. Phase 1 (now): Proposal A for local assist-mode workflows.
2. Phase 2: Add Proposal C support for CI/sandbox runs.
3. Phase 3: Move to Proposal B for high-scale or multi-team production use.

## Decision for METAWSM-009 V1

Selected now: **Proposal A** (run as current local operator identity).

## Guardrails (independent of credential mode)

1. Scope checks before action:
- repo in allowlist,
- branch naming policy satisfied,
- all required validations passed.
2. Action logging:
- record credential mode (not secret value),
- record actor identity (`gh api user` / git user),
- record push ref and PR URL.
3. Secret safety:
- never print tokens,
- scrub env vars from diagnostics,
- reject commands that would echo credentials.
4. Human merge policy:
- `metawsm` may push and open PRs,
- merges remain human-only.

## Command/UX suggestions

1. `metawsm auth check --ticket <TICKET>`
- reports credential readiness for push/PR.
2. `metawsm pr --dry-run`
- prints whether auth is valid and which mode is active.
3. clear remediation messages, for example:
- "No GitHub auth found. Run `gh auth login` or set `METAWSM_GITHUB_TOKEN`."

## Usage examples

### Example: local machine (Proposal A)

1. Operator ensures `gh auth status` succeeds.
2. Run:
- `metawsm commit --ticket METAWSM-009`
- `metawsm pr --ticket METAWSM-009`

### Example: CI run (Proposal C)

1. CI injects `METAWSM_GITHUB_TOKEN` for run duration.
2. `metawsm` uses token to push + open PR.
3. Token expires or is removed after job.

## Recommendation summary

If optimizing for immediate progress: start with Proposal A.
If optimizing for secure automation roadmap: design interfaces now to support A and C, then adopt B without CLI breakage.
