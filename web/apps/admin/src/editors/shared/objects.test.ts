import { describe, expect, it } from "vitest";
import { applyPatch, clone, isObject, moveItem, nextNumbered, objects, str, unique, uniqueName } from "./objects";

describe("applyPatch", () => {
  it("sets values and deletes undefined and empty-string keys by default", () => {
    const out = applyPatch<{ a?: string; b?: string; c?: number; d?: boolean }>(
      { a: "x", b: "y", c: 1 },
      { a: "", b: undefined, c: 0, d: false },
    );
    expect(out).toEqual({ c: 0, d: false });
  });

  it("keeps empty strings when asked", () => {
    expect(applyPatch<{ a?: string }>({ a: "x" }, { a: "" }, { dropEmptyStrings: false })).toEqual({ a: "" });
  });

  it("never mutates the target", () => {
    const target = { a: "x" };
    applyPatch(target, { a: "y" });
    expect(target).toEqual({ a: "x" });
  });
});

describe("naming helpers", () => {
  it("nextNumbered returns the first free numbered name", () => {
    expect(nextNumbered("field", [])).toBe("field1");
    expect(nextNumbered("field", ["field1", "field2", "field4"])).toBe("field3");
  });

  it("uniqueName tries the bare name first", () => {
    expect(uniqueName("name_copy", ["name"])).toBe("name_copy");
    expect(uniqueName("name_copy", ["name_copy", "name_copy2"])).toBe("name_copy3");
  });
});

describe("collections", () => {
  it("moveItem returns a new array with the item moved", () => {
    const items = ["a", "b", "c"];
    expect(moveItem(items, 0, 2)).toEqual(["b", "c", "a"]);
    expect(items).toEqual(["a", "b", "c"]);
  });

  it("unique keeps first occurrences in order", () => {
    expect(unique(["b", "a", "b", "c", "a"])).toEqual(["b", "a", "c"]);
  });

  it("objects keeps only plain objects from arrays", () => {
    expect(objects([1, "x", null, { a: 1 }, [2]])).toEqual([{ a: 1 }]);
    expect(objects("nope")).toEqual([]);
  });

  it("str and isObject coerce safely", () => {
    expect(str(5)).toBe("");
    expect(str("a")).toBe("a");
    expect(isObject({})).toBe(true);
    expect(isObject([])).toBe(false);
    expect(isObject(null)).toBe(false);
  });

  it("clone is deep", () => {
    const a = { x: { y: 1 } };
    const b = clone(a);
    b.x.y = 2;
    expect(a.x.y).toBe(1);
  });
});
