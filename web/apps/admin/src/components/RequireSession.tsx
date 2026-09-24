import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { isUnauthenticated } from "../lib/queryClient";
import { useSession } from "../lib/session";
import { ErrorMessage } from "./ErrorMessage";
import { Loading } from "./Loading";

export function RequireSession({ children }: { children: ReactNode }) {
  const session = useSession();
  const location = useLocation();
  if (session.isError) {
    if (isUnauthenticated(session.error)) {
      const next = location.pathname + location.search;
      return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />;
    }
    return <main className="main"><ErrorMessage error={session.error} /></main>;
  }
  if (session.isPending) return <main className="main"><Loading /></main>;
  return <>{children}</>;
}
