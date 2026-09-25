import type { ReactElement } from "react";
import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { createQueryClient } from "../lib/queryClient";
import { buildRoutes } from "../App";

/**
 * Renders `ui` at route pattern `path` (default "/") with the memory router
 * starting at `route` (default: `path`). Paths are relative to the /admin basename.
 */
export function renderWithProviders(ui: ReactElement, opts: { route?: string; path?: string } = {}) {
  const path = opts.path ?? "/";
  const route = opts.route ?? path;
  const queryClient = createQueryClient({ test: true });
  const router = createMemoryRouter([{ path, element: ui }, { path: "*", element: <p>Navigated away</p> }], { initialEntries: [route] });
  const user = userEvent.setup();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...utils, user, router, queryClient };
}

/** Renders the whole admin route table (login + authenticated shell) starting at `route`. */
export function renderApp(route: string) {
  const queryClient = createQueryClient({ test: true });
  const router = createMemoryRouter(buildRoutes(), { initialEntries: [route] });
  const user = userEvent.setup();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...utils, user, router, queryClient };
}
