import { describe, expect, it } from "vitest";
import { parseRoute, statusHref } from "./route.js";

describe("parseRoute", () => {
  it.each([
    ["/f/contact", "", { kind: "form", slug: "contact", embed: false }],
    ["/f/contact/", "?embed=1", { kind: "form", slug: "contact", embed: true }],
    ["/f/job%20application", "", { kind: "form", slug: "job application", embed: false }],
    ["/_app/hosted/f/contact", "", { kind: "form", slug: "contact", embed: false }],
    ["/s/sub-1", "?token=a%2Fb", { kind: "status", id: "sub-1", token: "a/b", embed: false }],
    ["/s/sub-1", "", { kind: "notFound", embed: false }],
    ["/f/", "", { kind: "notFound", embed: false }],
    ["/f/a/b", "", { kind: "notFound", embed: false }],
    ["/elsewhere", "?embed=1", { kind: "notFound", embed: true }],
    ["/f/%E0%A4%A", "", { kind: "form", slug: "%E0%A4%A", embed: false }],
  ])("%s%s", (pathname, search, expected) => {
    expect(parseRoute(pathname, search)).toEqual(expected);
  });
});

describe("statusHref", () => {
  it("encodes id and token", () => {
    expect(statusHref("sub 1", "a/b+c")).toBe("/s/sub%201?token=a%2Fb%2Bc");
  });
});
