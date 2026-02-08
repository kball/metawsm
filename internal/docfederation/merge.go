package docfederation

import (
	"sort"
	"strings"
	"time"
)

type ActiveContext struct {
	Ticket      string
	DocHomeRepo string
}

type EndpointHealth struct {
	Endpoint  Endpoint
	Reachable bool
	IndexedAt string
	ErrorText string
}

type AggregatedTicket struct {
	Ticket       string
	Title        string
	Status       string
	Topics       []string
	DocHomeRepo  string
	Active       bool
	SourceKind   EndpointKind
	SourceName   string
	SourceURL    string
	SourceWebURL string
	SourceRepo   string
	SourceWS     string
	UpdatedAt    string
	IndexedAt    string
}

type MergeResult struct {
	Tickets []AggregatedTicket
	Health  []EndpointHealth
}

func MergeWorkspaceFirst(snapshots []EndpointSnapshot, contexts []ActiveContext) MergeResult {
	health := make([]EndpointHealth, 0, len(snapshots))
	active := map[string]struct{}{}
	for _, context := range contexts {
		ticket := strings.ToUpper(strings.TrimSpace(context.Ticket))
		repo := strings.ToLower(strings.TrimSpace(context.DocHomeRepo))
		if ticket == "" || repo == "" {
			continue
		}
		active[ticket+"|"+repo] = struct{}{}
	}

	type candidate struct {
		ticket AggregatedTicket
	}
	byKey := map[string]candidate{}

	for _, snapshot := range snapshots {
		entry := EndpointHealth{
			Endpoint:  snapshot.Endpoint,
			Reachable: snapshot.Err == nil,
			IndexedAt: snapshot.Status.IndexedAt,
		}
		if snapshot.Err != nil {
			entry.ErrorText = snapshot.Err.Error()
			health = append(health, entry)
			continue
		}
		health = append(health, entry)

		docHomeRepo := strings.TrimSpace(snapshot.Endpoint.Repo)
		for _, item := range snapshot.Tickets {
			ticketID := strings.TrimSpace(item.Ticket)
			if ticketID == "" {
				continue
			}
			activeKey := strings.ToUpper(ticketID) + "|" + strings.ToLower(docHomeRepo)
			_, isActive := active[activeKey]
			key := strings.ToUpper(ticketID) + "|" + strings.ToLower(docHomeRepo) + "|" + boolKey(isActive)
			candidateTicket := AggregatedTicket{
				Ticket:       ticketID,
				Title:        strings.TrimSpace(item.Title),
				Status:       strings.TrimSpace(item.Status),
				Topics:       append([]string(nil), item.Topics...),
				DocHomeRepo:  docHomeRepo,
				Active:       isActive,
				SourceKind:   snapshot.Endpoint.Kind,
				SourceName:   snapshot.Endpoint.Name,
				SourceURL:    snapshot.Endpoint.BaseURL,
				SourceWebURL: webURLOrBase(snapshot.Endpoint),
				SourceRepo:   snapshot.Endpoint.Repo,
				SourceWS:     snapshot.Endpoint.Workspace,
				UpdatedAt:    item.UpdatedAt,
				IndexedAt:    snapshot.Status.IndexedAt,
			}
			existing, exists := byKey[key]
			if !exists || preferredOver(candidateTicket, existing.ticket) {
				byKey[key] = candidate{ticket: candidateTicket}
			}
		}
	}

	tickets := make([]AggregatedTicket, 0, len(byKey))
	for _, item := range byKey {
		tickets = append(tickets, item.ticket)
	}
	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].Active != tickets[j].Active {
			return tickets[i].Active
		}
		if strings.ToUpper(tickets[i].Ticket) != strings.ToUpper(tickets[j].Ticket) {
			return strings.ToUpper(tickets[i].Ticket) < strings.ToUpper(tickets[j].Ticket)
		}
		if strings.ToLower(tickets[i].DocHomeRepo) != strings.ToLower(tickets[j].DocHomeRepo) {
			return strings.ToLower(tickets[i].DocHomeRepo) < strings.ToLower(tickets[j].DocHomeRepo)
		}
		return tickets[i].SourceName < tickets[j].SourceName
	})

	sort.Slice(health, func(i, j int) bool {
		if health[i].Endpoint.Kind != health[j].Endpoint.Kind {
			return health[i].Endpoint.Kind == EndpointKindWorkspace
		}
		return health[i].Endpoint.Name < health[j].Endpoint.Name
	})

	return MergeResult{
		Tickets: tickets,
		Health:  health,
	}
}

func preferredOver(candidate AggregatedTicket, existing AggregatedTicket) bool {
	candidateKindRank := endpointKindRank(candidate.SourceKind)
	existingKindRank := endpointKindRank(existing.SourceKind)
	if candidateKindRank != existingKindRank {
		return candidateKindRank > existingKindRank
	}
	candidateUpdatedAt := parseFlexibleTime(candidate.UpdatedAt)
	existingUpdatedAt := parseFlexibleTime(existing.UpdatedAt)
	if !candidateUpdatedAt.Equal(existingUpdatedAt) {
		return candidateUpdatedAt.After(existingUpdatedAt)
	}
	candidateIndexedAt := parseFlexibleTime(candidate.IndexedAt)
	existingIndexedAt := parseFlexibleTime(existing.IndexedAt)
	if !candidateIndexedAt.Equal(existingIndexedAt) {
		return candidateIndexedAt.After(existingIndexedAt)
	}
	return candidate.SourceName < existing.SourceName
}

func endpointKindRank(kind EndpointKind) int {
	if kind == EndpointKindWorkspace {
		return 2
	}
	return 1
}

func parseFlexibleTime(raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func webURLOrBase(endpoint Endpoint) string {
	if strings.TrimSpace(endpoint.WebURL) != "" {
		return endpoint.WebURL
	}
	return endpoint.BaseURL
}

func boolKey(value bool) string {
	if value {
		return "active"
	}
	return "inactive"
}
