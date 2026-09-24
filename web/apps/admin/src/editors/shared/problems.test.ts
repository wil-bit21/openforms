import { describe, expect, it } from "vitest";
import { dedupeProblems, indexFromPath, mergeProblems, problemsAt, problemsUnder } from "./problems";

const problems = [
  { path: "fields[1]", message: "a" },
  { path: "fields[1].key", message: "b" },
  { path: "fields[10].key", message: "c" },
  { path: "fields[1].options[0].value", message: "d" },
  { path: "slug", message: "e" },
];

describe("problem helpers", () => {
  it("problemsAt matches exactly", () => {
    expect(problemsAt(problems, "fields[1].key")).toEqual([{ path: "fields[1].key", message: "b" }]);
  });

  it("problemsUnder matches the path and its descendants but not siblings with a longer index", () => {
    expect(problemsUnder(problems, "fields[1]").map((p) => p.message)).toEqual(["a", "b", "d"]);
  });

  it("indexFromPath extracts the collection index", () => {
    expect(indexFromPath("fields[10].key", "fields")).toBe(10);
    expect(indexFromPath("transitions[2].to", "fields")).toBeNull();
    expect(indexFromPath("slug", "fields")).toBeNull();
  });

  it("dedupe and merge remove identical problems", () => {
    expect(dedupeProblems([problems[4], problems[4]])).toEqual([problems[4]]);
    expect(mergeProblems([problems[0]], [problems[0], problems[4]])).toEqual([problems[0], problems[4]]);
  });
});
