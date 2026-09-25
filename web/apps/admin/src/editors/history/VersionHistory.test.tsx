import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { makePrincipal } from "../../test/fixtures";
import { OverviewExtras } from "../../extensions/OverviewExtras";
import { applyItem, formRecordJson, versionList } from "../test/records";
import { sampleForm } from "../test/samples";

afterEach(() => vi.restoreAllMocks());

const defV = (n: number) => ({ ...sampleForm(), title: `Job application v${n}` });

function mockVersions(versions: number[], roles: string[] = ["admin"]) {
  const puts: { url: string; body: { title: string } }[] = [];
  server.use(
    http.get("/api/v1/auth/me", () => HttpResponse.json({ principal: makePrincipal({ roles }) })),
    http.get("/api/v1/forms/job-application/versions", () => HttpResponse.json({ items: versionList(versions) })),
    http.get("/api/v1/forms/job-application/versions/:n", ({ params }) =>
      HttpResponse.json({ form: formRecordJson(defV(Number(params.n)), { version: Number(params.n) }) }),
    ),
    http.put("/api/v1/forms/job-application", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as { title: string } });
      return HttpResponse.json({ item: applyItem("form", "job-application", Math.max(...versions) + 1) });
    }),
  );
  return puts;
}

const renderHistory = () => renderWithProviders(<OverviewExtras kind="form" slug="job-application" />);

describe("VersionHistory via OverviewExtras", () => {
  it("compares the two latest versions by default", async () => {
    mockVersions([3, 2, 1]);
    renderHistory();
    const diff = await screen.findByRole("table", { name: "Differences between v2 and v3" });
    expect(within(diff).getByText("title: Job application v2")).toBeInTheDocument();
    expect(within(diff).getByText("title: Job application v3")).toBeInTheDocument();
  });

  it("switches the comparison when another version is ticked", async () => {
    mockVersions([3, 2, 1]);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("checkbox", { name: "Compare version 1" }));
    expect(await screen.findByRole("table", { name: "Differences between v1 and v2" })).toBeInTheDocument();
  });

  it("restores an older version as a new UI version", async () => {
    const puts = mockVersions([3, 2, 1]);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("button", { name: "Restore version 1" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0].body.title).toBe("Job application v1");
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(await screen.findByText("Restored as version 4.")).toBeInTheDocument();
  });

  it("does not restore when the confirmation is declined", async () => {
    const puts = mockVersions([2, 1]);
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("button", { name: "Restore version 1" }));
    expect(puts).toHaveLength(0);
  });

  it("hides restore for non-admins", async () => {
    mockVersions([3, 2, 1], ["reviewer"]);
    renderHistory();
    await screen.findByRole("checkbox", { name: "Compare version 3" });
    expect(screen.queryByRole("button", { name: /Restore version/ })).toBeNull();
  });

  it("explains when there is only one version", async () => {
    mockVersions([1]);
    renderHistory();
    expect(await screen.findByText(/Only one version so far/)).toBeInTheDocument();
  });
});
