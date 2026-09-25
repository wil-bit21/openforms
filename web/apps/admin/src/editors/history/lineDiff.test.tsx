import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { sideBySide } from "./lineDiff";
import { DiffView } from "./DiffView";

describe("sideBySide", () => {
  it("marks identical text as same with matching line numbers", () => {
    expect(sideBySide("a\nb\n", "a\nb\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "same", left: { n: 2, text: "b" }, right: { n: 2, text: "b" } },
    ]);
  });

  it("pairs a replaced line", () => {
    const rows = sideBySide("a\nb\nc\n", "a\nB\nc\n");
    expect(rows[1]).toEqual({ kind: "changed", left: { n: 2, text: "b" }, right: { n: 2, text: "B" } });
    expect(rows[2]).toEqual({ kind: "same", left: { n: 3, text: "c" }, right: { n: 3, text: "c" } });
  });

  it("shows added lines only on the right", () => {
    expect(sideBySide("a\n", "a\nb\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "added", left: null, right: { n: 2, text: "b" } },
    ]);
  });

  it("shows removed lines only on the left", () => {
    expect(sideBySide("a\nb\n", "a\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "removed", left: { n: 2, text: "b" }, right: null },
    ]);
  });

  it("pads uneven replacements", () => {
    expect(sideBySide("a\nx\n", "a\ny\nz\n").slice(1)).toEqual([
      { kind: "changed", left: { n: 2, text: "x" }, right: { n: 2, text: "y" } },
      { kind: "changed", left: null, right: { n: 3, text: "z" } },
    ]);
  });
});

describe("DiffView", () => {
  it("labels the table and notes identical content (e.g. a workflow re-pin)", () => {
    render(<DiffView left={"slug: a\n"} right={"slug: a\n"} leftTitle="v1" rightTitle="v2" />);
    expect(screen.getByRole("table", { name: "Differences between v1 and v2" })).toBeInTheDocument();
    expect(screen.getByText("These versions have identical content.")).toBeInTheDocument();
  });
});
