import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { renderApp } from "../test/render";

describe("password reset", () => {
  it("links from the login page to the reset request form", async () => {
    server.use(http.get(api("/auth/me"), () => apiError(401, "unauthenticated", "Sign in required")));
    const { user, router } = renderApp("/login");
    await user.click(await screen.findByRole("link", { name: "Forgot your password?" }));
    expect(router.state.location.pathname).toBe("/forgot-password");
    expect(await screen.findByRole("heading", { name: "Reset your password" })).toBeInTheDocument();
  });

  it("requests a reset link and confirms without revealing whether the email exists", async () => {
    const requests: unknown[] = [];
    server.use(
      http.post(api("/auth/password-reset"), async ({ request }) => {
        requests.push(await request.json());
        return new HttpResponse(null, { status: 202 });
      }),
    );
    const { user } = renderApp("/forgot-password");
    await user.type(await screen.findByLabelText("Email"), " ada@example.com ");
    await user.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByRole("status")).toHaveTextContent("If an account exists for ada@example.com");
    expect(requests).toEqual([{ email: "ada@example.com" }]);
  });

  it("shows the rate-limit message", async () => {
    server.use(http.post(api("/auth/password-reset"), () => apiError(429, "rate_limited", "too many reset requests; try again later")));
    const { user } = renderApp("/forgot-password");
    await user.type(await screen.findByLabelText("Email"), "ada@example.com");
    await user.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("too many reset requests");
  });

  it("sets a new password with the token from the link", async () => {
    const confirms: unknown[] = [];
    server.use(
      http.post(api("/auth/password-reset/confirm"), async ({ request }) => {
        confirms.push(await request.json());
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = renderApp("/reset-password?token=tok123");
    await user.type(await screen.findByLabelText("New password"), "new-password-1");
    await user.type(screen.getByLabelText("Confirm new password"), "new-password-2");
    await user.click(screen.getByRole("button", { name: "Set new password" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The passwords do not match.");
    expect(confirms).toEqual([]);

    await user.clear(screen.getByLabelText("Confirm new password"));
    await user.type(screen.getByLabelText("Confirm new password"), "new-password-1");
    await user.click(screen.getByRole("button", { name: "Set new password" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Your password was changed");
    expect(confirms).toEqual([{ token: "tok123", password: "new-password-1" }]);
    expect(screen.getByRole("link", { name: "Sign in" })).toBeInTheDocument();
  });

  it("explains an expired link and offers a new one", async () => {
    server.use(http.post(api("/auth/password-reset/confirm"), () => apiError(400, "invalid_token", "this password reset link is invalid or has expired")));
    const { user } = renderApp("/reset-password?token=old");
    await user.type(await screen.findByLabelText("New password"), "new-password-1");
    await user.type(screen.getByLabelText("Confirm new password"), "new-password-1");
    await user.click(screen.getByRole("button", { name: "Set new password" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This reset link is invalid or has expired.");
    expect(screen.getByRole("link", { name: "Request a new link" })).toBeInTheDocument();
  });

  it("treats a link without a token as invalid", async () => {
    renderApp("/reset-password");
    expect(await screen.findByRole("alert")).toHaveTextContent("This reset link is invalid or has expired.");
  });

  it("explains login throttling", async () => {
    server.use(
      http.get(api("/auth/me"), () => apiError(401, "unauthenticated", "Sign in required")),
      http.post(api("/auth/login"), () => apiError(429, "rate_limited", "too many sign-in attempts; try again later")),
    );
    const { user } = renderApp("/login");
    await user.type(await screen.findByLabelText("Email"), "ada@example.com");
    await user.type(screen.getByLabelText("Password"), "whatever1");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Too many sign-in attempts.");
  });
});
