import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "./auth";
import Layout from "./components/Layout";
import Login from "./pages/Login";
import HostedLogin from "./pages/HostedLogin";
import HostedProfile from "./pages/HostedProfile";
import Apps from "./pages/Apps";
import AppDetail from "./pages/AppDetail";
import Directory from "./pages/admin/Directory";
import Developers from "./pages/admin/Developers";
import Metrics from "./pages/admin/Metrics";

export default function App() {
  const { session, isSuperadmin, ready } = useAuth();
  const location = useLocation();

  // Public hosted-login page for consumer apps: no console session, minimal
  // chrome, iframe-friendly (spec "Hosted login"). Rendered before every
  // session gate so it works logged in, logged out, or embedded.
  if (/^\/client\/[^/]+\/user\/login\/?$/.test(location.pathname)) {
    return (
      <Routes>
        <Route path="/client/:clientId/user/login" element={<HostedLogin />} />
      </Routes>
    );
  }

  // Public hosted account page (spec "Hosted account page"): the end user
  // manages their own profile/sessions/password, authenticated purely by
  // their access token. Same pre-gate placement as the hosted login route.
  if (/^\/client\/[^/]+\/user\/profile\/?$/.test(location.pathname)) {
    return (
      <Routes>
        <Route path="/client/:clientId/user/profile" element={<HostedProfile />} />
      </Routes>
    );
  }

  if (!ready) return null;

  if (!session) {
    return (
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  if (location.pathname === "/login") return <Navigate to="/" replace />;

  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Apps />} />
        <Route path="/apps/:clientId" element={<AppDetail />} />
        {isSuperadmin && (
          <>
            <Route path="/admin/directory" element={<Directory />} />
            <Route path="/admin/developers" element={<Developers />} />
            <Route path="/admin/metrics" element={<Metrics />} />
          </>
        )}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  );
}
