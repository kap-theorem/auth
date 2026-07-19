import { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useAuth } from "../auth";

export default function Layout({ children }: { children: ReactNode }) {
  const { session, isSuperadmin, signOut } = useAuth();

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="wordmark">
          kaplabs <span>/</span> iam
        </div>

        <nav aria-label="Console">
          <div className="nav-group-label">Your org</div>
          <NavLink to="/" end className={({ isActive }) => "nav-link" + (isActive ? " active" : "")}>
            Apps
          </NavLink>

          {isSuperadmin && (
            <>
              <div className="nav-group-label">Platform</div>
              <NavLink to="/admin/directory" className={({ isActive }) => "nav-link" + (isActive ? " active" : "")}>
                Orgs &amp; apps
              </NavLink>
              <NavLink to="/admin/developers" className={({ isActive }) => "nav-link" + (isActive ? " active" : "")}>
                Developers
              </NavLink>
              <NavLink to="/admin/metrics" className={({ isActive }) => "nav-link" + (isActive ? " active" : "")}>
                Metrics
              </NavLink>
            </>
          )}
        </nav>

        <div className="sidebar-footer">
          <div className="who">{session?.developer.email}</div>
          {isSuperadmin && <span className="role-tag">superadmin</span>}
          <div style={{ marginTop: 10 }}>
            <button className="btn secondary small" onClick={signOut}>
              Sign out
            </button>
          </div>
        </div>
      </aside>

      <main className="main">{children}</main>
    </div>
  );
}
