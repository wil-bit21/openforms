import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { isAdmin, useSession } from "../lib/session";

export function RequireAdmin({ children }: { children: ReactNode }) {
  const { data: me } = useSession();
  if (!isAdmin(me)) {
    return (
      <div className="page">
        <h1>Admins only</h1>
        <p className="muted">
          Your account doesn't have the <code>admin</code> role. <Link to="/submissions">Back to the inbox</Link>
        </p>
      </div>
    );
  }
  return <>{children}</>;
}
