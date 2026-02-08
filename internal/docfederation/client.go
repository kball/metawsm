package docfederation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EndpointKind string

const (
	EndpointKindWorkspace EndpointKind = "workspace"
	EndpointKindRepo      EndpointKind = "repo"
)

type Endpoint struct {
	Name      string
	Kind      EndpointKind
	BaseURL   string
	WebURL    string
	Repo      string
	Workspace string
}

type WorkspaceStatus struct {
	Root      string
	RepoRoot  string
	IndexedAt string
	DocsCount int
}

type TicketItem struct {
	Ticket    string
	Title     string
	Status    string
	UpdatedAt string
	Topics    []string
}

type EndpointSnapshot struct {
	Endpoint Endpoint
	Status   WorkspaceStatus
	Tickets  []TicketItem
	Err      error
}

type RefreshResult struct {
	Endpoint  Endpoint
	Refreshed bool
	IndexedAt string
	DocsCount int
	Err       error
}

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) CollectSnapshots(ctx context.Context, endpoints []Endpoint) []EndpointSnapshot {
	out := make([]EndpointSnapshot, 0, len(endpoints))
	for _, endpoint := range endpoints {
		out = append(out, c.collectSnapshot(ctx, endpoint))
	}
	return out
}

func (c *Client) collectSnapshot(ctx context.Context, endpoint Endpoint) EndpointSnapshot {
	snapshot := EndpointSnapshot{Endpoint: endpoint}

	var status struct {
		Root        string `json:"root"`
		RepoRoot    string `json:"repoRoot"`
		IndexedAt   string `json:"indexedAt"`
		DocsIndexed int    `json:"docsIndexed"`
	}
	if err := c.getJSON(ctx, endpoint, "/api/v1/workspace/status", nil, &status); err != nil {
		snapshot.Err = fmt.Errorf("workspace status for %s: %w", endpoint.Name, err)
		return snapshot
	}
	snapshot.Status = WorkspaceStatus{
		Root:      strings.TrimSpace(status.Root),
		RepoRoot:  strings.TrimSpace(status.RepoRoot),
		IndexedAt: strings.TrimSpace(status.IndexedAt),
		DocsCount: status.DocsIndexed,
	}

	var ticketsResp struct {
		Results []struct {
			Ticket    string   `json:"ticket"`
			Title     string   `json:"title"`
			Status    string   `json:"status"`
			UpdatedAt string   `json:"updatedAt"`
			Topics    []string `json:"topics"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, endpoint, "/api/v1/workspace/tickets", map[string]string{
		"includeArchived": "true",
		"pageSize":        "1000",
	}, &ticketsResp); err != nil {
		snapshot.Err = fmt.Errorf("workspace tickets for %s: %w", endpoint.Name, err)
		return snapshot
	}
	tickets := make([]TicketItem, 0, len(ticketsResp.Results))
	for _, item := range ticketsResp.Results {
		tickets = append(tickets, TicketItem{
			Ticket:    strings.TrimSpace(item.Ticket),
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(item.Status),
			UpdatedAt: strings.TrimSpace(item.UpdatedAt),
			Topics:    append([]string(nil), item.Topics...),
		})
	}
	snapshot.Tickets = tickets
	return snapshot
}

func (c *Client) RefreshIndexes(ctx context.Context, endpoints []Endpoint) []RefreshResult {
	out := make([]RefreshResult, 0, len(endpoints))
	for _, endpoint := range endpoints {
		result := RefreshResult{Endpoint: endpoint}
		var refreshResp struct {
			Refreshed   bool   `json:"refreshed"`
			IndexedAt   string `json:"indexedAt"`
			DocsIndexed int    `json:"docsIndexed"`
		}
		if err := c.postJSON(ctx, endpoint, "/api/v1/index/refresh", nil, nil, &refreshResp); err != nil {
			result.Err = fmt.Errorf("refresh index for %s: %w", endpoint.Name, err)
			out = append(out, result)
			continue
		}
		result.Refreshed = refreshResp.Refreshed
		result.IndexedAt = strings.TrimSpace(refreshResp.IndexedAt)
		result.DocsCount = refreshResp.DocsIndexed
		out = append(out, result)
	}
	return out
}

func (c *Client) getJSON(ctx context.Context, endpoint Endpoint, path string, query map[string]string, out any) error {
	return c.doJSON(ctx, http.MethodGet, endpoint, path, query, nil, out)
}

func (c *Client) postJSON(ctx context.Context, endpoint Endpoint, path string, query map[string]string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	return c.doJSON(ctx, http.MethodPost, endpoint, path, query, reader, out)
}

func (c *Client) doJSON(ctx context.Context, method string, endpoint Endpoint, path string, query map[string]string, body io.Reader, out any) error {
	baseURL := strings.TrimSpace(endpoint.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("empty endpoint base URL")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	if len(query) > 0 {
		values := u.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		u.RawQuery = values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
