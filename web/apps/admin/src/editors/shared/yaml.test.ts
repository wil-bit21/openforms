import { describe, expect, it } from "vitest";
import { parseYaml, toYaml } from "./yaml";
import { sampleForm, sampleWorkflow } from "../test/samples";

describe("toYaml / parseYaml", () => {
  it("round-trips a form definition", () => {
    const form = sampleForm();
    expect(parseYaml(toYaml(form))).toEqual({ ok: true, value: form });
  });

  it("round-trips a workflow definition including template braces", () => {
    const wf = sampleWorkflow();
    const text = toYaml(wf);
    expect(text).toContain("{{submission.data.email}}");
    expect(parseYaml(text)).toEqual({ ok: true, value: wf });
  });

  it("keeps the definition's key order", () => {
    const text = toYaml(sampleForm());
    expect(text.indexOf("slug:")).toBeLessThan(text.indexOf("title:"));
    expect(text.indexOf("title:")).toBeLessThan(text.indexOf("fields:"));
  });

  it("omits undefined properties", () => {
    expect(toYaml({ slug: "a", description: undefined })).toBe("slug: a\n");
  });

  it("reports syntax errors without throwing", () => {
    const result = parseYaml("fields: [\n");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.length).toBeGreaterThan(0);
  });

  it("rejects documents that are not a top-level mapping", () => {
    const message = "The document must be a mapping (key: value pairs) at the top level.";
    expect(parseYaml("- a\n- b\n")).toEqual({ ok: false, error: message });
    expect(parseYaml("")).toEqual({ ok: false, error: message });
    expect(parseYaml("just text")).toEqual({ ok: false, error: message });
  });
});
