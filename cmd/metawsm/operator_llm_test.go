package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseOperatorLLMResponse(t *testing.T) {
	output := "debug line\n{\"intent\":\"auto_restart\",\"target_run\":\"run-1\",\"reason\":\"stalled\",\"confidence\":0.9,\"needs_human\":false}\n"
	resp, err := parseOperatorLLMResponse(output)
	if err != nil {
		t.Fatalf("parse operator response: %v", err)
	}
	if resp.Intent != operatorIntentAutoRestart {
		t.Fatalf("expected auto_restart intent, got %q", resp.Intent)
	}
	if resp.TargetRun != "run-1" {
		t.Fatalf("expected target run run-1, got %q", resp.TargetRun)
	}
}

func TestMergeOperatorDecisionsAssistNeverExecutes(t *testing.T) {
	rule := operatorRuleDecision{Intent: operatorIntentAutoRestart, Reason: "rule restart"}
	llm := &operatorLLMResponse{Intent: operatorIntentEscalateBlocked, Reason: "needs human"}
	merged := mergeOperatorDecisions("assist", rule, llm)
	if merged.Execute {
		t.Fatalf("expected assist mode not to execute")
	}
	if merged.Intent != operatorIntentAutoRestart {
		t.Fatalf("expected rule intent to remain, got %q", merged.Intent)
	}
	if !strings.Contains(merged.Reason, "llm_suggested") {
		t.Fatalf("expected llm suggestion annotation, got %q", merged.Reason)
	}
}

func TestMergeOperatorDecisionsAutoAllowsLLMEscalateWhenRuleNoop(t *testing.T) {
	rule := operatorRuleDecision{Intent: operatorIntentNoop, Reason: "no rule action"}
	llm := &operatorLLMResponse{Intent: operatorIntentEscalateBlocked, Reason: "missing credential"}
	merged := mergeOperatorDecisions("auto", rule, llm)
	if merged.Intent != operatorIntentEscalateBlocked {
		t.Fatalf("expected llm escalate blocked, got %q", merged.Intent)
	}
	if merged.Source != "llm" {
		t.Fatalf("expected llm source, got %q", merged.Source)
	}
	if merged.Execute {
		t.Fatalf("expected escalation not to execute actions")
	}
}

func TestCodexCLIAdapterValidateAndFallback(t *testing.T) {
	var capturedArgs []string
	adapter := newCodexCLIAdapter("codex", "", 200, time.Second, func(ctx context.Context, name string, args []string, stdin string) (string, string, error) {
		capturedArgs = append([]string(nil), args...)
		return "{\"intent\":\"noop\",\"reason\":\"ok\"}", "", nil
	})
	resp, err := adapter.Propose(context.Background(), operatorLLMRequest{RunID: "run-1"})
	if err != nil {
		t.Fatalf("adapter propose: %v", err)
	}
	if resp.Intent != operatorIntentNoop {
		t.Fatalf("expected noop intent, got %q", resp.Intent)
	}
	if resp.TargetRun != "run-1" {
		t.Fatalf("expected target run fallback to request run id, got %q", resp.TargetRun)
	}
	for i := range capturedArgs {
		if capturedArgs[i] == "--max-tokens" {
			t.Fatalf("did not expect unsupported --max-tokens arg: %v", capturedArgs)
		}
	}
}

func TestCodexCLIAdapterPropagatesRunnerError(t *testing.T) {
	adapter := newCodexCLIAdapter("codex", "", 200, time.Second, func(ctx context.Context, name string, args []string, stdin string) (string, string, error) {
		return "", "boom", errors.New("exit 1")
	})
	_, err := adapter.Propose(context.Background(), operatorLLMRequest{RunID: "run-1"})
	if err == nil {
		t.Fatalf("expected propose error")
	}
	if !strings.Contains(err.Error(), "codex exec failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
