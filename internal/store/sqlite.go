package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"metawsm/internal/model"
)

type SQLiteStore struct {
	DBPath     string
	SQLitePath string
}

func NewSQLiteStore(dbPath string) *SQLiteStore {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = ".metawsm/metawsm.db"
	}
	return &SQLiteStore{
		DBPath:     dbPath,
		SQLitePath: "sqlite3",
	}
}

func (s *SQLiteStore) Init() error {
	if err := os.MkdirAll(filepath.Dir(s.DBPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	schema := `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS runs (
  run_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  spec_json TEXT NOT NULL,
  policy_json TEXT NOT NULL,
  error_text TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS run_tickets (
  run_id TEXT NOT NULL,
  ticket TEXT NOT NULL,
  PRIMARY KEY (run_id, ticket)
);
CREATE TABLE IF NOT EXISTS steps (
  run_id TEXT NOT NULL,
  step_index INTEGER NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  command_text TEXT NOT NULL,
  blocking INTEGER NOT NULL,
  ticket TEXT NOT NULL DEFAULT '',
  workspace_name TEXT NOT NULL DEFAULT '',
  agent_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_text TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (run_id, step_index)
);
CREATE TABLE IF NOT EXISTS agents (
  run_id TEXT NOT NULL,
  agent_name TEXT NOT NULL,
  workspace_name TEXT NOT NULL,
  session_name TEXT NOT NULL,
  status TEXT NOT NULL,
  health_state TEXT NOT NULL,
  last_activity_at TEXT NOT NULL DEFAULT '',
  last_progress_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (run_id, agent_name, workspace_name)
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS run_briefs (
  run_id TEXT PRIMARY KEY,
  ticket TEXT NOT NULL,
  goal TEXT NOT NULL,
  scope TEXT NOT NULL,
  done_criteria TEXT NOT NULL,
  constraints_text TEXT NOT NULL,
  merge_intent TEXT NOT NULL,
  qa_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS guidance_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  workspace_name TEXT NOT NULL,
  agent_name TEXT NOT NULL,
  question TEXT NOT NULL,
  context_text TEXT NOT NULL DEFAULT '',
  answer_text TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  answered_at TEXT NOT NULL DEFAULT ''
);`

	return s.execSQL(schema)
}

func (s *SQLiteStore) CreateRun(spec model.RunSpec, policyJSON string) error {
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal run spec: %w", err)
	}
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`INSERT INTO runs (run_id, status, created_at, updated_at, spec_json, policy_json, error_text)
VALUES (%s, %s, %s, %s, %s, %s, '');`,
		quote(spec.RunID), quote(string(model.RunStatusCreated)), quote(now), quote(now), quote(string(specBytes)), quote(policyJSON),
	)
	if err := s.execSQL(sql); err != nil {
		return err
	}

	var ticketSQL strings.Builder
	for _, ticket := range spec.Tickets {
		ticketSQL.WriteString(fmt.Sprintf(
			"INSERT OR IGNORE INTO run_tickets (run_id, ticket) VALUES (%s, %s);\n",
			quote(spec.RunID), quote(ticket),
		))
	}
	if ticketSQL.Len() > 0 {
		if err := s.execSQL(ticketSQL.String()); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertRunBrief(brief model.RunBrief) error {
	qaJSON, err := json.Marshal(brief.QA)
	if err != nil {
		return fmt.Errorf("marshal run brief QA: %w", err)
	}
	now := time.Now()
	createdAt := brief.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := brief.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	sql := fmt.Sprintf(
		`INSERT OR REPLACE INTO run_briefs
  (run_id, ticket, goal, scope, done_criteria, constraints_text, merge_intent, qa_json, created_at, updated_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s);`,
		quote(brief.RunID),
		quote(brief.Ticket),
		quote(brief.Goal),
		quote(brief.Scope),
		quote(brief.DoneCriteria),
		quote(brief.Constraints),
		quote(brief.MergeIntent),
		quote(string(qaJSON)),
		quote(createdAt.Format(time.RFC3339)),
		quote(updatedAt.Format(time.RFC3339)),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) GetRunBrief(runID string) (*model.RunBrief, error) {
	sql := fmt.Sprintf(
		`SELECT run_id, ticket, goal, scope, done_criteria, constraints_text, merge_intent, qa_json, created_at, updated_at
FROM run_briefs WHERE run_id=%s;`,
		quote(runID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse run_briefs created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
	if err != nil {
		return nil, fmt.Errorf("parse run_briefs updated_at: %w", err)
	}
	qa := []model.IntakeQA{}
	qaValue := strings.TrimSpace(asString(row["qa_json"]))
	if qaValue != "" {
		if err := json.Unmarshal([]byte(qaValue), &qa); err != nil {
			return nil, fmt.Errorf("parse run_briefs qa_json: %w", err)
		}
	}
	brief := &model.RunBrief{
		RunID:        asString(row["run_id"]),
		Ticket:       asString(row["ticket"]),
		Goal:         asString(row["goal"]),
		Scope:        asString(row["scope"]),
		DoneCriteria: asString(row["done_criteria"]),
		Constraints:  asString(row["constraints_text"]),
		MergeIntent:  asString(row["merge_intent"]),
		QA:           qa,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	return brief, nil
}

func (s *SQLiteStore) AddGuidanceRequest(req model.GuidanceRequest) (int64, error) {
	now := time.Now()
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	status := req.Status
	if status == "" {
		status = model.GuidanceStatusPending
	}
	sql := fmt.Sprintf(
		`INSERT INTO guidance_requests
  (run_id, workspace_name, agent_name, question, context_text, answer_text, status, created_at, answered_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, %s, '');`,
		quote(req.RunID),
		quote(req.WorkspaceName),
		quote(req.AgentName),
		quote(req.Question),
		quote(req.Context),
		quote(req.Answer),
		quote(string(status)),
		quote(createdAt.Format(time.RFC3339)),
	)
	if err := s.execSQL(sql); err != nil {
		return 0, err
	}
	idRows, err := s.queryJSON(fmt.Sprintf(
		`SELECT id FROM guidance_requests
WHERE run_id=%s
ORDER BY id DESC
LIMIT 1;`,
		quote(req.RunID),
	))
	if err != nil {
		return 0, err
	}
	if len(idRows) == 0 {
		return 0, fmt.Errorf("missing row id after guidance insert")
	}
	return int64(asInt(idRows[0]["id"])), nil
}

func (s *SQLiteStore) ListGuidanceRequests(runID string, status model.GuidanceStatus) ([]model.GuidanceRequest, error) {
	clauses := []string{fmt.Sprintf("run_id=%s", quote(runID))}
	if strings.TrimSpace(string(status)) != "" {
		clauses = append(clauses, fmt.Sprintf("status=%s", quote(string(status))))
	}
	sql := fmt.Sprintf(
		`SELECT id, run_id, workspace_name, agent_name, question, context_text, answer_text, status, created_at, answered_at
FROM guidance_requests
WHERE %s
ORDER BY id;`,
		strings.Join(clauses, " AND "),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.GuidanceRequest, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
		if err != nil {
			return nil, fmt.Errorf("parse guidance created_at: %w", err)
		}
		item := model.GuidanceRequest{
			ID:            int64(asInt(row["id"])),
			RunID:         asString(row["run_id"]),
			WorkspaceName: asString(row["workspace_name"]),
			AgentName:     asString(row["agent_name"]),
			Question:      asString(row["question"]),
			Context:       asString(row["context_text"]),
			Answer:        asString(row["answer_text"]),
			Status:        model.GuidanceStatus(asString(row["status"])),
			CreatedAt:     createdAt,
			AnsweredAt:    parseTimePtr(asString(row["answered_at"])),
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *SQLiteStore) MarkGuidanceAnswered(id int64, answer string) error {
	now := time.Now().Format(time.RFC3339)
	sql := fmt.Sprintf(
		`UPDATE guidance_requests
SET status=%s, answer_text=%s, answered_at=%s
WHERE id=%d;`,
		quote(string(model.GuidanceStatusAnswered)),
		quote(answer),
		quote(now),
		id,
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) SaveSteps(runID string, steps []model.PlanStep) error {
	var b strings.Builder
	for _, step := range steps {
		blocking := 0
		if step.Blocking {
			blocking = 1
		}
		b.WriteString(fmt.Sprintf(
			`INSERT OR REPLACE INTO steps
  (run_id, step_index, name, kind, command_text, blocking, ticket, workspace_name, agent_name, status, error_text, started_at, finished_at)
VALUES
  (%s, %d, %s, %s, %s, %d, %s, %s, %s, %s, '', '', '');
`,
			quote(runID),
			step.Index,
			quote(step.Name),
			quote(step.Kind),
			quote(step.Command),
			blocking,
			quote(step.Ticket),
			quote(step.WorkspaceName),
			quote(step.Agent),
			quote(string(step.Status)),
		))
	}
	if b.Len() == 0 {
		return nil
	}
	return s.execSQL(b.String())
}

func (s *SQLiteStore) UpsertAgent(agent model.AgentRecord) error {
	sql := fmt.Sprintf(
		`INSERT OR REPLACE INTO agents
  (run_id, agent_name, workspace_name, session_name, status, health_state, last_activity_at, last_progress_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, %s);`,
		quote(agent.RunID),
		quote(agent.Name),
		quote(agent.WorkspaceName),
		quote(agent.SessionName),
		quote(string(agent.Status)),
		quote(string(agent.HealthState)),
		quote(formatTime(agent.LastActivityAt)),
		quote(formatTime(agent.LastProgressAt)),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) UpdateRunStatus(runID string, status model.RunStatus, errorText string) error {
	sql := fmt.Sprintf(
		`UPDATE runs
SET status=%s, updated_at=%s, error_text=%s
WHERE run_id=%s;`,
		quote(string(status)), quote(time.Now().Format(time.RFC3339)), quote(errorText), quote(runID),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) UpdateStepStatus(runID string, stepIndex int, status model.StepStatus, errorText string, markStarted bool, markFinished bool) error {
	started := ""
	finished := ""
	if markStarted {
		started = time.Now().Format(time.RFC3339)
	}
	if markFinished {
		finished = time.Now().Format(time.RFC3339)
	}
	sql := fmt.Sprintf(
		`UPDATE steps
SET status=%s,
    error_text=%s,
    started_at=CASE WHEN %s != '' THEN %s ELSE started_at END,
    finished_at=CASE WHEN %s != '' THEN %s ELSE finished_at END
WHERE run_id=%s AND step_index=%d;`,
		quote(string(status)),
		quote(errorText),
		quote(started),
		quote(started),
		quote(finished),
		quote(finished),
		quote(runID),
		stepIndex,
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) UpdateAgentStatus(runID string, agentName string, workspaceName string, status model.AgentStatus, health model.HealthState, lastActivity *time.Time, lastProgress *time.Time) error {
	sql := fmt.Sprintf(
		`UPDATE agents
SET status=%s,
    health_state=%s,
    last_activity_at=%s,
    last_progress_at=%s
WHERE run_id=%s AND agent_name=%s AND workspace_name=%s;`,
		quote(string(status)),
		quote(string(health)),
		quote(formatTime(lastActivity)),
		quote(formatTime(lastProgress)),
		quote(runID),
		quote(agentName),
		quote(workspaceName),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) AddEvent(runID, entityType, entityID, eventType, fromState, toState, message string) error {
	sql := fmt.Sprintf(
		`INSERT INTO events
  (run_id, entity_type, entity_id, event_type, from_state, to_state, message, created_at)
VALUES
  (%s, %s, %s, %s, %s, %s, %s, %s);`,
		quote(runID), quote(entityType), quote(entityID), quote(eventType), quote(fromState), quote(toState), quote(message), quote(time.Now().Format(time.RFC3339)),
	)
	return s.execSQL(sql)
}

func (s *SQLiteStore) GetRun(runID string) (model.RunRecord, string, string, error) {
	sql := fmt.Sprintf(
		`SELECT run_id, status, created_at, updated_at, error_text, spec_json, policy_json FROM runs WHERE run_id=%s;`,
		quote(runID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return model.RunRecord{}, "", "", err
	}
	if len(rows) == 0 {
		return model.RunRecord{}, "", "", fmt.Errorf("run %s not found", runID)
	}
	row := rows[0]
	record, err := parseRunRecord(row)
	if err != nil {
		return model.RunRecord{}, "", "", err
	}
	specJSON := asString(row["spec_json"])
	policyJSON := asString(row["policy_json"])
	return record, specJSON, policyJSON, nil
}

func (s *SQLiteStore) ListRuns() ([]model.RunRecord, error) {
	sql := `SELECT run_id, status, created_at, updated_at, error_text, spec_json, policy_json FROM runs ORDER BY updated_at DESC;`
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.RunRecord, 0, len(rows))
	for _, row := range rows {
		record, err := parseRunRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *SQLiteStore) GetTickets(runID string) ([]string, error) {
	sql := fmt.Sprintf(`SELECT ticket FROM run_tickets WHERE run_id=%s ORDER BY ticket;`, quote(runID))
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, asString(row["ticket"]))
	}
	return out, nil
}

func (s *SQLiteStore) GetSteps(runID string) ([]model.StepRecord, error) {
	sql := fmt.Sprintf(
		`SELECT run_id, step_index, name, kind, command_text, blocking, ticket, workspace_name, agent_name, status, error_text, started_at, finished_at
FROM steps WHERE run_id=%s ORDER BY step_index;`,
		quote(runID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.StepRecord, 0, len(rows))
	for _, row := range rows {
		step := model.StepRecord{
			RunID:         asString(row["run_id"]),
			Index:         asInt(row["step_index"]),
			Name:          asString(row["name"]),
			Kind:          asString(row["kind"]),
			Command:       asString(row["command_text"]),
			Blocking:      asInt(row["blocking"]) == 1,
			Ticket:        asString(row["ticket"]),
			WorkspaceName: asString(row["workspace_name"]),
			Agent:         asString(row["agent_name"]),
			Status:        model.StepStatus(asString(row["status"])),
			ErrorText:     asString(row["error_text"]),
			StartedAt:     parseTimePtr(asString(row["started_at"])),
			FinishedAt:    parseTimePtr(asString(row["finished_at"])),
		}
		out = append(out, step)
	}
	return out, nil
}

func (s *SQLiteStore) GetAgents(runID string) ([]model.AgentRecord, error) {
	sql := fmt.Sprintf(
		`SELECT run_id, agent_name, workspace_name, session_name, status, health_state, last_activity_at, last_progress_at
FROM agents WHERE run_id=%s ORDER BY workspace_name, agent_name;`,
		quote(runID),
	)
	rows, err := s.queryJSON(sql)
	if err != nil {
		return nil, err
	}
	out := make([]model.AgentRecord, 0, len(rows))
	for _, row := range rows {
		agent := model.AgentRecord{
			RunID:          asString(row["run_id"]),
			Name:           asString(row["agent_name"]),
			WorkspaceName:  asString(row["workspace_name"]),
			SessionName:    asString(row["session_name"]),
			Status:         model.AgentStatus(asString(row["status"])),
			HealthState:    model.HealthState(asString(row["health_state"])),
			LastActivityAt: parseTimePtr(asString(row["last_activity_at"])),
			LastProgressAt: parseTimePtr(asString(row["last_progress_at"])),
		}
		out = append(out, agent)
	}
	return out, nil
}

func (s *SQLiteStore) execSQL(sql string) error {
	cmd := exec.Command(s.SQLitePath, s.DBPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sqlite exec failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *SQLiteStore) queryJSON(sql string) ([]map[string]any, error) {
	cmd := exec.Command(s.SQLitePath, "-json", s.DBPath, sql)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite query failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return []map[string]any{}, nil
	}
	rows := []map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json output: %w", err)
	}
	return rows, nil
}

func parseRunRecord(row map[string]any) (model.RunRecord, error) {
	createdAt, err := time.Parse(time.RFC3339, asString(row["created_at"]))
	if err != nil {
		return model.RunRecord{}, fmt.Errorf("parse run created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, asString(row["updated_at"]))
	if err != nil {
		return model.RunRecord{}, fmt.Errorf("parse run updated_at: %w", err)
	}
	return model.RunRecord{
		RunID:     asString(row["run_id"]),
		Status:    model.RunStatus(asString(row["status"])),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		ErrorText: asString(row["error_text"]),
	}, nil
}

func quote(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}

func asString(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(v)
	}
}

func asInt(v any) int {
	switch typed := v.(type) {
	case float64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(typed)
		return n
	case int:
		return typed
	default:
		return 0
	}
}

func parseTimePtr(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

func formatTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format(time.RFC3339)
}
