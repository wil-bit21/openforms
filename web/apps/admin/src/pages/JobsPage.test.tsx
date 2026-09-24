import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { ids, makeJob } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("JobsPage", () => {
  it("shows failed jobs by default with error, attempts and a submission link, and retries them", async () => {
    const statuses: (string | null)[] = [];
    const retried: string[] = [];
    let failed = [makeJob()];
    server.use(
      http.get(api("/jobs"), ({ request }) => {
        const status = new URL(request.url).searchParams.get("status");
        statuses.push(status);
        return HttpResponse.json({ items: status === "failed" ? failed : [] });
      }),
      http.post(api("/jobs/:id/retry"), ({ params }) => {
        retried.push(String(params.id));
        failed = [];
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = renderApp("/jobs");
    const row = (await screen.findByText("action.webhook")).closest("tr")!;
    expect(within(row).getByText("8/8")).toBeInTheDocument();
    expect(within(row).getByText(/500 Internal Server Error/)).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: ids.sub1.slice(0, 8) })).toHaveAttribute("href", `/submissions/${ids.sub1}`);
    expect(statuses[0]).toBe("failed");

    await user.click(within(row).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(retried).toEqual(["7"]));
    expect(await screen.findByText("No failed jobs.")).toBeInTheDocument();
  });

  it("switches status tabs via the URL", async () => {
    const statuses: (string | null)[] = [];
    server.use(http.get(api("/jobs"), ({ request }) => {
      statuses.push(new URL(request.url).searchParams.get("status"));
      return HttpResponse.json({ items: [makeJob({ id: 8, status: "pending", attempts: 1, lastError: "" })] });
    }));
    const { user, router } = renderApp("/jobs");
    await screen.findByText("action.webhook");
    await user.click(screen.getByRole("button", { name: "Pending" }));
    await waitFor(() => expect(statuses.at(-1)).toBe("pending"));
    expect(router.state.location.search).toBe("?status=pending");
    expect(screen.getByRole("button", { name: "Pending" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });
});
