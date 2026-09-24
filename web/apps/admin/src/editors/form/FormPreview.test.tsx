import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FormPreview, PreviewBoundary } from "./FormPreview";
import { sampleForm } from "../test/samples";
import type { FormDef } from "../shared/types";

afterEach(() => vi.restoreAllMocks());

describe("FormPreview", () => {
  it("renders the definition with the real renderer", async () => {
    render(<FormPreview def={sampleForm()} />);
    expect(await screen.findByLabelText(/Full name/)).toBeInTheDocument();
  });

  it("does not take the editor down for semantically invalid definitions", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const broken: FormDef = {
      ...sampleForm(),
      fields: [
        { key: "a", type: "select", label: "No options" },
        { key: "b", type: "text", label: "Ghost condition", showIf: { field: "missing", equals: "x" } },
      ],
    };
    render(<FormPreview def={broken} />);
    expect(screen.getByRole("region", { name: "Preview" })).toBeInTheDocument();
  });
});

describe("PreviewBoundary", () => {
  it("shows a fallback when the preview throws", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    function Boom(): JSX.Element {
      throw new Error("kaboom");
    }
    render(
      <PreviewBoundary resetKey="a">
        <Boom />
      </PreviewBoundary>,
    );
    expect(screen.getByText(/Preview unavailable/)).toHaveTextContent("kaboom");
  });
});
