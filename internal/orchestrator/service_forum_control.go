package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"metawsm/internal/model"
)

type forumControlAgentState struct {
	RunID                   string
	AgentName               string
	WorkspaceName           string
	ThreadID                string
	Ticket                  string
	PendingGuidance         bool
	PendingGuidanceQuestion string
	CompletionSignaled      bool
	ValidationStatus        string
	ValidationDoneCriteria  string
}

func parseForumControlPayload(body string) (model.ForumControlPayloadV1, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return model.ForumControlPayloadV1{}, false
	}
	var payload model.ForumControlPayloadV1
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return model.ForumControlPayloadV1{}, false
	}
	if err := payload.Validate(); err != nil {
		return model.ForumControlPayloadV1{}, false
	}
	return payload, true
}

func (s *Service) forumControlStatesForRun(runID string, agents []model.AgentRecord) (map[string]forumControlAgentState, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	stateByAgent := make(map[string]forumControlAgentState, len(agents))
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		if name == "" {
			continue
		}
		stateByAgent[name] = forumControlAgentState{
			RunID:         runID,
			AgentName:     name,
			WorkspaceName: strings.TrimSpace(agent.WorkspaceName),
		}
	}

	mappings, err := s.store.ListForumControlThreads(runID)
	if err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		agentName := strings.TrimSpace(mapping.AgentName)
		state, ok := stateByAgent[agentName]
		if !ok {
			state = forumControlAgentState{
				RunID:     runID,
				AgentName: agentName,
			}
		}
		state.ThreadID = strings.TrimSpace(mapping.ThreadID)
		state.Ticket = strings.TrimSpace(mapping.Ticket)

		posts, err := s.store.ListForumPosts(state.ThreadID, 1000)
		if err != nil {
			return nil, err
		}
		for _, post := range posts {
			payload, ok := parseForumControlPayload(post.Body)
			if !ok {
				continue
			}
			if payload.RunID != runID || payload.AgentName != state.AgentName {
				continue
			}
			switch payload.ControlType {
			case model.ForumControlTypeGuidanceRequest:
				state.PendingGuidance = true
				state.PendingGuidanceQuestion = strings.TrimSpace(payload.Question)
			case model.ForumControlTypeGuidanceAnswer:
				state.PendingGuidance = false
				state.PendingGuidanceQuestion = ""
			case model.ForumControlTypeCompletion:
				state.CompletionSignaled = true
			case model.ForumControlTypeValidation:
				state.ValidationStatus = strings.TrimSpace(strings.ToLower(payload.Status))
				state.ValidationDoneCriteria = strings.TrimSpace(payload.DoneCriteria)
			}
		}
		stateByAgent[agentName] = state
	}
	return stateByAgent, nil
}
