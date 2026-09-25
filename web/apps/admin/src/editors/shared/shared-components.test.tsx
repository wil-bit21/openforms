import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CodeManagedBanner } from "./CodeManagedBanner";
import { OptionsEditor } from "./OptionsEditor";
import { ProblemList } from "./ProblemList";
import { TagInput } from "./TagInput";
import { TextControl } from "./controls";
import { YamlPane } from "./YamlPane";
import { UNSAVED_MESSAGE, useUnsavedChangesGuard } from "./useUnsavedChangesGuard";
import type { Option } from "./types";

afterEach(() => vi.restoreAllMocks());

describe("TextControl", () => {
  it("links errors to the input for assistive tech", () => {
    render(<TextControl label="Key" value="1x" onChange={() => {}} problems={[{ path: "fields[0].key", message: "bad key" }]} />);
    const input = screen.getByLabelText("Key");
    expect(input).toHaveAttribute("aria-invalid", "true");
    const describedBy = input.getAttribute("aria-describedby")!;
    expect(document.getElementById(describedBy)).toHaveTextContent("bad key");
  });
});

describe("OptionsEditor", () => {
  function Harness() {
    const [options, setOptions] = useState<Option[]>([{ value: "option1", label: "Option 1" }]);
    return (
      <>
        <OptionsEditor options={options} basePath="fields[0].options" problems={[{ path: "fields[0].options[0].value", message: "dup" }]} onChange={setOptions} />
        <output data-testid="options">{JSON.stringify(options)}</output>
      </>
    );
  }

  it("adds, edits and removes options and shows per-option problems", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    expect(screen.getByText("dup")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add option" }));
    await user.clear(screen.getByLabelText("Option 2 label"));
    await user.type(screen.getByLabelText("Option 2 label"), "Second");
    await user.click(screen.getByRole("button", { name: "Remove option 1" }));
    expect(JSON.parse(screen.getByTestId("options").textContent!)).toEqual([{ value: "option2", label: "Second" }]);
  });
});

describe("TagInput", () => {
  function Harness() {
    const [values, setValues] = useState<string[]>(["reviewer"]);
    return (
      <>
        <TagInput label="Roles" values={values} onChange={setValues} />
        <output data-testid="values">{values.join("|")}</output>
      </>
    );
  }

  it("adds on Enter and comma, ignores duplicates, removes on click", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByLabelText("Roles");
    await user.type(input, "lead{Enter}");
    await user.type(input, "ops,reviewer,");
    expect(screen.getByTestId("values")).toHaveTextContent("reviewer|lead|ops");
    await user.click(screen.getByRole("button", { name: "Remove lead" }));
    expect(screen.getByTestId("values")).toHaveTextContent("reviewer|ops");
  });
});

describe("ProblemList", () => {
  it("renders nothing without problems and selectable items otherwise", async () => {
    const onSelect = vi.fn();
    const { rerender } = render(<ProblemList problems={[]} onSelect={onSelect} />);
    expect(screen.queryByRole("region", { name: "Problems" })).toBeNull();
    rerender(<ProblemList problems={[{ path: "fields[1].key", message: "bad" }, { path: "", message: "whole thing" }]} onSelect={onSelect} />);
    expect(screen.getByRole("heading", { name: "2 problems" })).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: /fields\[1\]\.key/ }));
    expect(onSelect).toHaveBeenCalledWith({ path: "fields[1].key", message: "bad" });
    expect(screen.getByText("(definition)")).toBeInTheDocument();
  });
});

describe("CodeManagedBanner", () => {
  it("shows only for CLI-sourced definitions with the spec wording", () => {
    const { rerender, container } = render(<CodeManagedBanner kind="form" source="ui" />);
    expect(container).toBeEmptyDOMElement();
    rerender(<CodeManagedBanner kind="form" source="cli" />);
    expect(screen.getByRole("note")).toHaveTextContent(
      "This form is managed in code. Changes made here will be overwritten by the next openforms push unless you run openforms pull.",
    );
    rerender(<CodeManagedBanner kind="workflow" source="cli" />);
    expect(screen.getByRole("note")).toHaveTextContent("This workflow is managed in code.");
  });
});

describe("YamlPane", () => {
  it("applies valid YAML and reports invalid YAML without applying it", () => {
    const onApply = vi.fn();
    render(<YamlPane value={{ slug: "a", title: "A" }} onApply={onApply} />);
    const textarea = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    expect(textarea.value).toBe("slug: a\ntitle: A\n");

    fireEvent.change(textarea, { target: { value: "slug: a\ntitle: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(onApply).not.toHaveBeenCalled();
    expect(textarea.value).toBe("slug: a\ntitle: [\n");

    fireEvent.change(textarea, { target: { value: "slug: a\ntitle: B\n" } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(onApply).toHaveBeenCalledWith({ slug: "a", title: "B" });
  });
});

describe("useUnsavedChangesGuard", () => {
  function Guarded({ active }: { active: boolean }) {
    useUnsavedChangesGuard(active);
    return <a href="/elsewhere">Leave</a>;
  }

  it("asks before following links while active and blocks when declined", () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<Guarded active />);
    const notCancelled = fireEvent.click(screen.getByText("Leave"));
    expect(confirm).toHaveBeenCalledWith(UNSAVED_MESSAGE);
    expect(notCancelled).toBe(false);
  });

  it("lets navigation through when confirmed", () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<Guarded active />);
    expect(fireEvent.click(screen.getByText("Leave"))).toBe(true);
  });

  it("does nothing while inactive", () => {
    const confirm = vi.spyOn(window, "confirm");
    render(<Guarded active={false} />);
    fireEvent.click(screen.getByText("Leave"));
    expect(confirm).not.toHaveBeenCalled();
  });

  it("cancels beforeunload while active", () => {
    render(<Guarded active />);
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });
});
