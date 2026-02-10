import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { App } from "./App";

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  readonly close = vi.fn();

  constructor(public readonly url: string | URL) {
    MockWebSocket.instances.push(this);
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("App ask-question flow", () => {
  let createdThread = false;
  let createdThreadID = "fthr-new-1";
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    createdThread = false;
    createdThreadID = "fthr-new-1";
    MockWebSocket.instances = [];

    fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = (init?.method || "GET").toUpperCase();

      if (url === "/api/v1/runs" && method === "GET") {
        return jsonResponse({
          runs: [{ run_id: "run-1", status: "running", tickets: ["METAWSM-011"], pending_guidance: [] }],
        });
      }
      if (url.startsWith("/api/v1/forum/search") && method === "GET") {
        return jsonResponse({
          threads: createdThread
            ? [
                {
                  thread_id: createdThreadID,
                  ticket: "METAWSM-011",
                  run_id: "run-1",
                  title: "Need a human decision",
                  state: "new",
                  priority: "normal",
                  assignee_name: "",
                  posts_count: 1,
                  updated_at: new Date().toISOString(),
                  opened_at: new Date().toISOString(),
                },
              ]
            : [],
        });
      }
      if (url.startsWith("/api/v1/forum/queues") && method === "GET") {
        return jsonResponse({ threads: [] });
      }
      if (url.startsWith("/api/v1/forum/debug") && method === "GET") {
        return jsonResponse({ debug: null });
      }
      if (url === "/api/v1/forum/threads" && method === "POST") {
        createdThread = true;
        const parsed = init?.body ? (JSON.parse(String(init.body)) as { thread_id?: string }) : {};
        if (typeof parsed.thread_id === "string" && parsed.thread_id.trim() !== "") {
          createdThreadID = parsed.thread_id.trim();
        }
        return jsonResponse({
          thread: {
            thread_id: createdThreadID,
            ticket: "METAWSM-011",
            run_id: "run-1",
            title: "Need a human decision",
            state: "new",
            priority: "normal",
            assignee_name: "",
            posts_count: 1,
            updated_at: new Date().toISOString(),
            opened_at: new Date().toISOString(),
          },
        });
      }
      if (url === `/api/v1/forum/threads/${encodeURIComponent(createdThreadID)}` && method === "GET") {
        return jsonResponse({
          thread: {
            thread_id: createdThreadID,
            ticket: "METAWSM-011",
            run_id: "run-1",
            title: "Need a human decision",
            state: "new",
            priority: "normal",
            assignee_name: "",
            posts_count: 1,
            updated_at: new Date().toISOString(),
            opened_at: new Date().toISOString(),
          },
          posts: [],
          events: [],
        });
      }
      if (url.endsWith(`/api/v1/forum/threads/${encodeURIComponent(createdThreadID)}/seen`) && method === "POST") {
        return jsonResponse({
          seen: {
            thread_id: createdThreadID,
            viewer_type: "human",
            viewer_id: "human:operator",
            last_seen_event_sequence: 0,
            updated_at: new Date().toISOString(),
          },
        });
      }
      return jsonResponse({});
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps Ask Question disabled until required fields are populated", async () => {
    render(<App />);
    await screen.findByText("Threads Explorer");

    const askButton = screen.getByRole("button", { name: "Ask Question" });
    const ticketInput = screen.getByPlaceholderText("Question ticket (e.g. METAWSM-011)");
    const titleInput = screen.getByPlaceholderText("Question title");
    const bodyInput = screen.getByPlaceholderText("Describe the question for the forum...");

    expect(askButton).toBeDisabled();

    fireEvent.change(ticketInput, { target: { value: "METAWSM-011" } });
    expect(askButton).toBeDisabled();

    fireEvent.change(titleInput, { target: { value: "Need direction" } });
    expect(askButton).toBeDisabled();

    fireEvent.change(bodyInput, { target: { value: "Should we proceed with the migration?" } });
    expect(askButton).toBeEnabled();
  });

  it("submits ask-question payload and clears inputs on success", async () => {
    render(<App />);
    await screen.findByText("Threads Explorer");

    const ticketInput = screen.getByPlaceholderText("Question ticket (e.g. METAWSM-011)");
    const titleInput = screen.getByPlaceholderText("Question title");
    const bodyInput = screen.getByPlaceholderText("Describe the question for the forum...");
    const askButton = screen.getByRole("button", { name: "Ask Question" });

    fireEvent.change(ticketInput, { target: { value: "METAWSM-011" } });
    fireEvent.change(titleInput, { target: { value: "Need decision on retry policy" } });
    fireEvent.change(bodyInput, { target: { value: "Should retries cap at 3 attempts?" } });
    fireEvent.click(askButton);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/forum/threads",
        expect.objectContaining({
          method: "POST",
          headers: { "Content-Type": "application/json" },
        }),
      );
    });

    const createCall = fetchMock.mock.calls.find((call) => call[0] === "/api/v1/forum/threads");
    expect(createCall).toBeDefined();
    const payload = JSON.parse(String(createCall?.[1]?.body)) as {
      ticket: string;
      run_id: string;
      title: string;
      body: string;
      actor_type: string;
      actor_name: string;
      priority: string;
    };
    expect(payload.ticket).toBe("METAWSM-011");
    expect(payload.run_id).toBe("");
    expect(payload.actor_type).toBe("human");
    expect(payload.actor_name).toBe("human:operator");
    expect(payload.priority).toBe("normal");

    await waitFor(() => {
      expect(ticketInput).toHaveValue("METAWSM-011");
      expect(titleInput).toHaveValue("");
      expect(bodyInput).toHaveValue("");
    });
  });
});
