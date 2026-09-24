import { describe, expect, it } from "vitest";
import { http } from "msw";
import { client } from "../api";
import { qk } from "../queryKeys";
import { createQueryClient } from "../lib/queryClient";
import { errorCode, errorMessage, problemsByPath } from "../lib/errors";
import { server } from "./server";
import { api, apiError } from "./handlers";
import { makePrincipal } from "./fixtures";

describe("test harness", () => {
  it("serves fixture data through the real SDK singleton", async () => {
    await expect(client.me()).resolves.toEqual(makePrincipal());
  });

  it("invalidates the session when any query fails with 401", async () => {
    server.use(http.get(api("/forms"), () => apiError(401, "unauthenticated", "Sign in required")));
    const qc = createQueryClient({ test: true });
    qc.setQueryData(qk.me, makePrincipal());
    await qc.fetchQuery({ queryKey: qk.forms, queryFn: () => client.listForms() }).catch(() => undefined);
    expect(qc.getQueryState(qk.me)?.isInvalidated).toBe(true);
  });

  it("exposes error codes, messages and problem paths", async () => {
    server.use(http.get(api("/forms"), () =>
      apiError(422, "validation_failed", "Invalid", [{ path: "fields.score", message: "must be a number" }])));
    const err = await client.listForms().catch((e: unknown) => e);
    expect(errorCode(err)).toBe("validation_failed");
    expect(errorMessage(err)).toBe("Invalid");
    expect(problemsByPath(err)).toEqual({ "fields.score": "must be a number" });
    expect(errorMessage("weird")).toBe("Something went wrong.");
  });
});
