import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { FieldProps } from "./fields.js";
import { OpenForm } from "./OpenForm.js";
import { API, jobForm, newClient, submitResult } from "./test-fixtures.js";

const posts: unknown[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: jobForm })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/:slug/submissions`, async ({ request }) => {
    posts.push(await request.json());
    await delay(50);
    return HttpResponse.json(submitResult, { status: 201 });
  }),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  posts.length = 0;
});
afterAll(() => server.close());

const client = newClient();

async function renderJobForm(extra: Partial<React.ComponentProps<typeof OpenForm>> = {}) {
  const user = userEvent.setup();
  render(<OpenForm client={client} slug="job-application" {...extra} />);
  await screen.findByRole("heading", { name: "Job application" });
  return user;
}

async function fillValid(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Full name/), "Ada Lovelace");
  await user.type(screen.getByLabelText(/^Email/), "ada@example.com");
  await user.selectOptions(screen.getByLabelText(/^Role/), "engineer");
  await user.click(screen.getByLabelText(/I agree to the privacy policy/));
}

describe("<OpenForm>", () => {
  it("renders the header, labelled fields and help text", async () => {
    await renderJobForm();
    expect(screen.getByText("Apply to join the team.")).toBeInTheDocument();
    const email = screen.getByLabelText(/^Email/);
    expect(email).toHaveAttribute("type", "email");
    expect(email).toHaveAccessibleDescription("We only use this to reply to you.");
    expect(screen.getByRole("group", { name: /Skills/ })).toBeInTheDocument();
    expect(screen.getByLabelText(/Cover letter/)).toHaveAttribute("placeholder", "Tell us about yourself");
    expect(screen.getByRole("button", { name: "Send application" })).toBeInTheDocument();
  });

  it("shows conditional fields only when their condition holds", async () => {
    const user = await renderJobForm();
    expect(screen.queryByLabelText(/Portfolio URL/)).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText(/^Role/), "designer");
    expect(screen.getByLabelText(/Portfolio URL/)).toBeInTheDocument();
  });

  it("focuses an error summary and marks invalid fields on a failed submit", async () => {
    const user = await renderJobForm();
    await user.click(screen.getByRole("button", { name: "Send application" }));
    const summary = await screen.findByRole("alert");
    expect(summary).toHaveTextContent("Please fix 4 fields before submitting.");
    await waitFor(() => expect(summary).toHaveFocus());
    const name = screen.getByLabelText(/Full name/);
    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(name).toHaveAccessibleDescription("This field is required");
    expect(posts).toEqual([]);

    await user.click(screen.getByRole("link", { name: /Full name/ }));
    expect(name).toHaveFocus();
  });

  it("submits and shows the confirmation", async () => {
    const onSubmitted = vi.fn();
    const user = await renderJobForm({ onSubmitted });
    await fillValid(user);
    await user.type(screen.getByLabelText(/Years of experience/), "7");
    await user.click(screen.getByLabelText("TypeScript"));
    await user.click(screen.getByRole("button", { name: "Send application" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Thanks! We'll be in touch.");
    expect(onSubmitted).toHaveBeenCalledTimes(1);
    expect(onSubmitted).toHaveBeenCalledWith(submitResult);
    expect(posts).toEqual([
      {
        data: { name: "Ada Lovelace", email: "ada@example.com", role: "engineer", years: 7, skills: ["ts"], consent: true },
      },
    ]);
  });

  it("double-clicking submit sends one request", async () => {
    const user = await renderJobForm();
    await fillValid(user);
    await user.dblClick(screen.getByRole("button", { name: "Send application" }));
    await screen.findByRole("status");
    expect(posts).toHaveLength(1);
  });

  it("shows server-side field errors", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "submission is invalid",
              details: [{ path: "data.email", message: "That email domain is not accepted" }],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const user = await renderJobForm();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Send application" }));
    const email = screen.getByLabelText(/^Email/);
    await waitFor(() => expect(email).toHaveAttribute("aria-invalid", "true"));
    expect(email).toHaveAccessibleDescription("We only use this to reply to you. That email domain is not accepted");
    expect(screen.getByRole("button", { name: "Send application" })).toBeEnabled();
  });

  it("renders a load error for unavailable forms", async () => {
    render(<OpenForm client={client} slug="missing" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("This form isn't available right now.");
  });

  it("uses custom confirmation and load-error renderers", async () => {
    render(<OpenForm client={client} slug="missing" renderLoadError={() => <p>Custom unavailable</p>} />);
    expect(await screen.findByText("Custom unavailable")).toBeInTheDocument();
  });

  it("renders from a definition without the network and calls onSubmit", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <OpenForm
        definition={jobForm}
        onSubmit={onSubmit}
        hideHeader
        renderConfirmation={() => <p>Preview submitted</p>}
      />,
    );
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Send application" }));
    expect(await screen.findByText("Preview submitted")).toBeInTheDocument();
    expect(onSubmit).toHaveBeenCalledWith({ name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true });
  });

  it("lets callers override field components by type", async () => {
    function Stars({ id, value, onChange, field }: FieldProps) {
      return (
        <input id={id} aria-label={`${field.label} stars`} value={String(value ?? "")} onChange={(e) => onChange(e.target.value)} />
      );
    }
    render(<OpenForm definition={jobForm} components={{ text: Stars }} />);
    expect(screen.getByLabelText("Full name stars")).toBeInTheDocument();
  });
});
