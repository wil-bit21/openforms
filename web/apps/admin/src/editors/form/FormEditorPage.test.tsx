import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { FormEditorPage } from "./FormEditorPage";
import { applyItem, formRecordJson } from "../test/records";
import { sampleForm } from "../test/samples";
import type { FormDef } from "../shared/types";

function mockApi({ source = "ui", exists = true }: { source?: string; exists?: boolean } = {}) {
  const puts: { url: string; body: FormDef }[] = [];
  server.use(
    http.get("/api/v1/forms/job-application", () =>
      exists
        ? HttpResponse.json({ form: formRecordJson(sampleForm(), { source }) })
        : HttpResponse.json({ error: { code: "not_found", message: "definition not found" } }, { status: 404 }),
    ),
    http.get("/api/v1/workflows", () => HttpResponse.json({ items: [] })),
    http.put("/api/v1/forms/:slug", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as FormDef });
      return HttpResponse.json({ item: applyItem("form", "job-application", 4) });
    }),
  );
  return puts;
}

const renderEdit = () => renderWithProviders(<FormEditorPage />, { route: "/forms/job-application/edit", path: "/forms/:slug/edit" });
const fieldButton = (label: string) =>
  within(screen.getByRole("navigation", { name: "Fields" })).getAllByRole("button", { name: new RegExp(`^${label}`) })[0];

describe("FormEditorPage", () => {
  it("loads the form into the editor", async () => {
    mockApi();
    renderEdit();
    expect(await screen.findByRole("heading", { name: "Edit form: Job application" })).toBeInTheDocument();
    expect(fieldButton("Full name")).toBeInTheDocument();
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("warns when the form is managed by the CLI", async () => {
    mockApi({ source: "cli" });
    renderEdit();
    expect(await screen.findByRole("note")).toHaveTextContent("This form is managed in code.");
  });

  it("saves edits with source=ui", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: /^Full name/ }));
    const label = within(screen.getByRole("region", { name: "Field settings" })).getByLabelText("Label");
    await user.clear(label);
    await user.type(label, "Your name");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(puts[0].body.fields[0].label).toBe("Your name");
  });

  it("shows server problems, including ones that match no control", async () => {
    mockApi();
    server.use(
      http.put("/api/v1/forms/:slug", () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "invalid",
              details: [
                { path: "fields[0].label", message: "server says no" },
                { path: "settings.unknownThing", message: "odd server rule" },
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
    const problemList = await screen.findByRole("region", { name: "Problems" });
    expect(within(problemList).getByText("odd server rule")).toBeInTheDocument();
    await user.click(within(problemList).getByRole("button", { name: /server says no/ }));
    expect(within(screen.getByRole("region", { name: "Field settings" })).getByText("server says no")).toBeInTheDocument();
  });

  it("blocks saving while client-side problems exist", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: /^Email/ }));
    const key = within(screen.getByRole("region", { name: "Field settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "name");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Fix \d+ problems? before saving\./)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  });

  it("refuses to create a new form with a taken slug", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderWithProviders(<FormEditorPage />, { route: "/forms/new", path: "/forms/new" });
    expect(await screen.findByRole("heading", { name: "New form" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Slug"), "job-application");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/already exists/)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  });

  it("keeps the visual state when YAML is invalid and applies valid YAML", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit form: Job application" });
    await user.click(screen.getByRole("tab", { name: "YAML" }));
    const textarea = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    expect(textarea.value).toContain("slug: job-application");
    fireEvent.change(textarea, { target: { value: "fields: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Visual" }));
    expect(fieldButton("Full name")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "YAML" }));
    const fresh = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    fireEvent.change(fresh, { target: { value: fresh.value.replace("title: Job application", "title: Renamed") } });
    expect(screen.getByRole("heading", { name: "Edit form: Renamed" })).toBeInTheDocument();
  });

  it("reports a missing form", async () => {
    mockApi({ exists: false });
    renderEdit();
    expect(await screen.findByRole("alert")).toHaveTextContent('Form "job-application" was not found.');
  });
});
