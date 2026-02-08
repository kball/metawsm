package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type operatorIntent string

const (
	operatorIntentNoop             operatorIntent = "noop"
	operatorIntentEscalateGuidance operatorIntent = "escalate_guidance"
	operatorIntentEscalateBlocked  operatorIntent = "escalate_blocked"
	operatorIntentAutoRestart      operatorIntent = "auto_restart"
	operatorIntentAutoStopStale    operatorIntent = "auto_stop_stale"
)

type operatorRuleDecision struct {
	Intent operatorIntent
	Reason string
}

type operatorMergedDecision struct {
	Intent   operatorIntent
	Reason   string
	Source   string
	Execute  bool
	LLMReply *operatorLLMResponse
}

type operatorLLMRequest struct {
	RunID           string            `json:"run_id"`
	RunStatus       string            `json:"run_status"`
	Tickets         string            `json:"tickets"`
	HasGuidance     bool              `json:"has_guidance"`
	HasUnhealthy    bool              `json:"has_unhealthy"`
	RuleIntent      operatorIntent    `json:"rule_intent"`
	RuleReason      string            `json:"rule_reason"`
	UnhealthyAgents []watchAgentIssue `json:"unhealthy_agents"`
}

type operatorLLMResponse struct {
	Intent     operatorIntent `json:"intent"`
	TargetRun  string         `json:"target_run"`
	Reason     string         `json:"reason"`
	Confidence float64        `json:"confidence"`
	NeedsHuman bool           `json:"needs_human"`
}

type operatorLLMAdapter interface {
	Propose(ctx context.Context, req operatorLLMRequest) (operatorLLMResponse, error)
}

type operatorCommandRunner func(ctx context.Context, name string, args []string, stdin string) (stdout string, stderr string, err error)

type codexCLIAdapter struct {
	command   string
	model     string
	maxTokens int
	timeout   time.Duration
	runner    operatorCommandRunner
}

func newCodexCLIAdapter(command string, model string, maxTokens int, timeout time.Duration, runner operatorCommandRunner) operatorLLMAdapter {
	if runner == nil {
		runner = runOperatorCommand
	}
	return &codexCLIAdapter{
		command:   strings.TrimSpace(command),
		model:     strings.TrimSpace(model),
		maxTokens: maxTokens,
		timeout:   timeout,
		runner:    runner,
	}
}

func (c *codexCLIAdapter) Propose(ctx context.Context, req operatorLLMRequest) (operatorLLMResponse, error) {
	if c.command == "" {
		return operatorLLMResponse{}, fmt.Errorf("codex command is empty")
	}
	if c.timeout <= 0 {
		c.timeout = 30 * time.Second
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return operatorLLMResponse{}, fmt.Errorf("marshal operator llm request: %w", err)
	}
	prompt := strings.Join([]string{
		"You are an operator assistant for metawsm.",
		"Return a single JSON object with keys: intent, target_run, reason, confidence, needs_human.",
		"Allowed intents: noop, escalate_guidance, escalate_blocked, auto_restart, auto_stop_stale.",
		"Do not include markdown.",
		"Context JSON:",
		string(payload),
	}, "\n")

	args := []string{"exec"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	args = append(args, prompt)

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	stdout, stderr, err := c.runner(timeoutCtx, c.command, args, "")
	if err != nil {
		return operatorLLMResponse{}, fmt.Errorf("codex exec failed: %w: %s", err, strings.TrimSpace(stderr))
	}

	response, err := parseOperatorLLMResponse(stdout)
	if err != nil {
		return operatorLLMResponse{}, err
	}
	if response.TargetRun == "" {
		response.TargetRun = req.RunID
	}
	if !isOperatorIntentAllowlisted(response.Intent) {
		return operatorLLMResponse{}, fmt.Errorf("llm returned disallowed intent %q", response.Intent)
	}
	return response, nil
}

func runOperatorCommand(ctx context.Context, name string, args []string, stdin string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(stdin) != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func parseOperatorLLMResponse(output string) (operatorLLMResponse, error) {
	jsonObject, ok := extractJSONObject([]byte(output))
	if !ok {
		return operatorLLMResponse{}, fmt.Errorf("no JSON object found in llm output")
	}
	var response operatorLLMResponse
	if err := json.Unmarshal(jsonObject, &response); err != nil {
		return operatorLLMResponse{}, fmt.Errorf("parse llm response json: %w", err)
	}
	if response.Intent == "" {
		return operatorLLMResponse{}, fmt.Errorf("llm response missing intent")
	}
	return response, nil
}

func isOperatorIntentAllowlisted(intent operatorIntent) bool {
	switch intent {
	case operatorIntentNoop, operatorIntentEscalateGuidance, operatorIntentEscalateBlocked, operatorIntentAutoRestart, operatorIntentAutoStopStale:
		return true
	default:
		return false
	}
}

func mergeOperatorDecisions(mode string, rule operatorRuleDecision, llm *operatorLLMResponse) operatorMergedDecision {
	decision := operatorMergedDecision{
		Intent:  rule.Intent,
		Reason:  rule.Reason,
		Source:  "rule",
		Execute: rule.Intent == operatorIntentAutoRestart || rule.Intent == operatorIntentAutoStopStale,
	}

	if strings.EqualFold(mode, "assist") {
		decision.Execute = false
		if llm != nil && llm.Intent != "" && llm.Intent != rule.Intent {
			decision.Reason = strings.TrimSpace(decision.Reason + "; llm_suggested=" + string(llm.Intent) + " reason=" + llm.Reason)
			decision.LLMReply = llm
		}
		return decision
	}

	if strings.EqualFold(mode, "off") || llm == nil || llm.Intent == "" {
		if strings.EqualFold(mode, "off") {
			decision.Execute = decision.Execute
		}
		return decision
	}

	decision.LLMReply = llm
	if rule.Intent == operatorIntentEscalateGuidance {
		decision.Execute = false
		return decision
	}

	if strings.EqualFold(mode, "auto") {
		if rule.Intent == operatorIntentNoop && llm.Intent == operatorIntentEscalateBlocked {
			decision.Intent = llm.Intent
			decision.Reason = strings.TrimSpace(llm.Reason)
			decision.Source = "llm"
			decision.Execute = false
			return decision
		}
		if llm.Intent != rule.Intent {
			decision.Reason = strings.TrimSpace(decision.Reason + "; llm proposal rejected by policy gate")
		}
	}
	return decision
}

func extractJSONObject(output []byte) ([]byte, bool) {
	text := string(output)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end < start {
		return nil, false
	}
	for i := end; i > start; i-- {
		candidate := strings.TrimSpace(text[start : i+1])
		var tmp map[string]any
		if err := json.Unmarshal([]byte(candidate), &tmp); err == nil {
			return []byte(candidate), true
		}
	}
	return nil, false
}
