package docfederation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollectSnapshotsParsesTickets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspace/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"root":        "/tmp/ttmp",
				"repoRoot":    "/tmp/repo",
				"indexedAt":   "2026-02-08T12:00:00Z",
				"docsIndexed": 10,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspace/tickets":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"ticket":    "CLARIFY_INFO_FLOW",
						"title":     "Clarify info flow",
						"status":    "active",
						"updatedAt": "2026-02-08T12:01:00Z",
						"topics":    []string{"core", "cli"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	snapshots := client.CollectSnapshots(t.Context(), []Endpoint{
		{
			Name:      "workspace-docs",
			Kind:      EndpointKindWorkspace,
			BaseURL:   server.URL,
			WebURL:    server.URL,
			Repo:      "metawsm",
			Workspace: "ws-001",
		},
	})
	if len(snapshots) != 1 {
		t.Fatalf("expected one snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Err != nil {
		t.Fatalf("expected no snapshot error, got %v", snapshots[0].Err)
	}
	if len(snapshots[0].Tickets) != 1 {
		t.Fatalf("expected one ticket, got %d", len(snapshots[0].Tickets))
	}
	if snapshots[0].Tickets[0].Ticket != "CLARIFY_INFO_FLOW" {
		t.Fatalf("unexpected ticket %q", snapshots[0].Tickets[0].Ticket)
	}
}

func TestMergeWorkspaceFirstPrefersWorkspace(t *testing.T) {
	snapshots := []EndpointSnapshot{
		{
			Endpoint: Endpoint{
				Name:    "repo-docs",
				Kind:    EndpointKindRepo,
				BaseURL: "http://repo-docs.local",
				Repo:    "metawsm",
			},
			Status: WorkspaceStatus{IndexedAt: "2026-02-08T10:00:00Z"},
			Tickets: []TicketItem{
				{
					Ticket:    "CLARIFY_INFO_FLOW",
					Title:     "Clarify info flow",
					Status:    "active",
					UpdatedAt: "2026-02-08T09:00:00Z",
				},
			},
		},
		{
			Endpoint: Endpoint{
				Name:      "workspace-docs",
				Kind:      EndpointKindWorkspace,
				BaseURL:   "http://workspace-docs.local",
				Repo:      "metawsm",
				Workspace: "ws-clarify",
			},
			Status: WorkspaceStatus{IndexedAt: "2026-02-08T10:05:00Z"},
			Tickets: []TicketItem{
				{
					Ticket:    "CLARIFY_INFO_FLOW",
					Title:     "Clarify info flow",
					Status:    "active",
					UpdatedAt: "2026-02-08T09:30:00Z",
				},
			},
		},
	}
	result := MergeWorkspaceFirst(snapshots, []ActiveContext{
		{
			Ticket:      "CLARIFY_INFO_FLOW",
			DocHomeRepo: "metawsm",
		},
	})
	if len(result.Tickets) != 1 {
		t.Fatalf("expected one merged ticket, got %d", len(result.Tickets))
	}
	if result.Tickets[0].SourceKind != EndpointKindWorkspace {
		t.Fatalf("expected workspace source to win, got %s", result.Tickets[0].SourceKind)
	}
	if !result.Tickets[0].Active {
		t.Fatalf("expected merged ticket to be active")
	}
}

func TestRefreshIndexesPostsRefreshEndpoint(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/index/refresh" {
			called = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"refreshed":   true,
				"indexedAt":   "2026-02-08T12:02:00Z",
				"docsIndexed": 12,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	results := client.RefreshIndexes(t.Context(), []Endpoint{
		{
			Name:    "workspace-docs",
			Kind:    EndpointKindWorkspace,
			BaseURL: server.URL,
			Repo:    "metawsm",
		},
	})
	if !called {
		t.Fatalf("expected refresh endpoint to be called")
	}
	if len(results) != 1 {
		t.Fatalf("expected one refresh result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected refresh error: %v", results[0].Err)
	}
	if !results[0].Refreshed {
		t.Fatalf("expected refreshed=true")
	}
	if !strings.Contains(results[0].IndexedAt, "2026-02-08T12:02:00Z") {
		t.Fatalf("unexpected indexedAt: %q", results[0].IndexedAt)
	}
}

func TestMergeKeepsDistinctDocHomeRepos(t *testing.T) {
	snapshots := []EndpointSnapshot{
		{
			Endpoint: Endpoint{
				Name:    "repo-metawsm",
				Kind:    EndpointKindRepo,
				BaseURL: "http://repo-metawsm.local",
				Repo:    "metawsm",
			},
			Status: WorkspaceStatus{IndexedAt: "2026-02-08T10:00:00Z"},
			Tickets: []TicketItem{
				{
					Ticket:    "CLARIFY_INFO_FLOW",
					Title:     "Clarify info flow (metawsm)",
					Status:    "active",
					UpdatedAt: "2026-02-08T09:00:00Z",
				},
			},
		},
		{
			Endpoint: Endpoint{
				Name:    "repo-workspace-manager",
				Kind:    EndpointKindRepo,
				BaseURL: "http://repo-wsm.local",
				Repo:    "workspace-manager",
			},
			Status: WorkspaceStatus{IndexedAt: "2026-02-08T10:01:00Z"},
			Tickets: []TicketItem{
				{
					Ticket:    "CLARIFY_INFO_FLOW",
					Title:     "Clarify info flow (wsm)",
					Status:    "active",
					UpdatedAt: "2026-02-08T09:30:00Z",
				},
			},
		},
	}
	result := MergeWorkspaceFirst(snapshots, nil)
	if len(result.Tickets) != 2 {
		t.Fatalf("expected two merged tickets (one per doc_home_repo), got %d", len(result.Tickets))
	}
	if result.Tickets[0].DocHomeRepo == result.Tickets[1].DocHomeRepo {
		t.Fatalf("expected distinct doc home repos in merged output")
	}
}
