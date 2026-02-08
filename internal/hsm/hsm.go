package hsm

import "metawsm/internal/model"

var runTransitions = map[model.RunStatus]map[model.RunStatus]bool{
	model.RunStatusCreated: {
		model.RunStatusPlanning: true,
	},
	model.RunStatusPlanning: {
		model.RunStatusRunning:  true,
		model.RunStatusPaused:   true,
		model.RunStatusFailed:   true,
		model.RunStatusStopping: true,
	},
	model.RunStatusRunning: {
		model.RunStatusAwaitingGuidance: true,
		model.RunStatusPaused:           true,
		model.RunStatusFailed:           true,
		model.RunStatusComplete:         true,
		model.RunStatusStopping:         true,
	},
	model.RunStatusAwaitingGuidance: {
		model.RunStatusRunning:  true,
		model.RunStatusFailed:   true,
		model.RunStatusStopping: true,
	},
	model.RunStatusPaused: {
		model.RunStatusRunning:  true,
		model.RunStatusStopping: true,
	},
	model.RunStatusFailed: {
		model.RunStatusRunning:  true,
		model.RunStatusStopping: true,
	},
	model.RunStatusStopping: {
		model.RunStatusStopped: true,
	},
	model.RunStatusStopped: {
		model.RunStatusRunning: true,
	},
	model.RunStatusComplete: {
		model.RunStatusClosing: true,
		model.RunStatusRunning: true,
	},
	model.RunStatusClosing: {
		model.RunStatusClosed: true,
		model.RunStatusFailed: true,
	},
}

var stepTransitions = map[model.StepStatus]map[model.StepStatus]bool{
	model.StepStatusPending: {
		model.StepStatusRunning: true,
		model.StepStatusSkipped: true,
	},
	model.StepStatusRunning: {
		model.StepStatusDone:   true,
		model.StepStatusFailed: true,
	},
	model.StepStatusFailed: {
		model.StepStatusRunning: true,
		model.StepStatusSkipped: true,
	},
}

var agentTransitions = map[model.AgentStatus]map[model.AgentStatus]bool{
	model.AgentStatusPending: {
		model.AgentStatusRunning: true,
		model.AgentStatusStopped: true,
	},
	model.AgentStatusRunning: {
		model.AgentStatusIdle:     true,
		model.AgentStatusStalled:  true,
		model.AgentStatusDead:     true,
		model.AgentStatusStopping: true,
		model.AgentStatusFailed:   true,
	},
	model.AgentStatusIdle: {
		model.AgentStatusRunning:  true,
		model.AgentStatusStalled:  true,
		model.AgentStatusDead:     true,
		model.AgentStatusStopping: true,
	},
	model.AgentStatusStalled: {
		model.AgentStatusRunning:  true,
		model.AgentStatusDead:     true,
		model.AgentStatusStopping: true,
	},
	model.AgentStatusStopping: {
		model.AgentStatusStopped: true,
	},
}

func CanTransitionRun(from model.RunStatus, to model.RunStatus) bool {
	if from == to {
		return true
	}
	return runTransitions[from][to]
}

func CanTransitionStep(from model.StepStatus, to model.StepStatus) bool {
	if from == to {
		return true
	}
	return stepTransitions[from][to]
}

func CanTransitionAgent(from model.AgentStatus, to model.AgentStatus) bool {
	if from == to {
		return true
	}
	return agentTransitions[from][to]
}
