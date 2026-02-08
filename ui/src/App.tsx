import { useEffect, useMemo, useState } from "react";

type StepCounts = {
  total: number;
  done: number;
  running: number;
  pending: number;
  failed: number;
  skipped: number;
};

type AgentCounts = {
  total: number;
  running: number;
  pending: number;
  idle: number;
  stalled: number;
  dead: number;
  failed: number;
  stopped: number;
};

type RunSummary = {
  run_id: string;
  status: string;
  mode?: string;
  tickets: string[];
  created_at: string;
  updated_at: string;
  error_text?: string;
  step_counts: StepCounts;
  agent_counts: AgentCounts;
};

type RunDetail = {
  run: RunSummary;
  spec: {
    run_id: string;
    mode?: string;
    repos: string[];
    doc_home_repo?: string;
    base_branch?: string;
    workspace_strategy?: string;
  };
  steps: Array<{
    index: number;
    name: string;
    status: string;
    kind: string;
    workspace_name?: string;
    agent?: string;
    started_at?: string;
    finished_at?: string;
  }>;
  agents: Array<{
    name: string;
    workspace_name: string;
    status: string;
    health_state: string;
    session_name: string;
    last_activity_at?: string;
    last_progress_at?: string;
  }>;
  brief?: {
    goal: string;
    scope: string;
    done_criteria: string;
    constraints: string;
    merge_intent: string;
  };
  pending_guidance: Array<{
    id: number;
    workspace_name: string;
    agent_name: string;
    question: string;
    created_at: string;
  }>;
  doc_sync_states: Array<{
    ticket: string;
    workspace_name: string;
    status: string;
    revision: string;
    updated_at: string;
  }>;
};

export function App() {
  const [ticketFilter, setTicketFilter] = useState("");
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState("");
  const [selectedRunID, setSelectedRunID] = useState("");

  const [detail, setDetail] = useState<RunDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");

  const selectedRun = useMemo(
    () => runs.find((run) => run.run_id === selectedRunID) ?? null,
    [runs, selectedRunID],
  );

  useEffect(() => {
    void loadRuns(ticketFilter);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticketFilter]);

  useEffect(() => {
    if (selectedRunID === "") {
      setDetail(null);
      setDetailError("");
      return;
    }
    void loadRunDetail(selectedRunID);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedRunID]);

  async function loadRuns(ticket: string) {
    setRunsLoading(true);
    setRunsError("");
    try {
      const suffix = ticket.trim() ? `?ticket=${encodeURIComponent(ticket.trim())}` : "";
      const response = await fetch(`/api/v1/runs${suffix}`);
      if (!response.ok) {
        throw new Error(`Request failed (${response.status})`);
      }
      const payload = (await response.json()) as { runs: RunSummary[] };
      setRuns(payload.runs);

      if (payload.runs.length === 0) {
        setSelectedRunID("");
      } else {
        const stillPresent = payload.runs.some((run) => run.run_id === selectedRunID);
        if (!stillPresent) {
          setSelectedRunID(payload.runs[0].run_id);
        }
      }
    } catch (error) {
      setRunsError(errorToString(error));
    } finally {
      setRunsLoading(false);
    }
  }

  async function loadRunDetail(runID: string) {
    setDetailLoading(true);
    setDetailError("");
    try {
      const response = await fetch(`/api/v1/runs/${encodeURIComponent(runID)}`);
      if (!response.ok) {
        throw new Error(`Request failed (${response.status})`);
      }
      const payload = (await response.json()) as RunDetail;
      setDetail(payload);
    } catch (error) {
      setDetail(null);
      setDetailError(errorToString(error));
    } finally {
      setDetailLoading(false);
    }
  }

  function refreshAll() {
    void loadRuns(ticketFilter);
    if (selectedRunID) {
      void loadRunDetail(selectedRunID);
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div>
          <p className="kicker">metawsm monitor</p>
          <h1>Runs Dashboard</h1>
        </div>
        <div className="controls">
          <label htmlFor="ticket-filter">Ticket</label>
          <input
            id="ticket-filter"
            value={ticketFilter}
            onChange={(event) => setTicketFilter(event.target.value)}
            placeholder="METAWSM-006"
          />
          <button type="button" onClick={refreshAll}>
            Refresh
          </button>
        </div>
      </header>

      <main className="layout">
        <section className="panel list-panel" aria-label="Runs list">
          <div className="panel-head">
            <h2>Runs</h2>
            <p>{runs.length} total</p>
          </div>
          {runsLoading ? <p className="state">Loading runs…</p> : null}
          {runsError ? <p className="state error">{runsError}</p> : null}
          {!runsLoading && !runsError && runs.length === 0 ? <p className="state">No runs found.</p> : null}
          <ul className="run-list">
            {runs.map((run) => (
              <li key={run.run_id}>
                <button
                  type="button"
                  className={run.run_id === selectedRunID ? "run-card active" : "run-card"}
                  onClick={() => setSelectedRunID(run.run_id)}
                >
                  <span className={`badge status-${run.status}`}>{run.status}</span>
                  <strong>{run.run_id}</strong>
                  <span>{run.mode || "run"}</span>
                  <span>{run.tickets.join(", ") || "No tickets"}</span>
                  <small>Updated {formatDate(run.updated_at)}</small>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel detail-panel" aria-label="Run details">
          <div className="panel-head">
            <h2>Details</h2>
            <p>{selectedRun ? selectedRun.run_id : "Select a run"}</p>
          </div>

          {detailLoading ? <p className="state">Loading details…</p> : null}
          {detailError ? <p className="state error">{detailError}</p> : null}
          {!detailLoading && !detailError && detail == null ? <p className="state">Select a run to inspect.</p> : null}

          {detail ? (
            <>
              <div className="stats-grid">
                <article>
                  <h3>Step Progress</h3>
                  <p>
                    {detail.run.step_counts.done}/{detail.run.step_counts.total} done
                  </p>
                  <small>
                    running {detail.run.step_counts.running}, pending {detail.run.step_counts.pending}, failed {detail.run.step_counts.failed}
                  </small>
                </article>
                <article>
                  <h3>Agents</h3>
                  <p>{detail.run.agent_counts.total} total</p>
                  <small>
                    running {detail.run.agent_counts.running}, stalled {detail.run.agent_counts.stalled}, failed {detail.run.agent_counts.failed}
                  </small>
                </article>
                <article>
                  <h3>Scope</h3>
                  <p>{detail.spec.repos.join(", ") || "-"}</p>
                  <small>base branch {detail.spec.base_branch || "-"}</small>
                </article>
              </div>

              {detail.brief ? (
                <section className="detail-section">
                  <h3>Run Brief</h3>
                  <p>{detail.brief.goal}</p>
                  <p className="subtle">Done criteria: {detail.brief.done_criteria}</p>
                </section>
              ) : null}

              <section className="detail-section">
                <h3>Agents</h3>
                <table>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Workspace</th>
                      <th>Status</th>
                      <th>Health</th>
                      <th>Session</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.agents.map((agent) => (
                      <tr key={`${agent.workspace_name}:${agent.name}`}>
                        <td>{agent.name}</td>
                        <td>{agent.workspace_name}</td>
                        <td>{agent.status}</td>
                        <td>{agent.health_state}</td>
                        <td>{agent.session_name}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>

              <section className="detail-section">
                <h3>Plan Steps</h3>
                <table>
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>Name</th>
                      <th>Status</th>
                      <th>Kind</th>
                      <th>Agent</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.steps.map((step) => (
                      <tr key={step.index}>
                        <td>{step.index}</td>
                        <td>{step.name}</td>
                        <td>{step.status}</td>
                        <td>{step.kind}</td>
                        <td>{step.agent || "-"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>

              {detail.pending_guidance.length > 0 ? (
                <section className="detail-section">
                  <h3>Pending Guidance</h3>
                  <ul className="guidance-list">
                    {detail.pending_guidance.map((item) => (
                      <li key={item.id}>
                        <strong>#{item.id}</strong> {item.agent_name}@{item.workspace_name}: {item.question}
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
            </>
          ) : null}
        </section>
      </main>
    </div>
  );
}

function formatDate(value: string): string {
  if (!value) {
    return "unknown";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function errorToString(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
