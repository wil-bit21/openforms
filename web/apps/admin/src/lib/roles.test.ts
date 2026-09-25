import { describe, expect, it } from "vitest";
import { formatRoles, parseRoles } from "./roles";

describe("roles", () => {
  it("parses comma-separated roles, trimming blanks and duplicates", () => {
    expect(parseRoles(" admin, reviewer ,,reviewer, hiring-manager ")).toEqual(["admin", "reviewer", "hiring-manager"]);
    expect(parseRoles("")).toEqual([]);
  });
  it("formats roles for an input", () => {
    expect(formatRoles(["admin", "reviewer"])).toBe("admin, reviewer");
  });
});
