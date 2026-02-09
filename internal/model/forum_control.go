package model

import (
	"fmt"
	"strings"
	"time"
)

type ForumControlType string

const (
	ForumControlTypeGuidanceRequest ForumControlType = "guidance_request"
	ForumControlTypeGuidanceAnswer  ForumControlType = "guidance_answer"
	ForumControlTypeCompletion      ForumControlType = "completion"
	ForumControlTypeValidation      ForumControlType = "validation"
)

const ForumControlSchemaVersion1 = 1

type ForumControlPayloadV1 struct {
	SchemaVersion int              `json:"schema_version"`
	ControlType   ForumControlType `json:"control_type"`
	RunID         string           `json:"run_id"`
	AgentName     string           `json:"agent_name"`
	Question      string           `json:"question,omitempty"`
	Context       string           `json:"context,omitempty"`
	Answer        string           `json:"answer,omitempty"`
	Summary       string           `json:"summary,omitempty"`
	Status        string           `json:"status,omitempty"`
	DoneCriteria  string           `json:"done_criteria,omitempty"`
}

type ForumControlThread struct {
	RunID     string    `json:"run_id"`
	AgentName string    `json:"agent_name"`
	Ticket    string    `json:"ticket"`
	ThreadID  string    `json:"thread_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p ForumControlPayloadV1) Validate() error {
	if p.SchemaVersion != ForumControlSchemaVersion1 {
		return fmt.Errorf("forum control schema_version must be %d", ForumControlSchemaVersion1)
	}
	if strings.TrimSpace(p.RunID) == "" {
		return fmt.Errorf("forum control run_id is required")
	}
	if strings.TrimSpace(p.AgentName) == "" {
		return fmt.Errorf("forum control agent_name is required")
	}

	switch p.ControlType {
	case ForumControlTypeGuidanceRequest:
		if strings.TrimSpace(p.Question) == "" {
			return fmt.Errorf("forum control guidance_request requires question")
		}
	case ForumControlTypeGuidanceAnswer:
		if strings.TrimSpace(p.Answer) == "" {
			return fmt.Errorf("forum control guidance_answer requires answer")
		}
	case ForumControlTypeCompletion:
		// summary is optional
	case ForumControlTypeValidation:
		status := strings.TrimSpace(strings.ToLower(p.Status))
		if status != "passed" && status != "failed" {
			return fmt.Errorf("forum control validation status must be passed|failed")
		}
		if strings.TrimSpace(p.DoneCriteria) == "" {
			return fmt.Errorf("forum control validation requires done_criteria")
		}
	default:
		return fmt.Errorf("forum control type must be guidance_request|guidance_answer|completion|validation")
	}
	return nil
}
