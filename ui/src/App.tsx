import { useEffect, useMemo, useState } from "react";

type RunSnapshot = {
  run_id: string;
  status: string;
  tickets: string[];
  pending_guidance: Array<{
    thread_id: string;
    agent_name: string;
    workspace_name: string;
    question: string;
  }>;
};

type ForumThread = {
  thread_id: string;
  ticket: string;
  run_id: string;
  title: string;
  state: string;
  priority: string;
  assignee_name: string;
  posts_count: number;
  updated_at: string;
};

export function App() {
  const [runs, setRuns] = useState<RunSnapshot[]>([]);
  const [threads, setThreads] = useState<ForumThread[]>([]);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [error, setError] = useState("");

  const selectedRun = useMemo(
    () => runs.find((item) => item.run_id === selectedRunID) ?? null,
    [runs, selectedRunID],
  );
  const selectedTicket = selectedRun?.tickets?.[0] ?? "";

  useEffect(() => {
    void refreshRuns();
  }, []);

  useEffect(() => {
    if (selectedTicket) {
      void refreshThreads(selectedTicket, selectedRunID);
    } else {
      setThreads([]);
    }
  }, [selectedTicket, selectedRunID]);

  useEffect(() => {
    if (!selectedTicket) {
      return;
    }
    const socketURL = new URL("/api/v1/forum/stream", window.location.origin);
    socketURL.protocol = socketURL.protocol.replace("http", "ws");
    socketURL.searchParams.set("ticket", selectedTicket);

    const socket = new WebSocket(socketURL);
    socket.onmessage = () => {
      void refreshThreads(selectedTicket, selectedRunID);
    };
    socket.onerror = () => {
      setError("WebSocket stream unavailable; using pull refresh only.");
    };
    return () => {
      socket.close();
    };
  }, [selectedTicket, selectedRunID]);

  async function refreshRuns() {
    try {
      setError("");
      const response = await fetch("/api/v1/runs");
      if (!response.ok) {
        throw new Error(`runs request failed (${response.status})`);
      }
      const payload = (await response.json()) as { runs?: unknown[] };
      const normalizedRuns = Array.isArray(payload.runs)
        ? payload.runs
            .map(normalizeRunSnapshot)
            .filter((item): item is RunSnapshot => item !== null)
        : [];
      setRuns(normalizedRuns);
      if (!selectedRunID && normalizedRuns.length > 0) {
        setSelectedRunID(normalizedRuns[0].run_id);
      }
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function refreshThreads(ticket: string, runID: string) {
    try {
      setError("");
      const query = new URLSearchParams();
      query.set("ticket", ticket);
      if (runID) {
        query.set("run_id", runID);
      }
      query.set("limit", "200");
      const response = await fetch(`/api/v1/forum/threads?${query.toString()}`);
      if (!response.ok) {
        throw new Error(`threads request failed (${response.status})`);
      }
      const payload = (await response.json()) as { threads: ForumThread[] };
      setThreads(payload.threads);
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  return (
    <div className="layout">
      <header className="topbar">
        <h1>metawsm daemon dashboard</h1>
        <button onClick={() => void refreshRuns()} type="button">
          Refresh
        </button>
      </header>

      {error ? <div className="error">{error}</div> : null}

      <div className="grid">
        <section className="panel">
          <h2>Runs</h2>
          {runs.length === 0 ? <p className="muted">No runs available.</p> : null}
          <ul className="list">
            {runs.map((run) => (
              <li key={run.run_id}>
                <button
                  type="button"
                  className={run.run_id === selectedRunID ? "item selected" : "item"}
                  onClick={() => setSelectedRunID(run.run_id)}
                >
                  <strong>{run.run_id}</strong>
                  <span>{run.status}</span>
                  <small>{run.tickets.length > 0 ? run.tickets.join(", ") : "No tickets"}</small>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel">
          <h2>Forum Threads {selectedTicket ? `(${selectedTicket})` : ""}</h2>
          {threads.length === 0 ? <p className="muted">No threads for selection.</p> : null}
          <ul className="list">
            {threads.map((thread) => (
              <li key={thread.thread_id} className="thread">
                <strong>{thread.title}</strong>
                <span>
                  {thread.state} · {thread.priority} · posts={thread.posts_count}
                </span>
                <small>
                  thread={thread.thread_id} assignee={thread.assignee_name || "-"}
                </small>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

function toErrorString(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function normalizeRunSnapshot(value: unknown): RunSnapshot | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const raw = value as Record<string, unknown>;
  const runID = pickString(raw.run_id, raw.RunID);
  if (!runID) {
    return null;
  }
  return {
    run_id: runID,
    status: pickString(raw.status, raw.Status) ?? "unknown",
    tickets: normalizeStringArray(raw.tickets ?? raw.Tickets),
    pending_guidance: normalizeGuidanceArray(raw.pending_guidance ?? raw.PendingGuidance),
  };
}

function pickString(...values: unknown[]): string | null {
  for (const value of values) {
    if (typeof value === "string") {
      const trimmed = value.trim();
      if (trimmed) {
        return trimmed;
      }
    }
  }
  return null;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .filter((item): item is string => typeof item === "string")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function normalizeGuidanceArray(
  value: unknown,
): Array<{ thread_id: string; agent_name: string; workspace_name: string; question: string }> {
  if (!Array.isArray(value)) {
    return [];
  }
  const normalized = value
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const raw = item as Record<string, unknown>;
      const threadID = pickString(raw.thread_id, raw.ThreadID) ?? "";
      const agentName = pickString(raw.agent_name, raw.AgentName) ?? "";
      const workspaceName = pickString(raw.workspace_name, raw.WorkspaceName) ?? "";
      const question = pickString(raw.question, raw.Question) ?? "";
      if (!threadID || !agentName || !workspaceName || !question) {
        return null;
      }
      return {
        thread_id: threadID,
        agent_name: agentName,
        workspace_name: workspaceName,
        question,
      };
    })
    .filter(
      (
        item,
      ): item is {
        thread_id: string;
        agent_name: string;
        workspace_name: string;
        question: string;
      } => item !== null,
    );
  return normalized;
}
