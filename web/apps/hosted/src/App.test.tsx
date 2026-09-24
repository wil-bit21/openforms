import { OpenFormsClient, type FormDefinition } from "@openforms/sdk";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { App } from "./App.js";
import { parseRoute } from "./route.js";

const API = "http://api.test";
const contact: FormDefinition = {
  slug: "contact",
  title: "Contact us",
  description: "We usually reply within a day.",
  settings: { public: true, confirmationMessage: "Thanks, we got your message." },
  fields: [{ key: "email", type: "email", label: "Email", required: true }],
};

const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "contact"
      ? HttpResponse.json({ form: contact })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/contact/submissions`, () =>
    HttpResponse.json(
      { id: "sub-9", state: "new", stateLabel: "New", receiptToken: "r/t", confirmationMessage: "Thanks, we got your message." },
      { status: 201 },
    ),
  ),
  http.get(`${API}/api/v1/public/submissions/:id`, () =>
    HttpResponse.json({
      id: "sub-9",
      formTitle: "Contact us",
      state: "new",
      stateLabel: "New",
      terminal: false,
      states: [{ key: "new", label: "New" }],
      history: [{ state: "new", label: "New", at: "2026-09-23T10:00:00Z" }],
      createdAt: "2026-09-23T10:00:00Z",
    }),
  ),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  document.documentElement.classList.remove("of-embed");
});
afterAll(() => server.close());

const client = new OpenFormsClient({ baseUrl: API });

function renderAt(url: string, parent: { postMessage: ReturnType<typeof vi.fn> } | null = null) {
  const u = new URL(url, "http://hosted.test");
  return render(<App route={parseRoute(u.pathname, u.search)} client={client} parent={parent} />);
}

async function submitContact() {
  const user = userEvent.setup();
  await screen.findByRole("heading", { name: "Contact us" });
  await user.type(screen.getByLabelText(/^Email/), "ada@example.com");
  await user.click(screen.getByRole("button", { name: "Submit" }));
}

describe("hosted form page", () => {
  it("renders the form with page chrome and sets the document title", async () => {
    renderAt("/f/contact");
    expect(await screen.findByRole("heading", { name: "Contact us" })).toBeInTheDocument();
    expect(screen.getByText("We usually reply within a day.")).toBeInTheDocument();
    expect(screen.getByRole("contentinfo")).toHaveTextContent("Powered by openforms");
    await waitFor(() => expect(document.title).toBe("Contact us"));
  });

  it("shows the confirmation and a tracking link after submitting", async () => {
    renderAt("/f/contact");
    await submitContact();
    expect(await screen.findByText("Thanks, we got your message.")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Track your submission" });
    expect(link).toHaveAttribute("href", "/s/sub-9?token=r%2Ft");
    expect(link).not.toHaveAttribute("target");
  });

  it("shows a friendly page for unknown or private forms", async () => {
    renderAt("/f/secret");
    expect(await screen.findByRole("heading", { name: "This form isn't available" })).toBeInTheDocument();
  });

  it("shows not found for unknown paths", () => {
    renderAt("/nope");
    expect(screen.getByRole("heading", { name: "Page not found" })).toBeInTheDocument();
  });
});

describe("embed mode", () => {
  it("drops chrome, reports its height and announces submissions to the parent", async () => {
    const parent = { postMessage: vi.fn() };
    renderAt("/f/contact?embed=1", parent);
    await screen.findByRole("heading", { name: "Contact us" });
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
    expect(document.documentElement).toHaveClass("of-embed");
    expect(parent.postMessage).toHaveBeenCalledWith({ type: "openforms:resize", height: expect.any(Number) }, "*");

    await submitContact();
    await screen.findByText("Thanks, we got your message.");
    expect(parent.postMessage).toHaveBeenCalledWith({ type: "openforms:submitted", id: "sub-9", state: "new" }, "*");
    expect(screen.getByRole("link", { name: "Track your submission" })).toHaveAttribute("target", "_blank");
  });

  it("does not post messages when not framed", async () => {
    renderAt("/f/contact?embed=1", null);
    await screen.findByRole("heading", { name: "Contact us" });
    expect(document.documentElement).toHaveClass("of-embed");
  });
});

describe("status page", () => {
  it("renders the tracker for the submission", async () => {
    renderAt("/s/sub-9?token=r%2Ft");
    expect(await screen.findByText("New", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Contact us" })).toBeInTheDocument();
  });
});
