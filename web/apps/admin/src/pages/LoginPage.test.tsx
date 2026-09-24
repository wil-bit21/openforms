import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeDetail, makePrincipal, makeUser } from "../test/fixtures";
import { renderApp } from "../test/render";
import { safeNext } from "./LoginPage";

function signedOutUntilLogin(opts: { reject?: boolean } = {}) {
  let signedIn = false;
  const logins: unknown[] = [];
  server.use(
    http.get(api("/auth/me"), () =>
      signedIn ? HttpResponse.json({ principal: makePrincipal() }) : apiError(401, "unauthenticated", "Sign in required")),
    http.post(api("/auth/login"), async ({ request }) => {
      logins.push(await request.json());
      if (opts.reject) return apiError(401, "invalid_credentials", "invalid email or password");
      signedIn = true;
      return HttpResponse.json({ user: makeUser({ id: ids.admin, email: "admin@example.com", name: "Ada Admin", roles: ["admin"] }) });
    }),
  );
  return { logins };
}

describe("LoginPage", () => {
  it("redirects signed-out visitors to login and returns them to the deep link", async () => {
    const { logins } = signedOutUntilLogin();
    server.use(http.get(api("/submissions/:id"), () => HttpResponse.json(makeDetail())));
    const { user, router } = renderApp(`/submissions/${ids.sub1}`);

    await screen.findByRole("heading", { name: "Sign in to openforms" });
    expect(router.state.location.pathname).toBe("/login");
    expect(router.state.location.search).toBe(`?next=${encodeURIComponent(`/submissions/${ids.sub1}`)}`);

    await user.type(screen.getByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "correct-horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(router.state.location.pathname).toBe(`/submissions/${ids.sub1}`));
    expect(logins).toEqual([{ email: "admin@example.com", password: "correct-horse" }]);
  });

  it("shows a friendly message for wrong credentials", async () => {
    signedOutUntilLogin({ reject: true });
    const { user } = renderApp("/login");
    await user.type(await screen.findByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Email or password is incorrect.");
  });

  it("never redirects off-site after login", () => {
    expect(safeNext(null)).toBe("/submissions");
    expect(safeNext("")).toBe("/submissions");
    expect(safeNext("//evil.example/x")).toBe("/submissions");
    expect(safeNext("https://evil.example")).toBe("/submissions");
    expect(safeNext("/\\evil.example")).toBe("/submissions");
    expect(safeNext("/forms?x=1")).toBe("/forms?x=1");
  });
});
