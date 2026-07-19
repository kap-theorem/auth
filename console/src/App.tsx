import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "./auth";
import Layout from "./components/Layout";
import Login from "./pages/Login";
import Apps from "./pages/Apps";
import AppDetail from "./pages/AppDetail";
import Directory from "./pages/admin/Directory";
import Developers from "./pages/admin/Developers";
import Metrics from "./pages/admin/Metrics";

export default function App() {
  const { session, isSuperadmin, ready } = useAuth();
  const location = useLocation();

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
