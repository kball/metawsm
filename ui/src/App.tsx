import { useEffect, useMemo, useRef, useState } from "react";

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
  opened_at: string;
  last_event_sequence?: number;
  last_actor_type?: string;
  is_unseen?: boolean;
  is_unanswered?: boolean;
};

type ForumPost = {
  post_id: string;
  event_id: string;
  author_type: string;
  author_name: string;
  body: string;
  created_at: string;
};

type ForumEvent = {
  sequence: number;
  envelope: {
    event_id: string;
    event_type: string;
    thread_id: string;
    ticket: string;
    run_id?: string;
    actor_type?: string;
    actor_name?: string;
    occurred_at: string;
  };
  payload_json?: string;
};

type ForumThreadDetail = {
  thread: ForumThread;
  posts: ForumPost[];
  events: ForumEvent[];
};

type ForumOutboxMessage = {
  message_id: string;
  topic: string;
  status: string;
  attempt_count: number;
  last_error?: string;
  updated_at: string;
};

type ForumBusTopicDebug = {
  topic: string;
  stream: string;
  handler_registered: boolean;
  subscribed: boolean;
  stream_exists: boolean;
  stream_length: number;
  consumer_group_present: boolean;
  consumer_group_pending: number;
  consumer_group_lag: number;
  topic_error?: string;
};

type ForumDebugSnapshot = {
  generated_at: string;
  ticket?: string;
  run_id?: string;
  outbox: {
    pending_count: number;
    processing_count: number;
    failed_count: number;
    oldest_pending_age_seconds: number;
  };
  outbox_messages: ForumOutboxMessage[];
  events: ForumEvent[];
  bus: {
    running: boolean;
    healthy: boolean;
    health_error?: string;
    redis_url?: string;
    stream_name: string;
    consumer_group: string;
    consumer_name: string;
    topics: ForumBusTopicDebug[];
  };
};

type QueueTab = "all" | "unseen" | "unanswered";
type ForumStreamFrame = {
  type?: string;
  events?: unknown[];
};

export function App() {
  const [runs, setRuns] = useState<RunSnapshot[]>([]);
  const [selectedRunID, setSelectedRunID] = useState("");

  const [threads, setThreads] = useState<ForumThread[]>([]);
  const [selectedThreadID, setSelectedThreadID] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<ForumThreadDetail | null>(null);

  const [queueTab, setQueueTab] = useState<QueueTab>("all");
  const [queueCounts, setQueueCounts] = useState({ unseen: 0, unanswered: 0 });

  const [queryText, setQueryText] = useState("");
  const [stateFilter, setStateFilter] = useState("");
  const [priorityFilter, setPriorityFilter] = useState("");
  const [assigneeFilter, setAssigneeFilter] = useState("");

  const [viewerType, setViewerType] = useState("human");
  const [viewerID, setViewerID] = useState("human:operator");

  const [replyBody, setReplyBody] = useState("");
  const [savingReply, setSavingReply] = useState(false);

  const [debugSnapshot, setDebugSnapshot] = useState<ForumDebugSnapshot | null>(null);
  const [error, setError] = useState("");
  const streamRefreshTimer = useRef<number | null>(null);
  const selectedThreadRef = useRef("");

  const selectedRun = useMemo(
    () => runs.find((item) => item.run_id === selectedRunID) ?? null,
    [runs, selectedRunID],
  );
  const selectedTicket = selectedRun?.tickets?.[0] ?? "";

  const diagnosticsWarning = useMemo(() => {
    if (!debugSnapshot) {
      return "";
    }
    if (!debugSnapshot.bus.healthy) {
      return "Forum bus is unhealthy; queue/search freshness may lag.";
    }
    if (debugSnapshot.outbox.failed_count > 0) {
      return `Forum outbox has ${debugSnapshot.outbox.failed_count} failed message(s).`;
    }
    if (debugSnapshot.outbox.pending_count > 50) {
      return `Forum outbox backlog is elevated (${debugSnapshot.outbox.pending_count} pending).`;
    }
    return "";
  }, [debugSnapshot]);

  useEffect(() => {
    void refreshRuns();
  }, []);

  useEffect(() => {
    void refreshForumData();
  }, [
    selectedTicket,
    selectedRunID,
    queueTab,
    queryText,
    stateFilter,
    priorityFilter,
    assigneeFilter,
    viewerType,
    viewerID,
  ]);

  useEffect(() => {
    void refreshDebug(selectedTicket, selectedRunID);
    const interval = window.setInterval(() => {
      void refreshDebug(selectedTicket, selectedRunID);
    }, 15000);
    return () => {
      window.clearInterval(interval);
    };
  }, [selectedTicket, selectedRunID]);

  useEffect(() => {
    if (!selectedThreadID) {
      setSelectedDetail(null);
      return;
    }
    void refreshThreadDetail(selectedThreadID);
    void markThreadSeen(selectedThreadID);
  }, [selectedThreadID, viewerType, viewerID]);

  useEffect(() => {
    selectedThreadRef.current = selectedThreadID;
  }, [selectedThreadID]);

  useEffect(() => {
    if (!selectedTicket) {
      return;
    }
    const socketURL = new URL("/api/v1/forum/stream", window.location.origin);
    socketURL.protocol = socketURL.protocol.replace("http", "ws");
    socketURL.searchParams.set("ticket", selectedTicket);

    const socket = new WebSocket(socketURL);
    socket.onmessage = (event) => {
      const frame = parseForumStreamFrame(event.data);
      if (!frame || frame.type !== "forum.events" || !Array.isArray(frame.events) || frame.events.length === 0) {
        return;
      }
      if (streamRefreshTimer.current !== null) {
        window.clearTimeout(streamRefreshTimer.current);
      }
      streamRefreshTimer.current = window.setTimeout(() => {
        void refreshForumData();
        if (selectedThreadRef.current) {
          void refreshThreadDetail(selectedThreadRef.current);
        }
        streamRefreshTimer.current = null;
      }, 150);
    };
    socket.onerror = () => {
      setError("WebSocket stream unavailable; using pull refresh only.");
    };
    return () => {
      if (streamRefreshTimer.current !== null) {
        window.clearTimeout(streamRefreshTimer.current);
        streamRefreshTimer.current = null;
      }
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
      if (normalizedRuns.length > 0) {
        const hasSelected = normalizedRuns.some((run) => run.run_id === selectedRunID);
        if (!selectedRunID || !hasSelected) {
          setSelectedRunID(normalizedRuns[0].run_id);
        }
      }
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function refreshForumData() {
    if (!selectedTicket) {
      setThreads([]);
      setQueueCounts({ unseen: 0, unanswered: 0 });
      return;
    }
    try {
      setError("");
      const [threadRows, counts] = await Promise.all([loadThreadRows(), loadQueueCounts()]);
      setThreads(threadRows);
      setQueueCounts(counts);
      if (selectedThreadID && !threadRows.some((thread) => thread.thread_id === selectedThreadID)) {
        setSelectedThreadID("");
      }
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function loadThreadRows(): Promise<ForumThread[]> {
    const query = new URLSearchParams();
    query.set("ticket", selectedTicket);
    if (selectedRunID) {
      query.set("run_id", selectedRunID);
    }
    if (stateFilter) {
      query.set("state", stateFilter);
    }
    if (priorityFilter) {
      query.set("priority", priorityFilter);
    }
    if (assigneeFilter) {
      query.set("assignee", assigneeFilter);
    }
    if (viewerType) {
      query.set("viewer_type", viewerType);
    }
    if (viewerID) {
      query.set("viewer_id", viewerID);
    }
    query.set("limit", "200");

    let path = "/api/v1/forum/search";
    if (queueTab === "all") {
      if (queryText.trim()) {
        query.set("query", queryText.trim());
      }
    } else {
      path = "/api/v1/forum/queues";
      query.set("type", queueTab);
    }

    const response = await fetch(`${path}?${query.toString()}`);
    if (!response.ok) {
      throw new Error(`forum request failed (${response.status})`);
    }
    const payload = (await response.json()) as { threads?: ForumThread[] };
    return Array.isArray(payload.threads) ? payload.threads : [];
  }

  async function loadQueueCounts(): Promise<{ unseen: number; unanswered: number }> {
    if (!selectedTicket || !viewerID) {
      return { unseen: 0, unanswered: 0 };
    }
    const base = new URLSearchParams();
    base.set("ticket", selectedTicket);
    if (selectedRunID) {
      base.set("run_id", selectedRunID);
    }
    base.set("viewer_type", viewerType);
    base.set("viewer_id", viewerID);
    base.set("limit", "200");

    const unseenParams = new URLSearchParams(base);
    unseenParams.set("type", "unseen");
    const unansweredParams = new URLSearchParams(base);
    unansweredParams.set("type", "unanswered");

    const [unseenResponse, unansweredResponse] = await Promise.all([
      fetch(`/api/v1/forum/queues?${unseenParams.toString()}`),
      fetch(`/api/v1/forum/queues?${unansweredParams.toString()}`),
    ]);
    if (!unseenResponse.ok || !unansweredResponse.ok) {
      throw new Error("queue counters request failed");
    }
    const unseenPayload = (await unseenResponse.json()) as { threads?: unknown[] };
    const unansweredPayload = (await unansweredResponse.json()) as { threads?: unknown[] };
    return {
      unseen: Array.isArray(unseenPayload.threads) ? unseenPayload.threads.length : 0,
      unanswered: Array.isArray(unansweredPayload.threads) ? unansweredPayload.threads.length : 0,
    };
  }

  async function refreshThreadDetail(threadID: string) {
    try {
      const response = await fetch(`/api/v1/forum/threads/${encodeURIComponent(threadID)}`);
      if (!response.ok) {
        throw new Error(`thread detail request failed (${response.status})`);
      }
      const payload = (await response.json()) as ForumThreadDetail;
      setSelectedDetail(payload);
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function markThreadSeen(threadID: string) {
    if (!viewerType || !viewerID) {
      return;
    }
    try {
      await fetch(`/api/v1/forum/threads/${encodeURIComponent(threadID)}/seen`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          viewer_type: viewerType,
          viewer_id: viewerID,
          last_seen_event_sequence: selectedDetail?.thread?.last_event_sequence ?? 0,
        }),
      });
      void refreshForumData();
    } catch {
      // best effort in v1
    }
  }

  async function submitReply() {
    if (!selectedThreadID || !replyBody.trim()) {
      return;
    }
    setSavingReply(true);
    try {
      setError("");
      const response = await fetch(`/api/v1/forum/threads/${encodeURIComponent(selectedThreadID)}/posts`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          body: replyBody.trim(),
          actor_type: "human",
          actor_name: viewerID,
        }),
      });
      if (!response.ok) {
        throw new Error(`reply request failed (${response.status})`);
      }
      setReplyBody("");
      await Promise.all([refreshForumData(), refreshThreadDetail(selectedThreadID)]);
      await markThreadSeen(selectedThreadID);
    } catch (err) {
      setError(toErrorString(err));
    } finally {
      setSavingReply(false);
    }
  }

  async function refreshDebug(ticket: string, runID: string) {
    try {
      const query = new URLSearchParams();
      if (ticket) {
        query.set("ticket", ticket);
      }
      if (runID) {
        query.set("run_id", runID);
      }
      query.set("limit", "40");
      const response = await fetch(`/api/v1/forum/debug?${query.toString()}`);
      if (!response.ok) {
        throw new Error(`debug request failed (${response.status})`);
      }
      const payload = (await response.json()) as { debug?: ForumDebugSnapshot };
      setDebugSnapshot(payload.debug ?? null);
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  const timelineRows = useMemo(() => {
    if (!selectedDetail) {
      return [];
    }
    const postsByEventID = new Map(selectedDetail.posts.map((post) => [post.event_id, post]));
    return [...selectedDetail.events]
      .sort((a, b) => a.sequence - b.sequence)
      .map((event) => {
        const matchedPost = postsByEventID.get(event.envelope.event_id);
        return {
          id: `${event.sequence}-${event.envelope.event_id}`,
          sequence: event.sequence,
          eventType: event.envelope.event_type,
          actorType: event.envelope.actor_type || matchedPost?.author_type || "-",
          actorName: event.envelope.actor_name || matchedPost?.author_name || "-",
          occurredAt: event.envelope.occurred_at,
          body: matchedPost?.body || summarizePayload(event.payload_json),
        };
      });
  }, [selectedDetail]);

  return (
    <div className="layout">
      <header className="topbar">
        <h1>metawsm forum explorer</h1>
        <div className="toolbar">
          <button onClick={() => void refreshRuns()} type="button">
            Refresh Runs
          </button>
          <button
            onClick={() => {
              void refreshForumData();
              void refreshDebug(selectedTicket, selectedRunID);
              if (selectedThreadID) {
                void refreshThreadDetail(selectedThreadID);
              }
            }}
            type="button"
          >
            Refresh Forum
          </button>
        </div>
      </header>

      {error ? <div className="error">{error}</div> : null}

      {diagnosticsWarning ? (
        <div className="warning">
          <strong>Diagnostics Warning:</strong> {diagnosticsWarning} <a href="#debug-panel">Open debug health</a>
        </div>
      ) : null}

      <div className="grid">
        <section className="panel runs-panel">
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

        <section className="panel explorer-panel">
          <div className="explorer-header">
            <h2>Threads Explorer {selectedTicket ? `(${selectedTicket})` : ""}</h2>
            <div className="queue-tabs">
              <button
                type="button"
                className={queueTab === "all" ? "chip active" : "chip"}
                onClick={() => setQueueTab("all")}
              >
                All
              </button>
              <button
                type="button"
                className={queueTab === "unseen" ? "chip active" : "chip"}
                onClick={() => setQueueTab("unseen")}
              >
                Unseen ({queueCounts.unseen})
              </button>
              <button
                type="button"
                className={queueTab === "unanswered" ? "chip active" : "chip"}
                onClick={() => setQueueTab("unanswered")}
              >
                Unanswered ({queueCounts.unanswered})
              </button>
            </div>
          </div>

          <div className="filters">
            <input
              type="text"
              placeholder="Search title/body"
              value={queryText}
              onChange={(event) => setQueryText(event.target.value)}
              disabled={queueTab !== "all"}
            />
            <select value={stateFilter} onChange={(event) => setStateFilter(event.target.value)}>
              <option value="">All states</option>
              <option value="new">new</option>
              <option value="triaged">triaged</option>
              <option value="waiting_operator">waiting_operator</option>
              <option value="waiting_human">waiting_human</option>
              <option value="answered">answered</option>
              <option value="closed">closed</option>
            </select>
            <select value={priorityFilter} onChange={(event) => setPriorityFilter(event.target.value)}>
              <option value="">All priorities</option>
              <option value="urgent">urgent</option>
              <option value="high">high</option>
              <option value="normal">normal</option>
              <option value="low">low</option>
            </select>
            <input
              type="text"
              placeholder="assignee"
              value={assigneeFilter}
              onChange={(event) => setAssigneeFilter(event.target.value)}
            />
          </div>

          <div className="viewer-row">
            <select value={viewerType} onChange={(event) => setViewerType(event.target.value)}>
              <option value="human">human viewer</option>
              <option value="agent">agent viewer</option>
            </select>
            <input
              type="text"
              value={viewerID}
              placeholder="viewer identity"
              onChange={(event) => setViewerID(event.target.value)}
            />
          </div>

          {threads.length === 0 ? <p className="muted">No threads for current filters.</p> : null}
          <ul className="list">
            {threads.map((thread) => (
              <li key={thread.thread_id}>
                <button
                  type="button"
                  className={thread.thread_id === selectedThreadID ? "item selected" : "item"}
                  onClick={() => setSelectedThreadID(thread.thread_id)}
                >
                  <strong>{thread.title}</strong>
                  <span>
                    {thread.state} · {thread.priority} · posts={thread.posts_count}
                  </span>
                  <div className="badges">
                    {thread.is_unseen ? <span className="badge unseen">unseen</span> : null}
                    {thread.is_unanswered ? <span className="badge unanswered">unanswered</span> : null}
                    {thread.last_actor_type ? <span className="badge actor">last={thread.last_actor_type}</span> : null}
                  </div>
                  <small>
                    thread={thread.thread_id} assignee={thread.assignee_name || "-"} updated={formatShortTime(thread.updated_at)}
                  </small>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel detail-panel">
          <h2>Thread Detail</h2>
          {!selectedDetail ? <p className="muted">Select a thread to inspect timeline and respond.</p> : null}
          {selectedDetail ? (
            <>
              <div className="detail-meta">
                <strong>{selectedDetail.thread.title}</strong>
                <span>
                  {selectedDetail.thread.state} · {selectedDetail.thread.priority} · ticket={selectedDetail.thread.ticket}
                </span>
                <small>
                  thread={selectedDetail.thread.thread_id} run={selectedDetail.thread.run_id || "-"} assignee={
                    selectedDetail.thread.assignee_name || "-"
                  }
                </small>
              </div>

              <div className="timeline">
                {timelineRows.map((row) => (
                  <div key={row.id} className="timeline-row">
                    <div className="timeline-head">
                      <span>
                        #{row.sequence} {row.eventType}
                      </span>
                      <small>{formatShortTime(row.occurredAt)}</small>
                    </div>
                    <small>
                      {row.actorType}:{row.actorName}
                    </small>
                    {row.body ? <p>{row.body}</p> : <p className="muted">No payload preview</p>}
                  </div>
                ))}
              </div>

              <div className="composer">
                <h3>Respond as Human</h3>
                <textarea
                  value={replyBody}
                  onChange={(event) => setReplyBody(event.target.value)}
                  placeholder="Write a response to this thread..."
                  rows={5}
                />
                <button type="button" onClick={() => void submitReply()} disabled={savingReply || !replyBody.trim()}>
                  {savingReply ? "Sending..." : "Send Reply"}
                </button>
              </div>
            </>
          ) : null}
        </section>

        <section id="debug-panel" className="panel span-3">
          <h2>Forum Debug Health</h2>
          {!debugSnapshot ? <p className="muted">No debug snapshot available.</p> : null}
          {debugSnapshot ? (
            <div className="debug-grid">
              <div className="debug-card">
                <strong>Bus</strong>
                <span>
                  running={String(debugSnapshot.bus.running)} healthy={String(debugSnapshot.bus.healthy)}
                </span>
                <small>
                  stream={debugSnapshot.bus.stream_name || "-"} group={debugSnapshot.bus.consumer_group || "-"} consumer={
                    debugSnapshot.bus.consumer_name || "-"
                  }
                </small>
                {debugSnapshot.bus.health_error ? <small className="error-inline">{debugSnapshot.bus.health_error}</small> : null}
              </div>

              <div className="debug-card">
                <strong>Outbox</strong>
                <span>
                  pending={debugSnapshot.outbox.pending_count} processing={debugSnapshot.outbox.processing_count} failed={
                    debugSnapshot.outbox.failed_count
                  }
                </span>
                <small>oldest_pending_age={debugSnapshot.outbox.oldest_pending_age_seconds}s</small>
              </div>

              <div className="debug-card full">
                <strong>Topic Streams ({debugSnapshot.bus.topics.length})</strong>
                <ul className="list compact">
                  {debugSnapshot.bus.topics.map((topic) => (
                    <li key={topic.topic} className="thread">
                      <span>
                        {topic.topic} stream={topic.stream}
                      </span>
                      <small>
                        handler={String(topic.handler_registered)} subscribed={String(topic.subscribed)} exists={
                          String(topic.stream_exists)
                        } len={topic.stream_length} lag={topic.consumer_group_lag}
                      </small>
                      {topic.topic_error ? <small className="error-inline">{topic.topic_error}</small> : null}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}

function summarizePayload(raw?: string): string {
  if (!raw) {
    return "";
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === "object") {
      return JSON.stringify(parsed);
    }
    return String(parsed);
  } catch {
    return raw;
  }
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
  return value
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
}

function formatShortTime(raw: string): string {
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }
  return parsed.toISOString();
}

function parseForumStreamFrame(raw: unknown): ForumStreamFrame | null {
  if (typeof raw !== "string" || raw.trim() === "") {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    return parsed as ForumStreamFrame;
  } catch {
    return null;
  }
}
