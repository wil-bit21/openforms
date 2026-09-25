import { describe, expect, it, vi } from "vitest";
import { http, HttpResponse } from "msw";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "@testing-library/react";
import { server } from "./test/server";

vi.mock("./highlight", () => ({ highlight: async (code: string) => `<pre class="shiki"><code>${code}</code></pre>` }));

import { App } from "./App";

describe("demo App", () => {
  it("renders every section", async () => {
    render(<App />);
    for (const name of ["Define", "Collect", "Track & review", "Embed", "Automate from the CLI"]) {
      expect(screen.getByRole("heading", { level: 2, name: new RegExp(name) })).toBeInTheDocument();
    }
    expect(screen.getByRole("tab", { name: "forms/job-application.yaml" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByLabelText(/Full name/)).toBeInTheDocument();
  });

  it("shows the seed hint when the demo form is missing", async () => {
    server.use(
      http.get("*/api/v1/public/forms/job-application", () =>
        HttpResponse.json({ error: { code: "not_found", message: "not found" } }, { status: 404 }),
      ),
    );
    render(<App />);
    const collect = screen.getByRole("region", { name: /Collect/ });
    expect(await within(collect).findByText(/to enable the live demo/)).toBeInTheDocument();
    expect(within(collect).getByText("openforms seed --demo")).toBeInTheDocument();
  });

  it("shows an offline notice when the server is unreachable", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.error()));
    render(<App />);
    expect(await screen.findByText("Server unreachable")).toBeInTheDocument();
  });

  it("submits the application and starts tracking it", async () => {
    render(<App />);
    await userEvent.type(await screen.findByLabelText(/Full name/), "Ada Lovelace");
    await userEvent.click(screen.getByRole("button", { name: "Send application" }));

    const track = screen.getByRole("region", { name: /Track & review/ });
    expect(await within(track).findByRole("link", { name: "Open in the admin inbox" })).toHaveAttribute(
      "href",
      "/admin/submissions/sub-1",
    );
    expect((await within(track).findAllByText("New")).length).toBeGreaterThan(0);
    expect(await within(track).findByRole("button", { name: "Start screening" })).toBeEnabled();
  });
});
