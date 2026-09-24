import { useQueryClient } from "@tanstack/react-query";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { client } from "../api";
import { navItems } from "../nav";
import { isAdmin, useSession } from "../lib/session";

export function Shell() {
  const { data: me } = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const items = navItems.filter((item) => !item.adminOnly || isAdmin(me));

  async function signOut() {
    try {
      await client.logout();
    } finally {
      navigate("/login", { replace: true });
      queryClient.clear();
    }
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">openforms</div>
        <nav aria-label="Main">
          <ul>
            {items.map((item) => (
              <li key={item.to}>
                <NavLink to={item.to} className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}>
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
        <div className="sidebar-footer">
          <div className="who">
            <strong>{me?.name}</strong>
            {me?.email && <span className="muted">{me.email}</span>}
          </div>
          <button type="button" className="btn" onClick={signOut}>Sign out</button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
