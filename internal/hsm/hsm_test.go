package hsm

import (
	"testing"

	"metawsm/internal/model"
)

func TestRunTransitions(t *testing.T) {
	if !CanTransitionRun(model.RunStatusPlanning, model.RunStatusPaused) {
		t.Fatalf("expected planning -> paused transition to be allowed")
	}
	if CanTransitionRun(model.RunStatusCreated, model.RunStatusComplete) {
		t.Fatalf("expected created -> completed transition to be disallowed")
	}
}

func TestStepTransitions(t *testing.T) {
	if !CanTransitionStep(model.StepStatusFailed, model.StepStatusRunning) {
		t.Fatalf("expected failed -> running step transition to be allowed")
	}
	if CanTransitionStep(model.StepStatusDone, model.StepStatusRunning) {
		t.Fatalf("expected done -> running step transition to be disallowed")
	}
}

func TestAgentTransitions(t *testing.T) {
	if !CanTransitionAgent(model.AgentStatusRunning, model.AgentStatusStalled) {
		t.Fatalf("expected running -> stalled agent transition to be allowed")
	}
	if CanTransitionAgent(model.AgentStatusStopped, model.AgentStatusRunning) {
		t.Fatalf("expected stopped -> running agent transition to be disallowed")
	}
}
