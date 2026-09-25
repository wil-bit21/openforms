import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StateBadge, badgeColor } from "./StateBadge";

describe("StateBadge", () => {
  it("uses the workflow color", () => {
    render(<StateBadge label="Screening" color="blue" />);
    const badge = screen.getByText("Screening");
    expect(badge).toHaveClass("badge", "badge-blue");
    expect(badge).toHaveAttribute("data-color", "blue");
  });
  it("falls back to gray for unknown or missing colors", () => {
    expect(badgeColor("magenta")).toBe("gray");
    expect(badgeColor(undefined)).toBe("gray");
    expect(badgeColor(null)).toBe("gray");
    render(<StateBadge label="Submitted" />);
    expect(screen.getByText("Submitted")).toHaveAttribute("data-color", "gray");
  });
});
