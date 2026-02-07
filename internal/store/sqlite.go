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
