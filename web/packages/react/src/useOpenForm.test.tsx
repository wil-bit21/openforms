import { act, renderHook, waitFor } from "@testing-library/react";
import { OpenFormsError } from "@openforms/sdk";
import { delay, http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API, jobForm, newClient, submitResult } from "./test-fixtures.js";
import { useOpenForm } from "./useOpenForm.js";

const posts: unknown[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: jobForm })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/:slug/submissions`, async ({ request }) => {
    posts.push(await request.json());
    await delay(30);
    return HttpResponse.json(submitResult, { status: 201 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  posts.length = 0;
});
afterAll(() => server.close());

const client = newClient();

async function readyHook() {
  const hook = renderHook(() => useOpenForm({ client, slug: "job-application" }));
  await waitFor(() => expect(hook.result.current.status).toBe("ready"));
  return hook;
}

function fillValid(set: (k: string, v: unknown) => void) {
  set("name", "Ada Lovelace");
  set("email", "ada@example.com");
  set("role", "engineer");
  set("consent", true);
}

describe("useOpenForm (slug mode)", () => {
  it("loads the public form", async () => {
    const { result } = renderHook(() => useOpenForm({ client, slug: "job-application" }));
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.form?.title).toBe("Job application");
    expect(result.current.visible).toMatchObject({ name: true, portfolio: false });
  });

  it("reports a load error for unavailable forms", async () => {
    const { result } = renderHook(() => useOpenForm({ client, slug: "missing" }));
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.loadError).toBeInstanceOf(OpenFormsError);
    expect((result.current.loadError as OpenFormsError).code).toBe("not_found");
  });

  it("validates locally without sending a request", async () => {
    const { result } = await readyHook();
    let ok = true;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(false);
    expect(Object.keys(result.current.errors).sort()).toEqual(["consent", "email", "name", "role"]);
    expect(posts).toEqual([]);
  });

  it("clears a field's error when its value changes", async () => {
    const { result } = await readyHook();
    await act(async () => {
      await result.current.submit();
    });
    act(() => result.current.setValue("name", "Ada"));
    expect(result.current.errors.name).toBeUndefined();
    expect(result.current.errors.email).toBeDefined();
  });

  it("submits clean data and exposes the result", async () => {
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    let ok = false;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(true);
    expect(result.current.status).toBe("submitted");
    expect(result.current.result).toEqual(submitResult);
    expect(posts).toEqual([{ data: { name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true } }]);
  });

  it("drops values of fields that became hidden", async () => {
    const { result } = await readyHook();
    act(() => {
      fillValid(result.current.setValue);
      result.current.setValue("role", "designer");
      result.current.setValue("portfolio", "https://ada.design");
    });
    act(() => result.current.setValue("role", "engineer"));
    await act(async () => {
      await result.current.submit();
    });
    expect(posts).toHaveLength(1);
    expect((posts[0] as { data: Record<string, unknown> }).data).not.toHaveProperty("portfolio");
  });

  it("ignores a second submit while one is in flight", async () => {
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await Promise.all([result.current.submit(), result.current.submit()]);
    });
    expect(posts).toHaveLength(1);
  });

  it("maps server validation errors onto fields", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "submission is invalid",
              details: [{ path: "data.email", message: "That email domain is not accepted" }],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    let ok = true;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(false);
    expect(result.current.status).toBe("ready");
    expect(result.current.errors).toEqual({ email: "That email domain is not accepted" });
  });

  it("shows a friendly form-level error for rate limiting and other failures", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json({ error: { code: "rate_limited", message: "slow down" } }, { status: 429 }),
      ),
    );
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.errors._form).toMatch(/too many submissions/i);
  });
});

describe("useOpenForm (definition mode)", () => {
  it("is ready immediately and passes clean data to onSubmit", async () => {
    const onSubmit = vi.fn(async () => undefined);
    const { result } = renderHook(() => useOpenForm({ definition: jobForm, onSubmit }));
    expect(result.current.status).toBe("ready");
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await result.current.submit();
    });
    expect(onSubmit).toHaveBeenCalledWith({ name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true });
    expect(result.current.status).toBe("submitted");
    act(() => result.current.reset());
    expect(result.current.status).toBe("ready");
    expect(result.current.values).toEqual({});
  });
});
