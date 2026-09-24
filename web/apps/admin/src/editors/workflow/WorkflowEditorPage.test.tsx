import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { WorkflowEditorPage } from "./WorkflowEditorPage";
import { applyItem, workflowRecordJson } from "../test/records";
import { sampleWorkflow } from "../test/samples";
import type { WorkflowDef } from "../shared/types";

vi.mock("@xyflow/react", () => import("../test/xyflowMock"));

function mockApi({ source = "ui" }: { source?: string } = {}) {
  const puts: { url: string; body: WorkflowDef }[] = [];
  server.use(
    http.get("/api/v1/workflows/hiring", () => HttpResponse.json({ workflow: workflowRecordJson(sampleWorkflow(), { source }) })),
    http.put("/api/v1/workflows/:slug", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as WorkflowDef });
      return HttpResponse.json({ item: applyItem("workflow", "hiring", 4) });
    }),
  );
  return puts;
}

const renderEdit = () => renderWithProviders(<WorkflowEditorPage />, { route: "/workflows/hiring/edit", path: "/workflows/:slug/edit" });
const outline = () => screen.getByRole("navigation", { name: "Workflow outline" });

describe("WorkflowEditorPage", () => {
  it("loads the workflow and shows the CLI banner when code-managed", async () => {
    mockApi({ source: "cli" });
    renderEdit();
    expect(await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" })).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent("This workflow is managed in code.");
  });

  it("selects a state from the diagram", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: "node hired" }));
    expect(screen.getByRole("heading", { name: "State: Hired" })).toBeInTheDocument();
  });

  it("renames a state and saves the updated transitions with source=ui", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" });
    await user.click(within(outline()).getByRole("button", { name: /^Screening/ }));
    const key = within(screen.getByRole("region", { name: "State settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "review");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(puts[0].body.states[1].key).toBe("review");
    expect(puts[0].body.transitions[0].to).toBe("review");
  });

  it("maps server problems to the transition and lists unmapped ones", async () => {
    mockApi();
    server.use(
      http.put("/api/v1/workflows/:slug", () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "invalid",
              details: [
                { path: "transitions[1].guard.requireFields[0]", message: "server rejects field" },
                { path: "something.else", message: "unmapped server rule" },
              ],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: "Save" }));
    const list = await screen.findByRole("region", { name: "Problems" });
    expect(within(list).getByText("unmapped server rule")).toBeInTheDocument();
    await user.click(within(list).getByRole("button", { name: /server rejects field/ }));
    expect(screen.getByRole("heading", { name: "Transition: Hire" })).toBeInTheDocument();
  });

  it("refuses to create a new workflow with a taken slug", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderWithProviders(<WorkflowEditorPage />, { route: "/workflows/new", path: "/workflows/new" });
    expect(await screen.findByRole("heading", { name: "New workflow" })).toBeInTheDocument();
    await user.type(within(screen.getByRole("region", { name: "Workflow settings" })).getByLabelText("Slug"), "hiring");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect((await screen.findAllByText(/already exists/)).length).toBeGreaterThan(0);
    expect(puts).toHaveLength(0);
  });

  it("survives invalid YAML", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" });
    await user.click(screen.getByRole("tab", { name: "YAML" }));
    fireEvent.change(screen.getByLabelText("YAML"), { target: { value: "states: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Visual" }));
    expect(within(outline()).getByRole("button", { name: /^Screening/ })).toBeInTheDocument();
  });
});
