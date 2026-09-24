import { describe, expect, it } from "vitest";
import { http, HttpResponse } from "msw";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "../test/server";
import { renderWithProviders } from "../test/render";
import { LoginPage } from "./LoginPage";

describe("LoginPage in demo mode", () => {
  it("fills the form from a demo account", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })));
    renderWithProviders(<LoginPage />, { route: "/login", path: "/login" });

    await userEvent.click(await screen.findByRole("button", { name: "Use Hiring manager" }));

    expect(screen.getByLabelText(/email/i)).toHaveValue("manager@demo.local");
    expect(screen.getByLabelText(/password/i)).toHaveValue("demo1234");
  });

  it("does not show demo accounts outside demo mode", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: false })));
    renderWithProviders(<LoginPage />, { route: "/login", path: "/login" });
    await screen.findByLabelText(/email/i);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("heading", { name: "Demo accounts" })).not.toBeInTheDocument();
  });
});
