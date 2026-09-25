import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("../highlight", () => ({
  highlight: vi.fn(async (code: string) => `<pre class="shiki"><code>HL:${code}</code></pre>`),
}));

import { CodeBlock } from "./CodeBlock";

describe("CodeBlock", () => {
  it("shows plain code immediately, then the highlighted version", async () => {
    render(<CodeBlock code="slug: demo" lang="yaml" label="forms/demo.yaml" />);
    expect(screen.getByText("slug: demo")).toBeInTheDocument();
    expect(screen.getByText("forms/demo.yaml")).toBeInTheDocument();
    expect(await screen.findByText("HL:slug: demo")).toBeInTheDocument();
  });
});
