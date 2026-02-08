package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"metawsm/internal/docfederation"
	"metawsm/internal/model"
	"metawsm/internal/policy"
)

func TestCollectBootstrapBriefNonInteractiveRequiresAllFields(t *testing.T) {
	_, err := collectBootstrapBrief(strings.NewReader(""), &bytes.Buffer{}, false, "METAWSM-002", model.RunBrief{
		Ticket: "METAWSM-002",
		Goal:   "Implement bootstrap",
	})
	if err == nil {
		t.Fatalf("expected error for missing non-interactive fields")
	}
	if !strings.Contains(err.Error(), "missing required bootstrap intake field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectBootstrapBriefInteractivePrompts(t *testing.T) {
	input := strings.NewReader("Goal answer\nScope answer\nDone answer\nConstraints answer\ndefault\n")
	var output bytes.Buffer
	brief, err := collectBootstrapBrief(input, &output, true, "METAWSM-002", model.RunBrief{Ticket: "METAWSM-002"})
	if err != nil {
		t.Fatalf("collect bootstrap brief: %v", err)
	}
	if brief.Goal != "Goal answer" {
		t.Fatalf("expected goal answer, got %q", brief.Goal)
	}
	if brief.Scope != "Scope answer" {
		t.Fatalf("expected scope answer, got %q", brief.Scope)
	}
	if brief.DoneCriteria != "Done answer" {
		t.Fatalf("expected done criteria answer, got %q", brief.DoneCriteria)
	}
	if brief.Constraints != "Constraints answer" {
		t.Fatalf("expected constraints answer, got %q", brief.Constraints)
	}
	if brief.MergeIntent != "default" {
		t.Fatalf("expected merge intent default, got %q", brief.MergeIntent)
	}
	if len(brief.QA) != 5 {
		t.Fatalf("expected 5 QA entries, got %d", len(brief.QA))
	}
}

func TestCollectBootstrapBriefNonInteractiveWithSeed(t *testing.T) {
	brief, err := collectBootstrapBrief(strings.NewReader(""), &bytes.Buffer{}, false, "METAWSM-002", model.RunBrief{
		Ticket:       "METAWSM-002",
		Goal:         "Goal",
		Scope:        "Scope",
		DoneCriteria: "Done",
		Constraints:  "Constraints",
		MergeIntent:  "default",
	})
	if err != nil {
		t.Fatalf("collect bootstrap brief with seed: %v", err)
	}
	if len(brief.QA) != 5 {
		t.Fatalf("expected 5 QA entries, got %d", len(brief.QA))
	}
}

func TestExtractJSONArray(t *testing.T) {
	payload := []map[string]any{
		{"ticket": "METAWSM-001"},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	out := []byte(`{"level":"debug"}\n` + string(b) + "\n")
	extracted, ok := extractJSONArray(out)
	if !ok {
		t.Fatalf("expected to extract json array")
	}
	var parsed []map[string]any
	if err := json.Unmarshal(extracted, &parsed); err != nil {
		t.Fatalf("unmarshal extracted json: %v", err)
	}
	if len(parsed) != 1 || parsed[0]["ticket"] != "METAWSM-001" {
		t.Fatalf("unexpected parsed payload: %#v", parsed)
	}
}

func TestRequireRunSelector(t *testing.T) {
	_, _, err := requireRunSelector("", "")
	if err == nil {
		t.Fatalf("expected selector error")
	}

	runID, ticket, err := requireRunSelector(" run-1 ", "")
	if err != nil {
		t.Fatalf("selector with run id: %v", err)
	}
	if runID != "run-1" || ticket != "" {
		t.Fatalf("unexpected selector result run=%q ticket=%q", runID, ticket)
	}

	runID, ticket, err = requireRunSelector("", " METAWSM-003 ")
	if err != nil {
		t.Fatalf("selector with ticket: %v", err)
	}
	if runID != "" || ticket != "METAWSM-003" {
		t.Fatalf("unexpected selector result run=%q ticket=%q", runID, ticket)
	}
}

func TestFederationEndpointsFromPolicyWorkspaceFirst(t *testing.T) {
	cfg := policy.Default()
	cfg.Docs.API.WorkspaceEndpoints = []policy.DocAPIEndpoint{
		{
			Name:      "ws-metawsm",
			BaseURL:   "http://127.0.0.1:8787",
			WebURL:    "http://127.0.0.1:8787",
			Repo:      "metawsm",
			Workspace: "ws-001",
		},
	}
	cfg.Docs.API.RepoEndpoints = []policy.DocAPIEndpoint{
		{
			Name:    "repo-metawsm",
			BaseURL: "http://127.0.0.1:8790",
			WebURL:  "http://127.0.0.1:8790",
			Repo:    "metawsm",
		},
	}
	endpoints := federationEndpointsFromPolicy(cfg)
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].Kind != docfederation.EndpointKindWorkspace {
		t.Fatalf("expected workspace endpoint first, got %s", endpoints[0].Kind)
	}
	if endpoints[1].Kind != docfederation.EndpointKindRepo {
		t.Fatalf("expected repo endpoint second, got %s", endpoints[1].Kind)
	}
}

func TestSelectFederationEndpointsByName(t *testing.T) {
	endpoints := []docfederation.Endpoint{
		{Name: "repo-z", Kind: docfederation.EndpointKindRepo},
		{Name: "workspace-a", Kind: docfederation.EndpointKindWorkspace},
		{Name: "repo-b", Kind: docfederation.EndpointKindRepo},
	}
	selected := selectFederationEndpoints(endpoints, []string{"repo-b", "workspace-a"})
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected endpoints, got %d", len(selected))
	}
	if selected[0].Name != "repo-b" {
		t.Fatalf("expected sorted selection starting with repo-b, got %q", selected[0].Name)
	}
	if selected[1].Name != "workspace-a" {
		t.Fatalf("expected sorted selection ending with workspace-a, got %q", selected[1].Name)
	}
}
