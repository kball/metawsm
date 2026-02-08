package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"metawsm/internal/model"
	"metawsm/internal/store"
)

type API struct {
	store *store.SQLiteStore
}

func New(dbPath string) (*API, error) {
	s := store.NewSQLiteStore(dbPath)
	if err := s.Init(); err != nil {
		return nil, err
	}
	return &API{store: s}, nil
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", a.handleHealth)
	mux.HandleFunc("/api/v1/runs", a.handleRuns)
	mux.HandleFunc("/api/v1/runs/", a.handleRunByID)
}

type runStepCounts struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Running int `json:"running"`
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type runAgentCounts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Pending int `json:"pending"`
	Idle    int `json:"idle"`
	Stalled int `json:"stalled"`
	Dead    int `json:"dead"`
	Failed  int `json:"failed"`
	Stopped int `json:"stopped"`
}

type runSummary struct {
	RunID       string          `json:"run_id"`
	Status      model.RunStatus `json:"status"`
	Mode        model.RunMode   `json:"mode,omitempty"`
	Tickets     []string        `json:"tickets"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	ErrorText   string          `json:"error_text,omitempty"`
	StepCounts  runStepCounts   `json:"step_counts"`
	AgentCounts runAgentCounts  `json:"agent_counts"`
}

type runDetail struct {
	Run             runSummary              `json:"run"`
	Spec            model.RunSpec           `json:"spec"`
	Steps           []model.StepRecord      `json:"steps"`
	Agents          []model.AgentRecord     `json:"agents"`
	Brief           *model.RunBrief         `json:"brief,omitempty"`
	PendingGuidance []model.GuidanceRequest `json:"pending_guidance"`
	DocSyncStates   []model.DocSyncState    `json:"doc_sync_states"`
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	ticketFilter := strings.TrimSpace(r.URL.Query().Get("ticket"))

	runs, err := a.store.ListRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	items := make([]runSummary, 0, len(runs))
	for _, run := range runs {
		summary, _, err := a.getRunSummary(run)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "run_summary_error", err.Error())
			return
		}
		if ticketFilter != "" && !hasTicket(summary.Tickets, ticketFilter) {
			continue
		}
		items = append(items, summary)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runs": items,
	})
}

func (a *API) handleRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	runID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/runs/"))
	if runID == "" || strings.Contains(runID, "/") {
		writeError(w, http.StatusBadRequest, "invalid_run_id", "run id is required")
		return
	}

	run, _, _, err := a.store.GetRun(runID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "run_not_found", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	summary, spec, err := a.getRunSummary(run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "run_summary_error", err.Error())
		return
	}

	steps, err := a.store.GetSteps(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	agents, err := a.store.GetAgents(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	brief, err := a.store.GetRunBrief(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	pendingGuidance, err := a.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	docSyncStates, err := a.store.ListDocSyncStates(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, runDetail{
		Run:             summary,
		Spec:            spec,
		Steps:           steps,
		Agents:          agents,
		Brief:           brief,
		PendingGuidance: pendingGuidance,
		DocSyncStates:   docSyncStates,
	})
}

func (a *API) getRunSummary(run model.RunRecord) (runSummary, model.RunSpec, error) {
	runID := strings.TrimSpace(run.RunID)
	_, specJSON, _, err := a.store.GetRun(runID)
	if err != nil {
		return runSummary{}, model.RunSpec{}, err
	}
	spec := model.RunSpec{}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return runSummary{}, model.RunSpec{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	tickets, err := a.store.GetTickets(runID)
	if err != nil {
		return runSummary{}, model.RunSpec{}, err
	}
	steps, err := a.store.GetSteps(runID)
	if err != nil {
		return runSummary{}, model.RunSpec{}, err
	}
	agents, err := a.store.GetAgents(runID)
	if err != nil {
		return runSummary{}, model.RunSpec{}, err
	}

	summary := runSummary{
		RunID:       runID,
		Status:      run.Status,
		Mode:        spec.Mode,
		Tickets:     tickets,
		CreatedAt:   run.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   run.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ErrorText:   run.ErrorText,
		StepCounts:  summarizeSteps(steps),
		AgentCounts: summarizeAgents(agents),
	}
	return summary, spec, nil
}

func summarizeSteps(steps []model.StepRecord) runStepCounts {
	counts := runStepCounts{Total: len(steps)}
	for _, step := range steps {
		switch step.Status {
		case model.StepStatusDone:
			counts.Done++
		case model.StepStatusRunning:
			counts.Running++
		case model.StepStatusFailed:
			counts.Failed++
		case model.StepStatusSkipped:
			counts.Skipped++
		default:
			counts.Pending++
		}
	}
	return counts
}

func summarizeAgents(agents []model.AgentRecord) runAgentCounts {
	counts := runAgentCounts{Total: len(agents)}
	for _, agent := range agents {
		switch agent.Status {
		case model.AgentStatusRunning:
			counts.Running++
		case model.AgentStatusPending:
			counts.Pending++
		case model.AgentStatusIdle:
			counts.Idle++
		case model.AgentStatusStalled:
			counts.Stalled++
		case model.AgentStatusDead:
			counts.Dead++
		case model.AgentStatusFailed:
			counts.Failed++
		case model.AgentStatusStopped:
			counts.Stopped++
		}
	}
	return counts
}

func hasTicket(tickets []string, ticket string) bool {
	for _, item := range tickets {
		if strings.EqualFold(strings.TrimSpace(item), ticket) {
			return true
		}
	}
	return false
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]apiError{
		"error": {
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
