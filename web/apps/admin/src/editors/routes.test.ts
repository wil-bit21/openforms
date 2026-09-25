import { describe, expect, it, vi } from "vitest";
import { routes } from "../routes";

vi.mock("@xyflow/react", () => import("./test/xyflowMock"));

describe("editor routes", () => {
  it.each(["forms/new", "forms/:slug/edit", "workflows/new", "workflows/:slug/edit"])("registers %s as admin-only", (path) => {
    const route = routes.find((r) => r.path === path);
    expect(route).toBeDefined();
    expect(route?.adminOnly).toBe(true);
  });
});
