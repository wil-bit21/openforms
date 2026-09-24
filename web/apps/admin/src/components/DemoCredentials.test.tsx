import { describe, expect, it, vi } from "vitest";
import { http, HttpResponse } from "msw";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "../test/server";
import { renderWithProviders } from "../test/render";
import { DemoCredentials, DEMO_PASSWORD } from "./DemoCredentials";

describe("DemoCredentials", () => {
  it("lists the demo accounts and picks one", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })));
    const onPick = vi.fn();
    renderWithProviders(<DemoCredentials onPick={onPick} />);

    expect(await screen.findByRole("heading", { name: "Demo accounts" })).toBeInTheDocument();
    expect(screen.getByText("admin@demo.local")).toBeInTheDocument();
    expect(screen.getByText("reviewer@demo.local")).toBeInTheDocument();
    expect(screen.getByText("manager@demo.local")).toBeInTheDocument();
    expect(screen.getByText(DEMO_PASSWORD)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Use Reviewer" }));
    expect(onPick).toHaveBeenCalledWith("reviewer@demo.local", "demo1234");
  });

  it("renders nothing when demo is false", async () => {
    let requested = false;
    server.use(
      http.get("*/api/v1/public/config", () => {
        requested = true;
        return HttpResponse.json({ demo: false });
      }),
    );
    const { container } = renderWithProviders(<DemoCredentials onPick={() => {}} />);
    await waitFor(() => expect(requested).toBe(true));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the config request fails", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.error()));
    const { container } = renderWithProviders(<DemoCredentials onPick={() => {}} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(container).toBeEmptyDOMElement();
  });
});
