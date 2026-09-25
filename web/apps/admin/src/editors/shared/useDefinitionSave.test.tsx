import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { useDefinitionSave } from "./useDefinitionSave";
import { formRecordJson } from "../test/records";
import { sampleForm } from "../test/samples";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function countPuts(response: () => Response) {
  const calls = { count: 0 };
  server.use(
    http.put("/api/v1/forms/:slug", () => {
      calls.count++;
      return response();
    }),
  );
  return calls;
}

describe("useDefinitionSave", () => {
  it("saves and reports the new version", async () => {
    countPuts(() => HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 4, changed: true, created: false } }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, onSaved));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith({ version: 4, changed: true }));
    expect(result.current.serverProblems).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("exposes 422 details as server problems", async () => {
    const details = [{ path: "fields[0].label", message: "server says no" }];
    countPuts(() => HttpResponse.json({ error: { code: "validation_failed", message: "invalid", details } }, { status: 422 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.serverProblems).toEqual(details));
    act(() => result.current.clearServerProblems());
    expect(result.current.serverProblems).toEqual([]);
  });

  it("keeps a 422 without details visible as a whole-definition problem", async () => {
    countPuts(() => HttpResponse.json({ error: { code: "validation_failed", message: "slug must match path" } }, { status: 422 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.serverProblems).toHaveLength(1));
    expect(result.current.serverProblems[0].path).toBe("");
  });

  it("refuses to create a definition whose slug already exists", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson() })));
    const puts = countPuts(() => HttpResponse.json({ item: {} }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), true, onSaved));
    await waitFor(() => expect(result.current.serverProblems).toHaveLength(1));
    expect(result.current.serverProblems[0]).toEqual({
      path: "slug",
      message: 'A form with the slug "job-application" already exists. Choose another slug.',
    });
    expect(puts.count).toBe(0);
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("creates a new definition when the slug is free", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ error: { code: "not_found", message: "nope" } }, { status: 404 })));
    const puts = countPuts(() => HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 1, changed: true, created: true } }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), true, onSaved));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(puts.count).toBe(1);
  });

  it("reports other failures as an error message", async () => {
    countPuts(() => HttpResponse.json({ error: { code: "internal", message: "database unavailable" } }, { status: 500 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.serverProblems).toEqual([]);
  });
});
