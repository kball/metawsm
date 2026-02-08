package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"metawsm/internal/policy"
)

type gitPRValidationOperation string

const (
	gitPRValidationOperationCommit gitPRValidationOperation = "commit"
	gitPRValidationOperationPR     gitPRValidationOperation = "pr"
)

const (
	gitPRValidationStatusPassed  = "passed"
	gitPRValidationStatusFailed  = "failed"
	gitPRValidationStatusSkipped = "skipped"
)

type gitPRValidationInput struct {
	Operation     gitPRValidationOperation
	RunID         string
	Ticket        string
	WorkspaceName string
	Repo          string
	RepoPath      string
	BaseBranch    string
	HeadBranch    string
}

type gitPRValidationCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type gitPRValidationReport struct {
	Operation      string                       `json:"operation"`
	RequireAll     bool                         `json:"require_all"`
	RequiredChecks []string                     `json:"required_checks"`
	Results        []gitPRValidationCheckResult `json:"results"`
	Passed         bool                         `json:"passed"`
	EvaluatedAt    string                       `json:"evaluated_at"`
}

type gitPRValidationCheck interface {
	Name() string
	Supports(op gitPRValidationOperation) bool
	Run(ctx context.Context, cfg policy.Config, input gitPRValidationInput) (gitPRValidationCheckResult, error)
}

func runGitPRValidations(ctx context.Context, cfg policy.Config, input gitPRValidationInput) (gitPRValidationReport, error) {
	requiredChecks := normalizeTokens(cfg.GitPR.RequiredChecks)
	report := gitPRValidationReport{
		Operation:      string(input.Operation),
		RequireAll:     cfg.GitPR.RequireAll,
		RequiredChecks: append([]string(nil), requiredChecks...),
		Results:        []gitPRValidationCheckResult{},
		Passed:         true,
		EvaluatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if len(requiredChecks) == 0 {
		return report, nil
	}

	checks := defaultGitPRValidationChecks()
	applicableChecks := 0
	passedChecks := 0
	failedChecks := 0

	for _, configuredName := range requiredChecks {
		name := strings.TrimSpace(strings.ToLower(configuredName))
		check, ok := checks[name]
		if !ok {
			return report, fmt.Errorf("required check %q is not supported", configuredName)
		}
		if !check.Supports(input.Operation) {
			report.Results = append(report.Results, gitPRValidationCheckResult{
				Name:   name,
				Status: gitPRValidationStatusSkipped,
				Detail: fmt.Sprintf("check not applicable for %s workflow", input.Operation),
			})
			continue
		}

		result, err := check.Run(ctx, cfg, input)
		if err != nil {
			return report, fmt.Errorf("run required check %q: %w", name, err)
		}
		result.Name = name
		switch result.Status {
		case gitPRValidationStatusPassed:
			applicableChecks++
			passedChecks++
		case gitPRValidationStatusFailed:
			applicableChecks++
			failedChecks++
		case gitPRValidationStatusSkipped:
		default:
			return report, fmt.Errorf("required check %q returned unknown status %q", name, result.Status)
		}
		report.Results = append(report.Results, result)
	}

	switch {
	case applicableChecks == 0:
		report.Passed = true
	case cfg.GitPR.RequireAll:
		report.Passed = failedChecks == 0 && passedChecks == applicableChecks
	default:
		report.Passed = passedChecks > 0
	}
	if report.Passed {
		return report, nil
	}
	return report, fmt.Errorf("required checks failed: %s", summarizeFailedGitPRChecks(report.Results, cfg.GitPR.RequireAll))
}

func marshalGitPRValidationReport(report gitPRValidationReport) string {
	encoded, err := json.Marshal(report)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func summarizeFailedGitPRChecks(results []gitPRValidationCheckResult, requireAll bool) string {
	failures := []string{}
	for _, result := range results {
		if result.Status != gitPRValidationStatusFailed {
			continue
		}
		message := result.Name
		if strings.TrimSpace(result.Detail) != "" {
			message = fmt.Sprintf("%s (%s)", result.Name, strings.TrimSpace(result.Detail))
		}
		failures = append(failures, message)
	}
	if len(failures) > 0 {
		return strings.Join(failures, "; ")
	}
	if requireAll {
		return "one or more required checks did not pass"
	}
	return "no required checks passed"
}

func defaultGitPRValidationChecks() map[string]gitPRValidationCheck {
	return map[string]gitPRValidationCheck{
		"tests":           gitPRTestsCheck{},
		"forbidden_files": gitPRForbiddenFilesCheck{},
		"clean_tree":      gitPRCleanTreeCheck{},
	}
}

type gitPRTestsCheck struct{}

func (gitPRTestsCheck) Name() string { return "tests" }

func (gitPRTestsCheck) Supports(op gitPRValidationOperation) bool {
	return op == gitPRValidationOperationCommit || op == gitPRValidationOperationPR
}

func (gitPRTestsCheck) Run(ctx context.Context, cfg policy.Config, input gitPRValidationInput) (gitPRValidationCheckResult, error) {
	commands := normalizeTokens(cfg.GitPR.TestCommands)
	if len(commands) == 0 {
		return gitPRValidationCheckResult{
			Status: gitPRValidationStatusPassed,
			Detail: "no test commands configured",
		}, nil
	}

	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		if _, err := runCommandInDir(ctx, input.RepoPath, "zsh", "-lc", command); err != nil {
			return gitPRValidationCheckResult{
				Status: gitPRValidationStatusFailed,
				Detail: fmt.Sprintf("command %q failed: %v", command, err),
			}, nil
		}
	}

	return gitPRValidationCheckResult{
		Status: gitPRValidationStatusPassed,
		Detail: fmt.Sprintf("all %d test command(s) passed", len(commands)),
	}, nil
}

type gitPRForbiddenFilesCheck struct{}

func (gitPRForbiddenFilesCheck) Name() string { return "forbidden_files" }

func (gitPRForbiddenFilesCheck) Supports(op gitPRValidationOperation) bool {
	return op == gitPRValidationOperationCommit || op == gitPRValidationOperationPR
}

func (gitPRForbiddenFilesCheck) Run(ctx context.Context, cfg policy.Config, input gitPRValidationInput) (gitPRValidationCheckResult, error) {
	patterns := normalizeTokens(cfg.GitPR.ForbiddenPatterns)
	if len(patterns) == 0 {
		return gitPRValidationCheckResult{
			Status: gitPRValidationStatusPassed,
			Detail: "no forbidden file patterns configured",
		}, nil
	}

	lines, err := gitStatusShortLines(ctx, input.RepoPath)
	if err != nil {
		return gitPRValidationCheckResult{}, err
	}
	changedPaths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := parseGitStatusPath(line)
		if path == "" {
			continue
		}
		changedPaths = append(changedPaths, path)
	}
	if len(changedPaths) == 0 {
		return gitPRValidationCheckResult{
			Status: gitPRValidationStatusPassed,
			Detail: "no changed files detected",
		}, nil
	}

	matches := findForbiddenPathMatches(changedPaths, patterns)
	if len(matches) > 0 {
		return gitPRValidationCheckResult{
			Status: gitPRValidationStatusFailed,
			Detail: fmt.Sprintf("forbidden files detected: %s", strings.Join(matches, ", ")),
		}, nil
	}

	return gitPRValidationCheckResult{
		Status: gitPRValidationStatusPassed,
		Detail: fmt.Sprintf("checked %d changed file(s); no forbidden matches", len(changedPaths)),
	}, nil
}

type gitPRCleanTreeCheck struct{}

func (gitPRCleanTreeCheck) Name() string { return "clean_tree" }

func (gitPRCleanTreeCheck) Supports(op gitPRValidationOperation) bool {
	return op == gitPRValidationOperationPR
}

func (gitPRCleanTreeCheck) Run(ctx context.Context, _ policy.Config, input gitPRValidationInput) (gitPRValidationCheckResult, error) {
	dirty, err := hasDirtyGitState(ctx, input.RepoPath)
	if err != nil {
		return gitPRValidationCheckResult{}, err
	}
	if !dirty {
		return gitPRValidationCheckResult{
			Status: gitPRValidationStatusPassed,
			Detail: "working tree is clean",
		}, nil
	}
	lines, err := gitStatusShortLines(ctx, input.RepoPath)
	if err != nil {
		return gitPRValidationCheckResult{}, err
	}
	return gitPRValidationCheckResult{
		Status: gitPRValidationStatusFailed,
		Detail: fmt.Sprintf("working tree is dirty: %s", summarizeStatusLines(lines, 5)),
	}, nil
}

func summarizeStatusLines(lines []string, limit int) string {
	lines = append([]string(nil), lines...)
	sort.Strings(lines)
	if len(lines) == 0 {
		return "unknown changes"
	}
	if limit <= 0 || len(lines) <= limit {
		return strings.Join(lines, ", ")
	}
	return strings.Join(lines[:limit], ", ") + fmt.Sprintf(" (and %d more)", len(lines)-limit)
}

func findForbiddenPathMatches(paths []string, patterns []string) []string {
	matches := []string{}
	for _, path := range paths {
		if matchesForbiddenPattern(path, patterns) {
			matches = append(matches, path)
		}
	}
	sort.Strings(matches)
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		unique = append(unique, match)
	}
	return unique
}

func matchesForbiddenPattern(path string, patterns []string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	base := filepath.Base(path)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if ok, err := filepath.Match(pattern, path); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(pattern, base); err == nil && ok {
			return true
		}
		prefix := strings.TrimSuffix(pattern, "/")
		if prefix != "" && (path == prefix || strings.HasPrefix(path, prefix+"/")) {
			return true
		}
	}
	return false
}
