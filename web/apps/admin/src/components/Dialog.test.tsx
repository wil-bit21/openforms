import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dialog } from "./Dialog";

describe("Dialog", () => {
  it("is a labelled modal dialog that focuses its first input", () => {
    render(<Dialog title="Add user" onClose={() => {}}><input aria-label="Name" /></Dialog>);
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByLabelText("Name")).toHaveFocus();
  });
  it("closes on Escape and on backdrop click, but not on inner click", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Dialog title="Confirm" onClose={onClose}><p>Body text</p></Dialog>);
    await user.click(screen.getByText("Body text"));
    expect(onClose).not.toHaveBeenCalled();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.click(screen.getByTestId("dialog-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
