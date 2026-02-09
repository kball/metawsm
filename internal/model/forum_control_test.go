package model

import "testing"

func TestForumControlPayloadV1Validate(t *testing.T) {
	valid := ForumControlPayloadV1{
		SchemaVersion: ForumControlSchemaVersion1,
		ControlType:   ForumControlTypeGuidanceRequest,
		RunID:         "run-1",
		AgentName:     "agent",
		Question:      "Need guidance",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid payload, got error: %v", err)
	}

	cases := []ForumControlPayloadV1{
		{SchemaVersion: 2, ControlType: ForumControlTypeGuidanceRequest, RunID: "run-1", AgentName: "agent", Question: "q"},
		{SchemaVersion: ForumControlSchemaVersion1, ControlType: ForumControlTypeGuidanceRequest, RunID: "", AgentName: "agent", Question: "q"},
		{SchemaVersion: ForumControlSchemaVersion1, ControlType: ForumControlTypeGuidanceAnswer, RunID: "run-1", AgentName: "agent", Answer: ""},
		{SchemaVersion: ForumControlSchemaVersion1, ControlType: ForumControlTypeValidation, RunID: "run-1", AgentName: "agent", Status: "unknown", DoneCriteria: "done"},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d expected validation error", i)
		}
	}
}
