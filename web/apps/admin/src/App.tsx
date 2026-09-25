import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createBrowserRouter, Navigate, RouterProvider, type RouteObject } from "react-router-dom";
import { RequireAdmin } from "./components/RequireAdmin";
import { RequireSession } from "./components/RequireSession";
import { Shell } from "./components/Shell";
import { createQueryClient } from "./lib/queryClient";
import { ForgotPasswordPage } from "./pages/ForgotPasswordPage";
import { LoginPage } from "./pages/LoginPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { ResetPasswordPage } from "./pages/ResetPasswordPage";
import { routes } from "./routes";

export function buildRoutes(): RouteObject[] {
  return [
    { path: "/login", element: <LoginPage /> },
    { path: "/forgot-password", element: <ForgotPasswordPage /> },
    { path: "/reset-password", element: <ResetPasswordPage /> },
    {
      path: "/",
      element: (
        <RequireSession>
          <Shell />
        </RequireSession>
      ),
      children: [
        { index: true, element: <Navigate to="/submissions" replace /> },
        ...routes.map((r) => ({
          path: r.path,
          element: r.adminOnly ? <RequireAdmin>{r.element}</RequireAdmin> : r.element,
        })),
        { path: "*", element: <NotFoundPage /> },
      ],
    },
  ];
}

/** Production builds are served at /admin/*; `vite dev` serves the app under its base /_app/admin/. */
export function routerBasename(): string {
  return import.meta.env.DEV ? "/_app/admin" : "/admin";
}

export function App() {
  const [queryClient] = useState(() => createQueryClient());
  const [router] = useState(() => createBrowserRouter(buildRoutes(), { basename: routerBasename() }));
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
