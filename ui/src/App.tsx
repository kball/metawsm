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
  agent_name?: string;
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

type ForumStreamFrame = {
  type?: string;
  events?: unknown[];
};

type BoardKey = "in_progress" | "needs_me" | "recently_completed";
type TopicMode = "ticket" | "run" | "agent";

type BoardBuckets = {
  inProgressNew: ForumThread[];
  inProgressActive: ForumThread[];
  inProgressAwaitingClose: ForumThread[];
  needsMeUnseen: ForumThread[];
  needsMeUnanswered: ForumThread[];
  needsMeAssigned: ForumThread[];
  recentlyClosed: ForumThread[];
};

const EMPTY_BUCKETS: BoardBuckets = {
  inProgressNew: [],
  inProgressActive: [],
  inProgressAwaitingClose: [],
  needsMeUnseen: [],
  needsMeUnanswered: [],
  needsMeAssigned: [],
  recentlyClosed: [],
};

export function App() {
  const [runs, setRuns] = useState<RunSnapshot[]>([]);
  const [runFilter, setRunFilter] = useState("");
  const [ticketFilter, setTicketFilter] = useState("");

  const [activeBoard, setActiveBoard] = useState<BoardKey>("in_progress");
  const [topicMode, setTopicMode] = useState<TopicMode>("ticket");
  const [selectedAgentName, setSelectedAgentName] = useState("");

  const [boardBuckets, setBoardBuckets] = useState<BoardBuckets>(EMPTY_BUCKETS);
  const [selectedThreadID, setSelectedThreadID] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<ForumThreadDetail | null>(null);

  const [queryText, setQueryText] = useState("");
  const [priorityFilter, setPriorityFilter] = useState("");

  const [viewerType, setViewerType] = useState("human");
  const [viewerID, setViewerID] = useState("human:operator");

  const [replyBody, setReplyBody] = useState("");
  const [savingReply, setSavingReply] = useState(false);
  const [questionTicket, setQuestionTicket] = useState("");
  const [questionTitle, setQuestionTitle] = useState("");
  const [questionBody, setQuestionBody] = useState("");
  const [questionPriority, setQuestionPriority] = useState("normal");
  const [savingQuestion, setSavingQuestion] = useState(false);

  const [showDiagnostics, setShowDiagnostics] = useState(false);
  const [debugSnapshot, setDebugSnapshot] = useState<ForumDebugSnapshot | null>(null);
  const [error, setError] = useState("");
  const streamRefreshTimer = useRef<number | null>(null);
  const selectedThreadRef = useRef("");

  const boardThreads = useMemo(
    () => [
      ...boardBuckets.inProgressNew,
      ...boardBuckets.inProgressActive,
      ...boardBuckets.inProgressAwaitingClose,
      ...boardBuckets.needsMeUnseen,
      ...boardBuckets.needsMeUnanswered,
      ...boardBuckets.needsMeAssigned,
      ...boardBuckets.recentlyClosed,
    ],
    [boardBuckets],
  );

  const availableAgents = useMemo(() => {
    const values = new Set<string>();
    for (const thread of boardThreads) {
      const agentName = (thread.agent_name || "").trim();
      if (agentName) {
        values.add(agentName);
      }
    }
    return [...values].sort((a, b) => a.localeCompare(b));
  }, [boardThreads]);

  const boardCounts = useMemo(() => {
    const inProgress =
      boardBuckets.inProgressNew.length +
      boardBuckets.inProgressActive.length +
      boardBuckets.inProgressAwaitingClose.length;
    const needsMeUnique = new Set<string>([
      ...boardBuckets.needsMeUnseen.map((thread) => thread.thread_id),
      ...boardBuckets.needsMeUnanswered.map((thread) => thread.thread_id),
      ...boardBuckets.needsMeAssigned.map((thread) => thread.thread_id),
    ]);
    return {
      inProgress,
      needsMe: needsMeUnique.size,
      recentlyCompleted: boardBuckets.recentlyClosed.length,
    };
  }, [boardBuckets]);

  const knownTickets = useMemo(() => {
    const set = new Set<string>();
    for (const run of runs) {
      for (const ticket of run.tickets) {
        const normalized = ticket.trim();
        if (normalized) {
          set.add(normalized);
        }
      }
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  }, [runs]);

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

  const canSubmitQuestion =
    viewerType === "human" &&
    !!questionTicket.trim() &&
    !!viewerID.trim() &&
    !!questionTitle.trim() &&
    !!questionBody.trim() &&
    !savingQuestion;

  const boardScopeLabel = useMemo(() => {
    const parts: string[] = [];
    if (ticketFilter.trim()) {
      parts.push(`ticket:${ticketFilter.trim()}`);
    }
    if (runFilter.trim()) {
      parts.push(`run:${runFilter.trim()}`);
    }
    if (topicMode === "agent") {
      parts.push(selectedAgentName ? `agent:${selectedAgentName}` : "agent:all");
    }
    return parts.length > 0 ? parts.join(" · ") : "global";
  }, [ticketFilter, runFilter, topicMode, selectedAgentName]);

  useEffect(() => {
    void refreshRuns();
  }, []);

  useEffect(() => {
    void refreshForumData();
  }, [ticketFilter, runFilter, topicMode, selectedAgentName, queryText, priorityFilter, viewerType, viewerID]);

  useEffect(() => {
    if (topicMode !== "agent") {
      return;
    }
    if (!selectedAgentName) {
      return;
    }
    if (!availableAgents.includes(selectedAgentName)) {
      setSelectedAgentName("");
    }
  }, [topicMode, selectedAgentName, availableAgents]);

  useEffect(() => {
    if (!questionTicket.trim() && ticketFilter.trim()) {
      setQuestionTicket(ticketFilter.trim());
    }
  }, [questionTicket, ticketFilter]);

  useEffect(() => {
    void refreshDebug(ticketFilter, runFilter);
    const interval = window.setInterval(() => {
      void refreshDebug(questionTicket.trim(), runFilter);
    }, 15000);
    return () => {
      window.clearInterval(interval);
    };
  }, [ticketFilter, runFilter]);

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
    const socketURL = new URL("/api/v1/forum/stream", window.location.origin);
    socketURL.protocol = socketURL.protocol.replace("http", "ws");
    if (ticketFilter.trim()) {
      socketURL.searchParams.set("ticket", ticketFilter.trim());
    }
    if (runFilter.trim()) {
      socketURL.searchParams.set("run_id", runFilter.trim());
    }

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
  }, [ticketFilter, runFilter]);

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
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function refreshForumData() {
    const scope = resolveScope(ticketFilter, runFilter);

    try {
      setError("");

      const [allRows, closedRows, unseenRows, unansweredRows] = await Promise.all([
        fetchSearchThreads(scope, {
          queryText,
          priorityFilter,
          viewerType,
          viewerID,
        }),
        fetchSearchThreads(scope, {
          state: "closed",
          queryText,
          priorityFilter,
          viewerType,
          viewerID,
        }),
        fetchQueueThreads(scope, {
          queueType: "unseen",
          priorityFilter,
          viewerType,
          viewerID,
        }),
        fetchQueueThreads(scope, {
          queueType: "unanswered",
          priorityFilter,
          viewerType,
          viewerID,
        }),
      ]);

      const scopedAllRows = applyTopicAgentFilter(allRows, topicMode, selectedAgentName);
      const scopedClosedRows = applyTopicAgentFilter(closedRows, topicMode, selectedAgentName);
      const scopedUnseenRows = applyTopicAgentFilter(applyQueryFilter(unseenRows, queryText), topicMode, selectedAgentName);
      const scopedUnansweredRows = applyTopicAgentFilter(
        applyQueryFilter(unansweredRows, queryText),
        topicMode,
        selectedAgentName,
      );

      const openRows = scopedAllRows.filter((thread) => !isClosedState(thread.state));
      const inProgressNew = openRows.filter((thread) => isState(thread.state, ["new"]));
      const inProgressActive = openRows.filter((thread) =>
        isState(thread.state, ["triaged", "waiting_operator", "waiting_human"]),
      );
      const inProgressAwaitingClose = openRows.filter((thread) => isState(thread.state, ["answered"]));

      const inferredAssigneeIDs = inferViewerAssigneeIDs(viewerID);
      const needsMeAssigned = scopedAllRows.filter((thread) => assigneeMatchesViewer(thread.assignee_name, inferredAssigneeIDs));

      const nextBuckets: BoardBuckets = {
        inProgressNew,
        inProgressActive,
        inProgressAwaitingClose,
        needsMeUnseen: scopedUnseenRows,
        needsMeUnanswered: scopedUnansweredRows,
        needsMeAssigned,
        recentlyClosed: scopedClosedRows,
      };
      setBoardBuckets(nextBuckets);

      if (selectedThreadID) {
        const visibleIDs = new Set(
          [
            ...nextBuckets.inProgressNew,
            ...nextBuckets.inProgressActive,
            ...nextBuckets.inProgressAwaitingClose,
            ...nextBuckets.needsMeUnseen,
            ...nextBuckets.needsMeUnanswered,
            ...nextBuckets.needsMeAssigned,
            ...nextBuckets.recentlyClosed,
          ].map((thread) => thread.thread_id),
        );
        if (!visibleIDs.has(selectedThreadID)) {
          setSelectedThreadID("");
        }
      }
    } catch (err) {
      setError(toErrorString(err));
    }
  }

  async function fetchSearchThreads(
    scope: { ticket?: string; run_id?: string },
    options: {
      state?: string;
      queryText: string;
      priorityFilter: string;
      viewerType: string;
      viewerID: string;
    },
  ): Promise<ForumThread[]> {
    const query = new URLSearchParams();
    if (scope.ticket) {
      query.set("ticket", scope.ticket);
    }
    if (scope.run_id) {
      query.set("run_id", scope.run_id);
    }
    if (options.state) {
      query.set("state", options.state);
    }
    if (options.queryText.trim()) {
      query.set("query", options.queryText.trim());
    }
    if (options.priorityFilter) {
      query.set("priority", options.priorityFilter);
    }
    if (options.viewerType) {
      query.set("viewer_type", options.viewerType);
    }
    if (options.viewerID) {
      query.set("viewer_id", options.viewerID);
    }
    query.set("limit", "300");

    const response = await fetch(`/api/v1/forum/search?${query.toString()}`);
    if (!response.ok) {
      throw new Error(`forum search request failed (${response.status})`);
    }
    const payload = (await response.json()) as { threads?: ForumThread[] };
    return Array.isArray(payload.threads) ? payload.threads : [];
  }

  async function fetchQueueThreads(
    scope: { ticket?: string; run_id?: string },
    options: {
      queueType: "unseen" | "unanswered";
      priorityFilter: string;
      viewerType: string;
      viewerID: string;
    },
  ): Promise<ForumThread[]> {
    const query = new URLSearchParams();
    if (scope.ticket) {
      query.set("ticket", scope.ticket);
    }
    if (scope.run_id) {
      query.set("run_id", scope.run_id);
    }
    query.set("type", options.queueType);
    if (options.priorityFilter) {
      query.set("priority", options.priorityFilter);
    }
    if (options.viewerType) {
      query.set("viewer_type", options.viewerType);
    }
    if (options.viewerID) {
      query.set("viewer_id", options.viewerID);
    }
    query.set("limit", "300");

    const response = await fetch(`/api/v1/forum/queues?${query.toString()}`);
    if (!response.ok) {
      throw new Error(`forum queue request failed (${response.status})`);
    }
    const payload = (await response.json()) as { threads?: ForumThread[] };
    return Array.isArray(payload.threads) ? payload.threads : [];
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

  async function submitQuestion() {
    if (!canSubmitQuestion) {
      return;
    }
    setSavingQuestion(true);
    try {
      setError("");
      const response = await fetch("/api/v1/forum/threads", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ticket: questionTicket.trim(),
          run_id: runFilter.trim(),
          title: questionTitle.trim(),
          body: questionBody.trim(),
          priority: questionPriority,
          actor_type: "human",
          actor_name: viewerID.trim(),
        }),
      });
      if (!response.ok) {
        throw new Error(`question request failed (${response.status})`);
      }
      const payload = (await response.json()) as { thread?: ForumThread };
      const createdThreadID = payload.thread?.thread_id ?? "";

      setTicketFilter(questionTicket.trim());
      setQuestionTitle("");
      setQuestionBody("");
      setQuestionPriority("normal");
      setActiveBoard("in_progress");

      await refreshForumData();
      if (createdThreadID) {
        setSelectedThreadID(createdThreadID);
        await refreshThreadDetail(createdThreadID);
        await markThreadSeen(createdThreadID);
      }
      void refreshDebug(ticketFilter, runFilter);
    } catch (err) {
      setError(toErrorString(err));
    } finally {
      setSavingQuestion(false);
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
              void refreshDebug(ticketFilter, runFilter);
              if (selectedThreadID) {
                void refreshThreadDetail(selectedThreadID);
              }
            }}
            type="button"
          >
            Refresh Forum
          </button>
          <button type="button" onClick={() => setShowDiagnostics((value) => !value)}>
            {showDiagnostics ? "Hide System Health" : "Show System Health"}
          </button>
        </div>
      </header>

      {error ? <div className="error">{error}</div> : null}

      {diagnosticsWarning ? (
        <div className="warning">
          <strong>Diagnostics Warning:</strong> {diagnosticsWarning}{" "}
          <button
            type="button"
            className="link-button"
            onClick={() => {
              setShowDiagnostics(true);
            }}
          >
            Open system health
          </button>
        </div>
      ) : null}

      <div className="grid">
        <section className="panel explorer-panel">
          <div className="explorer-header">
            <h2>Threads Explorer</h2>
            <span className="scope">scope: {boardScopeLabel}</span>
          </div>

          <div className="board-tabs">
            <button
              type="button"
              className={activeBoard === "in_progress" ? "chip active" : "chip"}
              onClick={() => setActiveBoard("in_progress")}
            >
              In Progress ({boardCounts.inProgress})
            </button>
            <button
              type="button"
              className={activeBoard === "needs_me" ? "chip active" : "chip"}
              onClick={() => setActiveBoard("needs_me")}
            >
              Needs Me ({boardCounts.needsMe})
            </button>
            <button
              type="button"
              className={activeBoard === "recently_completed" ? "chip active" : "chip"}
              onClick={() => setActiveBoard("recently_completed")}
            >
              Recently Completed ({boardCounts.recentlyCompleted})
            </button>
          </div>

          <div className="scope-filters">
            <select value={ticketFilter} onChange={(event) => setTicketFilter(event.target.value)}>
              <option value="">All tickets</option>
              {knownTickets.map((ticket) => (
                <option key={ticket} value={ticket}>
                  {ticket}
                </option>
              ))}
            </select>
            <select value={runFilter} onChange={(event) => setRunFilter(event.target.value)}>
              <option value="">All runs</option>
              {runs.map((run) => (
                <option key={run.run_id} value={run.run_id}>
                  {run.run_id}
                </option>
              ))}
            </select>
          </div>

          <div className="topic-tabs">
            <span className="topic-label">Topic area:</span>
            <button
              type="button"
              className={topicMode === "ticket" ? "chip active" : "chip"}
              onClick={() => setTopicMode("ticket")}
            >
              Ticket first
            </button>
            <button
              type="button"
              className={topicMode === "run" ? "chip active" : "chip"}
              onClick={() => setTopicMode("run")}
            >
              Run
            </button>
            <button
              type="button"
              className={topicMode === "agent" ? "chip active" : "chip"}
              onClick={() => setTopicMode("agent")}
            >
              Agent
            </button>
          </div>

          {topicMode === "agent" ? (
            <div className="filters single">
              <select value={selectedAgentName} onChange={(event) => setSelectedAgentName(event.target.value)}>
                <option value="">All agents</option>
                {availableAgents.map((agentName) => (
                  <option value={agentName} key={agentName}>
                    {agentName}
                  </option>
                ))}
              </select>
            </div>
          ) : null}

          <div className="filters">
            <input
              type="text"
              placeholder="Search title/body"
              value={queryText}
              onChange={(event) => setQueryText(event.target.value)}
            />
            <select value={priorityFilter} onChange={(event) => setPriorityFilter(event.target.value)}>
              <option value="">All priorities</option>
              <option value="urgent">urgent</option>
              <option value="high">high</option>
              <option value="normal">normal</option>
              <option value="low">low</option>
            </select>
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

          {activeBoard === "in_progress" ? (
            <div className="board-columns">
              <BoardLane
                title="New / Triage"
                rows={boardBuckets.inProgressNew}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No new threads."
              />
              <BoardLane
                title="Active"
                rows={boardBuckets.inProgressActive}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No active threads."
              />
              <BoardLane
                title="Awaiting Close"
                rows={boardBuckets.inProgressAwaitingClose}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No answered threads awaiting close."
              />
            </div>
          ) : null}

          {activeBoard === "needs_me" ? (
            <div className="board-columns">
              <BoardLane
                title="Unseen for Me"
                rows={boardBuckets.needsMeUnseen}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No unseen threads."
              />
              <BoardLane
                title="Needs Human/Operator Response"
                rows={boardBuckets.needsMeUnanswered}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No unanswered threads."
              />
              <BoardLane
                title="Assigned to Me"
                rows={boardBuckets.needsMeAssigned}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No threads assigned to current viewer."
              />
            </div>
          ) : null}

          {activeBoard === "recently_completed" ? (
            <div className="board-columns single">
              <BoardLane
                title="Recently Closed"
                rows={boardBuckets.recentlyClosed}
                selectedThreadID={selectedThreadID}
                onSelectThread={setSelectedThreadID}
                emptyLabel="No recently closed threads."
              />
            </div>
          ) : null}
        </section>

        <section className="panel detail-panel">
          <h2>Thread Detail</h2>
          <div className="composer">
            <h3>Ask as Human</h3>
            <input
              type="text"
              placeholder="Question ticket (e.g. METAWSM-011)"
              value={questionTicket}
              onChange={(event) => setQuestionTicket(event.target.value)}
            />
            <input
              type="text"
              placeholder="Question title"
              value={questionTitle}
              onChange={(event) => setQuestionTitle(event.target.value)}
            />
            <select value={questionPriority} onChange={(event) => setQuestionPriority(event.target.value)}>
              <option value="urgent">urgent</option>
              <option value="high">high</option>
              <option value="normal">normal</option>
              <option value="low">low</option>
            </select>
            <textarea
              value={questionBody}
              onChange={(event) => setQuestionBody(event.target.value)}
              placeholder="Describe the question for the forum..."
              rows={4}
            />
            <button type="button" onClick={() => void submitQuestion()} disabled={!canSubmitQuestion}>
              {savingQuestion ? "Asking..." : "Ask Question"}
            </button>
            {viewerType !== "human" ? <small className="muted">Set viewer type to human to open a question.</small> : null}
          </div>

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
          <div className="explorer-header">
            <h2>System Health</h2>
            <button type="button" onClick={() => setShowDiagnostics((value) => !value)}>
              {showDiagnostics ? "Collapse" : "Expand"}
            </button>
          </div>

          {!showDiagnostics ? <p className="muted">Diagnostics are collapsed by default.</p> : null}

          {showDiagnostics ? (
            <>
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
                    {debugSnapshot.bus.health_error ? (
                      <small className="error-inline">{debugSnapshot.bus.health_error}</small>
                    ) : null}
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
            </>
          ) : null}
        </section>
      </div>
    </div>
  );
}

type BoardLaneProps = {
  title: string;
  rows: ForumThread[];
  selectedThreadID: string;
  onSelectThread: (threadID: string) => void;
  emptyLabel: string;
};

function BoardLane({ title, rows, selectedThreadID, onSelectThread, emptyLabel }: BoardLaneProps) {
  return (
    <section className="board-lane">
      <div className="lane-header">
        <h3>{title}</h3>
        <span className="lane-count">{rows.length}</span>
      </div>
      {rows.length === 0 ? <p className="muted">{emptyLabel}</p> : null}
      <ul className="list">
        {rows.map((thread) => (
          <li key={thread.thread_id}>
            <button
              type="button"
              className={thread.thread_id === selectedThreadID ? "item selected" : "item"}
              onClick={() => onSelectThread(thread.thread_id)}
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
  );
}

function resolveScope(ticketFilter: string, runFilter: string) {
  const ticket = ticketFilter.trim();
  const runID = runFilter.trim();
  return {
    ticket: ticket || undefined,
    run_id: runID || undefined,
  };
}

function isState(value: string, expected: string[]): boolean {
  const normalized = value.trim().toLowerCase();
  return expected.some((candidate) => candidate === normalized);
}

function isClosedState(value: string): boolean {
  return isState(value, ["closed"]);
}

function applyQueryFilter(rows: ForumThread[], queryText: string): ForumThread[] {
  const query = queryText.trim().toLowerCase();
  if (!query) {
    return rows;
  }
  return rows.filter((thread) => {
    return (
      thread.title.toLowerCase().includes(query) ||
      thread.thread_id.toLowerCase().includes(query) ||
      (thread.assignee_name || "").toLowerCase().includes(query) ||
      (thread.agent_name || "").toLowerCase().includes(query)
    );
  });
}

function applyTopicAgentFilter(rows: ForumThread[], topicMode: TopicMode, selectedAgentName: string): ForumThread[] {
  if (topicMode !== "agent") {
    return rows;
  }
  const agent = selectedAgentName.trim().toLowerCase();
  if (!agent) {
    return rows;
  }
  return rows.filter((thread) => (thread.agent_name || "").trim().toLowerCase() === agent);
}

function inferViewerAssigneeIDs(viewerID: string): string[] {
  const normalized = viewerID.trim().toLowerCase();
  if (!normalized) {
    return [];
  }
  const set = new Set<string>();
  set.add(normalized);

  const parts = normalized.split(":").map((part) => part.trim()).filter((part) => part.length > 0);
  if (parts.length >= 2) {
    set.add(parts[1]);
  }
  if (parts.length >= 3) {
    set.add(parts[parts.length - 1]);
  }

  return [...set];
}

function assigneeMatchesViewer(assigneeName: string, inferredIDs: string[]): boolean {
  const assignee = assigneeName.trim().toLowerCase();
  if (!assignee || inferredIDs.length === 0) {
    return false;
  }
  if (inferredIDs.includes(assignee)) {
    return true;
  }
  const parts = assignee.split(":").map((part) => part.trim()).filter((part) => part.length > 0);
  if (parts.length >= 2 && inferredIDs.includes(parts[1])) {
    return true;
  }
  if (parts.length >= 3 && inferredIDs.includes(parts[parts.length - 1])) {
    return true;
  }
  return false;
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
