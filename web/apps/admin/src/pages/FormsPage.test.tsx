import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { makeFormDefinition, makeFormRecord, makeFormSummary, makeReviewerPrincipal } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("FormsPage", () => {
  it("lists forms with workflow, visibility, version and submission counts", async () => {
    renderApp("/forms");
    const link = await screen.findByRole("link", { name: "Job application" });
    expect(link).toHaveAttribute("href", "/forms/job-application");
    const row = link.closest("tr")!;
    expect(within(row).getByRole("link", { name: "hiring" })).toHaveAttribute("href", "/workflows/hiring");
    expect(within(row).getByText("v3")).toBeInTheDocument();
    expect(within(row).getByText("12")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New form" })).toHaveAttribute("href", "/forms/new");
  });

  it("explains how to create the first form", async () => {
    server.use(http.get(api("/forms"), () => HttpResponse.json({ items: [] })));
    renderApp("/forms");
    expect(await screen.findByText(/No forms yet/)).toBeInTheDocument();
  });

  it("hides admin-only actions from reviewers", async () => {
    server.use(http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })));
    renderApp("/forms");
    await screen.findByRole("link", { name: "Job application" });
    expect(screen.queryByRole("link", { name: "New form" })).not.toBeInTheDocument();
  });
});

describe("FormOverviewPage", () => {
  it("shows metadata, links, fields and versions", async () => {
    renderApp("/forms/job-application");
    expect(await screen.findByRole("heading", { level: 1, name: "Job application" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View submissions" })).toHaveAttribute("href", "/submissions?form=job-application");
    expect(screen.getByRole("link", { name: "Open hosted form" })).toHaveAttribute("href", "/f/job-application");
    expect(screen.getByRole("link", { name: "Download CSV" }).getAttribute("href")).toContain("/api/v1/forms/job-application/submissions.csv");
    expect(screen.getByRole("link", { name: "Edit" })).toHaveAttribute("href", "/forms/job-application/edit");

    const fields = screen.getByRole("region", { name: "Fields" });
    expect(within(fields).getByText("Full name")).toBeInTheDocument();
    expect(within(fields).getByText("role = designer")).toBeInTheDocument();

    const versions = screen.getByRole("region", { name: "Versions" });
    expect(await within(versions).findByText("v3")).toBeInTheDocument();
    expect(within(versions).getByText("v2")).toBeInTheDocument();
    expect(within(versions).getByText("Editor")).toBeInTheDocument();
  });

  it("does not link to the hosted page for private forms", async () => {
    server.use(http.get(api("/forms/:slug"), () => HttpResponse.json({
      form: makeFormRecord({ definition: makeFormDefinition({ settings: { public: false } }) }),
    })));
    renderApp("/forms/job-application");
    await screen.findByRole("heading", { level: 1, name: "Job application" });
    expect(screen.queryByRole("link", { name: "Open hosted form" })).not.toBeInTheDocument();
  });

  it("shows not found for unknown forms", async () => {
    renderApp("/forms/missing");
    expect(await screen.findByRole("heading", { name: "Form not found" })).toBeInTheDocument();
  });

  it("keeps the summary list and overview in sync with the summary fixture", () => {
    expect(makeFormSummary().slug).toBe(makeFormRecord().slug);
  });
});
