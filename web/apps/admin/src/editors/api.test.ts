import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../test/server";
import { definitionExists, listVersions, listWorkflowSlugs, loadForm, loadVersion, loadWorkflow, saveDefinition } from "./api";
import { formRecordJson, versionList, workflowRecordJson } from "./test/records";
import { sampleForm, sampleWorkflow } from "./test/samples";

describe("editor SDK adapter", () => {
  it("loads a form record and unwraps definition, source and version", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson(sampleForm(), { source: "cli", version: 7 }) })));
    await expect(loadForm("job-application")).resolves.toEqual({ definition: sampleForm(), source: "cli", version: 7 });
  });

  it("loads a workflow record", async () => {
    server.use(http.get("/api/v1/workflows/hiring", () => HttpResponse.json({ workflow: workflowRecordJson() })));
    const loaded = await loadWorkflow("hiring");
    expect(loaded.definition).toEqual(sampleWorkflow());
  });

  it("saves with source=ui and returns the new version", async () => {
    let url = "";
    let body: unknown;
    server.use(
      http.put("/api/v1/forms/:slug", async ({ request }) => {
        url = request.url;
        body = await request.json();
        return HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 8, changed: true, created: false } });
      }),
    );
    await expect(saveDefinition("form", sampleForm())).resolves.toEqual({ version: 8, changed: true });
    expect(new URL(url).pathname).toBe("/api/v1/forms/job-application");
    expect(new URL(url).searchParams.get("source")).toBe("ui");
    expect(body).toEqual(sampleForm());
  });

  it("saves workflows to the workflow endpoint", async () => {
    let path = "";
    server.use(
      http.put("/api/v1/workflows/:slug", ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.json({ item: { kind: "workflow", slug: "hiring", version: 2, changed: false, created: false } });
      }),
    );
    await expect(saveDefinition("workflow", sampleWorkflow())).resolves.toEqual({ version: 2, changed: false });
    expect(path).toBe("/api/v1/workflows/hiring");
  });

  it("definitionExists maps 404 to false, 200 to true and rethrows other errors", async () => {
    server.use(
      http.get("/api/v1/forms/missing", () => HttpResponse.json({ error: { code: "not_found", message: "definition not found" } }, { status: 404 })),
      http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson() })),
      http.get("/api/v1/forms/broken", () => HttpResponse.json({ error: { code: "internal", message: "boom" } }, { status: 500 })),
    );
    await expect(definitionExists("form", "missing")).resolves.toBe(false);
    await expect(definitionExists("form", "job-application")).resolves.toBe(true);
    await expect(definitionExists("form", "broken")).rejects.toBeTruthy();
  });

  it("lists versions and loads a specific version", async () => {
    server.use(
      http.get("/api/v1/forms/job-application/versions", () => HttpResponse.json({ items: versionList([2, 1], "cli") })),
      http.get("/api/v1/forms/job-application/versions/1", () =>
        HttpResponse.json({ form: formRecordJson({ ...sampleForm(), title: "Old" }, { version: 1 }) }),
      ),
    );
    await expect(listVersions("form", "job-application")).resolves.toEqual([
      { version: 2, source: "cli", createdBy: "admin@demo.local", createdAt: "2026-09-12T10:00:00Z" },
      { version: 1, source: "cli", createdBy: "admin@demo.local", createdAt: "2026-09-11T10:00:00Z" },
    ]);
    await expect(loadVersion("form", "job-application", 1)).resolves.toMatchObject({ title: "Old" });
  });

  it("lists workflow slugs sorted", async () => {
    server.use(
      http.get("/api/v1/workflows", () =>
        HttpResponse.json({
          items: [
            { slug: "triage", title: "Triage", version: 1, source: "ui", updatedAt: "2026-09-01T00:00:00Z", stateCount: 3 },
            { slug: "hiring", title: "Hiring", version: 2, source: "cli", updatedAt: "2026-09-01T00:00:00Z", stateCount: 5 },
          ],
        }),
      ),
    );
    await expect(listWorkflowSlugs()).resolves.toEqual(["hiring", "triage"]);
  });
});
