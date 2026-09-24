import { render, screen, waitFor } from "@testing-library/react";
import type { PublicStatus, State } from "@openforms/sdk";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { StatusTracker } from "./StatusTracker.js";
import { API, newClient } from "./test-fixtures.js";

const states: State[] = [
  { key: "new", label: "New", color: "gray" },
  { key: "screening", label: "Screening", color: "blue" },
  { key: "hired", label: "Hired", color: "green", terminal: true },
  { key: "rejected", label: "Rejected", color: "red", terminal: true },
];

function snapshot(state: string, history: string[], terminal = false): PublicStatus {
  const label = (key: string) => states.find((s) => s.key === key)!.label;
  return {
    id: "sub-1",
    formTitle: "Job application",
    state,
    stateLabel: label(state),
    terminal,
    states,
    history: history.map((key, i) => ({ state: key, label: label(key), at: `2026-09-2${3 + i}T10:00:00Z` })),
    createdAt: "2026-09-23T10:00:00Z",
  };
}

let calls = 0;
let sequence: PublicStatus[] = [];
let tokens: string[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/submissions/:id`, ({ request }) => {
    tokens.push(new URL(request.url).searchParams.get("token") ?? "");
    const next = sequence[Math.min(calls, sequence.length - 1)];
    calls += 1;
    return next
      ? HttpResponse.json(next)
      : HttpResponse.json({ error: { code: "not_found", message: "not found" } }, { status: 404 });
  }),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  calls = 0;
  sequence = [];
  tokens = [];
});
afterAll(() => server.close());

const client = newClient();
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe("<StatusTracker>", () => {
  it("renders the chain with the current and completed steps", async () => {
    sequence = [snapshot("screening", ["new", "screening"])];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={10_000} />);
    expect(await screen.findByRole("heading", { name: "Job application" })).toBeInTheDocument();
    expect(screen.getByText("Screening", { selector: "strong" })).toBeInTheDocument();

    const current = screen.getByText("Screening", { selector: "li.of-step" });
    expect(current).toHaveAttribute("aria-current", "step");
    expect(current).toHaveClass("of-step-current", "of-color-blue");
    expect(screen.getByText("New", { selector: "li.of-step" })).toHaveClass("of-step-done");
    // Terminal states are only shown once reached.
    expect(screen.queryByText("Hired", { selector: "li.of-step" })).not.toBeInTheDocument();
    expect(tokens[0]).toBe("tok-123");
  });

  it("polls until the submission reaches a terminal state, then stops", async () => {
    sequence = [snapshot("new", ["new"]), snapshot("screening", ["new", "screening"]), snapshot("hired", ["new", "screening", "hired"], true)];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    expect(await screen.findByText("Hired", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByText("Hired", { selector: "li.of-step" })).toHaveAttribute("aria-current", "step");
    const seen = calls;
    await sleep(120);
    expect(calls).toBe(seen);
    expect(seen).toBe(3);
  });

  it("stops polling when unmounted", async () => {
    sequence = [snapshot("new", ["new"])];
    const { unmount } = render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    await screen.findByText("New", { selector: "strong" });
    unmount();
    const seen = calls;
    await sleep(100);
    expect(calls).toBe(seen);
  });

  it("explains an unknown submission or bad token and does not retry", async () => {
    sequence = [];
    render(<StatusTracker client={client} submissionId="sub-x" token="wrong" pollMs={20} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("We couldn't find this submission.");
    await sleep(100);
    expect(calls).toBe(1);
  });

  it("keeps the last status and retries after a transient failure", async () => {
    sequence = [snapshot("new", ["new"])];
    let failNext = false;
    server.use(
      http.get(`${API}/api/v1/public/submissions/:id`, () => {
        calls += 1;
        if (calls === 2) failNext = true;
        if (failNext && calls === 2) return new HttpResponse("upstream down", { status: 502 });
        return HttpResponse.json(calls >= 3 ? snapshot("hired", ["new", "hired"], true) : snapshot("new", ["new"]));
      }),
    );
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    await screen.findByText("New", { selector: "strong" });
    await waitFor(() => expect(screen.getByText("Hired", { selector: "strong" })).toBeInTheDocument());
    expect(calls).toBe(3);
  });

  it("lists the history with timestamps", async () => {
    sequence = [snapshot("screening", ["new", "screening"])];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={10_000} />);
    const history = await screen.findByRole("list", { name: "History" });
    const times = history.querySelectorAll("time");
    expect(times).toHaveLength(2);
    expect(times[0]).toHaveAttribute("dateTime", "2026-09-23T10:00:00Z");
  });
});
