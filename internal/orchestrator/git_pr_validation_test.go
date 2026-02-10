package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"metawsm/internal/policy"
)

func TestTicketWorkflowCheckPassesWithoutDeclaredContract(t *testing.T) {
	ticket := "METAWSM-TICKET-WORKFLOW-20260210"
	docRootPath := writeTicketWorkflowFixture(t, ticket, false, false)

	check := gitPRTicketWorkflowCheck{}
	result, err := check.Run(t.Context(), policy.Default(), gitPRValidationInput{
		Operation:   gitPRValidationOperationCommit,
		Ticket:      ticket,
		DocRootPath: docRootPath,
	})
	if err != nil {
		t.Fatalf("run ticket workflow check: %v", err)
	}
	if result.Status != gitPRValidationStatusPassed {
		t.Fatalf("expected passed status when no contract is declared, got %q detail=%q", result.Status, result.Detail)
	}
}

func TestTicketWorkflowCheckFailsWhenDeclaredContractIsIncomplete(t *testing.T) {
	ticket := "METAWSM-TICKET-WORKFLOW-20260210"
	docRootPath := writeTicketWorkflowFixture(t, ticket, true, false)

	check := gitPRTicketWorkflowCheck{}
	result, err := check.Run(t.Context(), policy.Default(), gitPRValidationInput{
		Operation:   gitPRValidationOperationCommit,
		Ticket:      ticket,
		DocRootPath: docRootPath,
	})
	if err != nil {
		t.Fatalf("run ticket workflow check: %v", err)
	}
	if result.Status != gitPRValidationStatusFailed {
		t.Fatalf("expected failed status for incomplete workflow artifacts, got %q detail=%q", result.Status, result.Detail)
	}
	if !strings.Contains(result.Detail, "analysis") {
		t.Fatalf("expected detail to mention missing analysis artifact, got %q", result.Detail)
	}
}

func TestTicketWorkflowCheckPassesWhenDeclaredContractIsSatisfied(t *testing.T) {
	ticket := "METAWSM-TICKET-WORKFLOW-20260210"
	docRootPath := writeTicketWorkflowFixture(t, ticket, true, true)

	check := gitPRTicketWorkflowCheck{}
	result, err := check.Run(t.Context(), policy.Default(), gitPRValidationInput{
		Operation:   gitPRValidationOperationPR,
		Ticket:      ticket,
		DocRootPath: docRootPath,
	})
	if err != nil {
		t.Fatalf("run ticket workflow check: %v", err)
	}
	if result.Status != gitPRValidationStatusPassed {
		t.Fatalf("expected passed status for complete workflow artifacts, got %q detail=%q", result.Status, result.Detail)
	}
}

func writeTicketWorkflowFixture(t *testing.T, ticket string, declareContract bool, complete bool) string {
	t.Helper()
	docRootPath := t.TempDir()
	ticketPath := filepath.Join(docRootPath, "ttmp", "2026", "02", "10", strings.ToLower(ticket)+"--fixture")
	if err := os.MkdirAll(filepath.Join(ticketPath, "reference"), 0o755); err != nil {
		t.Fatalf("mkdir reference dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ticketPath, "design-doc"), 0o755); err != nil {
		t.Fatalf("mkdir design-doc dir: %v", err)
	}

	feedback := "# Operator Feedback\n\n## 2026-02-10T07:06:49-08:00\n\nNo staged workflow required.\n"
	if declareContract {
		feedback = "# Operator Feedback\n\n## 2026-02-10T07:06:49-08:00\n\nRequired workflow: Step 1 use docmgr to create codebase relevance analysis and ask for feedback; if relevant areas are unclear, ask first. Step 2 create implementation plan, request feedback, and iterate until approval. Step 3 break approved plan into tasks, then implement while maintaining diary updates and incremental commits. Step 4 open PR and iterate on review feedback until done.\n"
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "reference", "99-operator-feedback.md"), []byte(feedback), 0o644); err != nil {
		t.Fatalf("write operator feedback doc: %v", err)
	}

	if complete {
		analysis := "# Codebase Relevance Analysis\n\n## Relevant Areas\n\n- `internal/orchestrator/service.go`\n\n## Feedback Request\n\nPlease confirm these areas.\n\n## Feedback Response\n\nApproved.\n"
		if err := os.WriteFile(filepath.Join(ticketPath, "reference", "100-codebase-relevance-analysis.md"), []byte(analysis), 0o644); err != nil {
			t.Fatalf("write analysis doc: %v", err)
		}

		plan := "# Implementation Plan\n\n## Plan\n\n1. Add workflow validation.\n\n## Feedback Request\n\nPlease review this plan.\n\n## Feedback Response\n\nApproved.\n"
		if err := os.WriteFile(filepath.Join(ticketPath, "design-doc", "01-implementation-plan.md"), []byte(plan), 0o644); err != nil {
			t.Fatalf("write plan doc: %v", err)
		}

		diary := "# Diary\n\n## Step 1: Build workflow gate\n\n### Prompt Context\n\n**User prompt (verbatim):** \"test prompt\"\n\n**Assistant interpretation:** implement a workflow gate.\n\n**Inferred user intent:** enforce staged workflow.\n"
		if err := os.WriteFile(filepath.Join(ticketPath, "reference", "101-diary.md"), []byte(diary), 0o644); err != nil {
			t.Fatalf("write diary doc: %v", err)
		}

		tasks := "# Tasks\n\n## TODO\n\n- [x] Add workflow check\n- [x] Add tests\n"
		if err := os.WriteFile(filepath.Join(ticketPath, "tasks.md"), []byte(tasks), 0o644); err != nil {
			t.Fatalf("write tasks doc: %v", err)
		}

		changelog := "# Changelog\n\n## 2026-02-10\n\n- Step 1: Added staged workflow check.\n"
		if err := os.WriteFile(filepath.Join(ticketPath, "changelog.md"), []byte(changelog), 0o644); err != nil {
			t.Fatalf("write changelog doc: %v", err)
		}
	}
	return docRootPath
}
