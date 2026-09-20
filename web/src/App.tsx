import { useEffect, useState, type ReactNode } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Spin } from "antd";
import { api } from "@/lib/api";
import { Layout } from "@/components/Layout";
import { AuthForm } from "@/pages/AuthForm";
import { SetupLocked } from "@/pages/SetupLocked";
import { Dashboard } from "@/pages/Dashboard";
import { Accounts } from "@/pages/Accounts";
import { Files } from "@/pages/Files";
import { TerminalPage } from "@/pages/Terminal";
import { Software } from "@/pages/Software";
import { Sites } from "@/pages/Sites";
import { Backup } from "@/pages/Backup";
import { Tools } from "@/pages/Tools";
import { Databases } from "@/pages/Databases";
import { Security } from "@/pages/Security";
import { ScanLogs } from "@/pages/ScanLogs";
import { PHP } from "@/pages/PHP";
import { Monitoring } from "@/pages/Monitoring";
import { Apache } from "@/pages/Apache";
import { Nginx } from "@/pages/Nginx";
import { PHPFPM } from "@/pages/PHPFPM";
import { MariaDB, MySQL } from "@/pages/DatabaseStatus";
import { canUsePath } from "@/lib/nav";

function setupTokenFromURL() {
  const q = new URLSearchParams(window.location.search);
  return (q.get("token") || "").trim();
}

function AdminOnly({ admin, children }: { admin?: boolean; children: ReactNode }) {
  const loc = useLocation();
  if (!canUsePath(admin, loc.pathname)) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

export function App() {
  const [ready, setReady] = useState(false);
  const [needed, setNeeded] = useState(false);
  const [setupOK, setSetupOK] = useState(false);
  const [setupToken, setSetupToken] = useState("");
  const [setupStack, setSetupStack] = useState<{ name: string; title: string; version: string }[]>([]);
  const [user, setUser] = useState<string | null>(null);
  const [admin, setAdmin] = useState(false);
  const [version, setVersion] = useState("");

  async function boot() {
    const token = setupTokenFromURL();
    setSetupToken(token);
    const path = token ? `/api/setup/status?token=${encodeURIComponent(token)}` : "/api/setup/status";
    const s = await api.get<{ needed: boolean; authorized?: boolean; stack?: { name: string; title: string; version: string }[] }>(path);
    setNeeded(s.needed);
    setSetupOK(!!s.authorized);
    setSetupStack(s.stack || []);
    if (!s.needed) {
      try {
        const me = await api.get<{ username: string; admin?: boolean; version?: string }>("/api/me");
        setUser(me.username);
        setAdmin(!!me.admin);
        setVersion(me.version || "");
      } catch {
        setUser(null);
        setAdmin(false);
      }
    }
    setReady(true);
  }

  useEffect(() => {
    boot().catch(() => setReady(true));
  }, []);

  if (!ready) {
    return (
      <div style={{ display: "grid", placeItems: "center", minHeight: "100vh" }}>
        <Spin size="large" tip="Loading…">
          <div style={{ padding: 80 }} />
        </Spin>
      </div>
    );
  }
  if (needed) {
    if (!setupOK) {
      return <SetupLocked invalidToken={!!setupToken} />;
    }
    return (
      <AuthForm
        title="Initial setup"
        subtitle="Create the panel administrator and a Let's Encrypt account. This one-time link is token-protected."
        submitLabel="Create admin"
        endpoint="/api/setup"
        extra={{ token: setupToken }}
        setup
        stack={setupStack}
        onDone={() => {
          setNeeded(false);
          setSetupOK(false);
          const url = new URL(window.location.href);
          url.searchParams.delete("token");
          if (url.pathname === "/setup") {
            url.pathname = "/login";
          }
          window.history.replaceState({}, "", url.toString());
        }}
      />
    );
  }

  return (
    <Routes>
      <Route path="/setup" element={<Navigate to="/login" replace />} />
      <Route
        path="/login"
        element={
          user ? (
            <Navigate to="/" replace />
          ) : (
            <AuthForm
              title="Sign in"
              subtitle="Siroc control panel"
              submitLabel="Sign in"
              endpoint="/api/login"
              onDone={() => boot()}
            />
          )
        }
      />
      <Route element={user ? <Layout user={user} admin={admin} version={version} /> : <Navigate to="/login" replace />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/accounts" element={<AdminOnly admin={admin}><Accounts /></AdminOnly>} />
        <Route path="/files" element={<Files />} />
        <Route path="/terminal" element={<TerminalPage />} />
        <Route path="/software" element={<AdminOnly admin={admin}><Software /></AdminOnly>} />
        <Route path="/security" element={<AdminOnly admin={admin}><Security /></AdminOnly>} />
        <Route path="/scan-logs" element={<AdminOnly admin={admin}><ScanLogs /></AdminOnly>} />
        <Route path="/sites" element={<Sites />} />
        <Route path="/php" element={<PHP />} />
        <Route path="/databases" element={<Databases />} />
        <Route path="/backup" element={<Backup />} />
        <Route path="/tools" element={<AdminOnly admin={admin}><Tools /></AdminOnly>} />
        <Route path="/monitoring" element={<AdminOnly admin={admin}><Monitoring /></AdminOnly>} />
        <Route path="/apache" element={<AdminOnly admin={admin}><Apache /></AdminOnly>} />
        <Route path="/nginx" element={<AdminOnly admin={admin}><Nginx /></AdminOnly>} />
        <Route path="/php-fpm" element={<AdminOnly admin={admin}><PHPFPM /></AdminOnly>} />
        <Route path="/mysql" element={<AdminOnly admin={admin}><MySQL /></AdminOnly>} />
        <Route path="/mariadb" element={<AdminOnly admin={admin}><MariaDB /></AdminOnly>} />
      </Route>
    </Routes>
  );
}
