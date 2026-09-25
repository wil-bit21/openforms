import { describe, expect, it } from "vitest";
import { formatValue, relativeTime, shortId, submissionSummary } from "./format";

const now = new Date("2026-09-23T12:00:00Z");

describe("relativeTime", () => {
  it("formats seconds, minutes, hours and days relative to now", () => {
    expect(relativeTime("2026-09-23T12:00:00Z", now)).toBe("now");
    expect(relativeTime("2026-09-23T11:59:00Z", now)).toBe("1 minute ago");
    expect(relativeTime("2026-09-23T10:00:00Z", now)).toBe("2 hours ago");
    expect(relativeTime("2026-09-22T12:00:00Z", now)).toBe("yesterday");
  });
  it("falls back to a date after 30 days", () => {
    expect(relativeTime("2026-01-05T12:00:00Z", now)).toBe("Jan 5, 2026");
  });
});

describe("formatValue", () => {
  const select = { key: "role", type: "select", options: [{ value: "engineer", label: "Engineer" }, { value: "designer", label: "Designer" }] };
  it("renders empty values as an em dash", () => {
    expect(formatValue(undefined)).toBe("—");
    expect(formatValue(null)).toBe("—");
    expect(formatValue("")).toBe("—");
    expect(formatValue([])).toBe("—");
  });
  it("renders booleans, option labels, arrays and numbers", () => {
    expect(formatValue(true)).toBe("Yes");
    expect(formatValue(false)).toBe("No");
    expect(formatValue("engineer", select)).toBe("Engineer");
    expect(formatValue(["engineer", "designer"], { ...select, type: "multiselect" })).toBe("Engineer, Designer");
    expect(formatValue(12)).toBe("12");
    expect(formatValue("unknown", select)).toBe("unknown");
  });
  it("never renders [object Object]", () => {
    expect(formatValue({ a: 1 })).toBe('{"a":1}');
  });
});

describe("submissionSummary", () => {
  const form = { fields: [
    { key: "role", type: "select" },
    { key: "name", type: "text" },
    { key: "email", type: "email" },
    { key: "bio", type: "text" },
  ] };
  it("joins the first two non-empty text/email values in field order", () => {
    expect(submissionSummary({ role: "engineer", name: "Grace", email: "g@example.com", bio: "x" }, form)).toBe("Grace · g@example.com");
    expect(submissionSummary({ name: "  ", email: "g@example.com", bio: "Pioneer" }, form)).toBe("g@example.com · Pioneer");
  });
  it("falls back to string values in data order when the form is unknown", () => {
    expect(submissionSummary({ n: 3, a: "Alpha", b: "Beta", c: "Gamma" })).toBe("Alpha · Beta");
  });
  it("returns an em dash when there is nothing to show", () => {
    expect(submissionSummary({}, form)).toBe("—");
  });
});

describe("shortId", () => {
  it("keeps the first 8 characters", () => {
    expect(shortId("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")).toBe("aaaaaaaa");
  });
});
