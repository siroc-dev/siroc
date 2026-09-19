import { useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { Alert, Badge, Button, Flex, Layout as AntLayout, Menu, Typography } from "antd";
import {
  AppstoreOutlined,
  CloudDownloadOutlined,
  CloudServerOutlined,
  CodeOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  FileSearchOutlined,
  FolderOutlined,
  GlobalOutlined,
  LaptopOutlined,
  LogoutOutlined,
  MonitorOutlined,
  SafetyOutlined,
  ToolOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { api } from "@/lib/api";
import { elapsed, jobLabel, type InstallQueue } from "@/lib/jobs";

const { Sider, Content, Header } = AntLayout;

const items = [
  { key: "/", label: "Dashboard", icon: <DashboardOutlined /> },
  { key: "/accounts", label: "Accounts", icon: <UserOutlined /> },
  { key: "/sites", label: "Websites", icon: <GlobalOutlined /> },
  { key: "/php", label: "PHP", icon: <CodeOutlined /> },
  { key: "/files", label: "Files", icon: <FolderOutlined /> },
  { key: "/terminal", label: "Terminal", icon: <LaptopOutlined /> },
  { key: "/software", label: "Software", icon: <AppstoreOutlined /> },
  { key: "/security", label: "Security", icon: <SafetyOutlined /> },
  { key: "/scan-logs", label: "Scan logs", icon: <FileSearchOutlined /> },
  { key: "/databases", label: "Databases", icon: <DatabaseOutlined /> },
  { key: "/backup", label: "Backup", icon: <CloudDownloadOutlined /> },
  { key: "/tools", label: "System tools", icon: <ToolOutlined /> },
  { key: "/monitoring", label: "Monitoring", icon: <MonitorOutlined /> },
  { key: "/apache", label: "Status", icon: <CloudServerOutlined /> },
];

export function Layout({ user, admin, version }: { user: string; admin?: boolean; version?: string }) {
  const nav = useNavigate();
  const loc = useLocation();
  const [queue, setQueue] = useState<InstallQueue | null>(null);
  const [tick, setTick] = useState(0);
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    if (!admin) return;
    let stop = false;
    async function poll() {
      try {
        const data = await api.get<InstallQueue>("/api/software/jobs");
        if (!stop) setQueue(data);
      } catch {
        if (!stop) setQueue(null);
      }
    }
    poll();
    const t = setInterval(poll, 2000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [admin]);

  useEffect(() => {
    if (!queue?.current) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [queue?.current?.id]);

  async function logout() {
    await api.post("/api/logout");
    nav("/login");
  }

  const current = queue?.current;
  const waiting = queue?.queue?.length || 0;
  const installing = !!current || waiting > 0;
  const selected = useMemo(() => {
    const status = ["/apache", "/nginx", "/php-fpm", "/mysql", "/mariadb"];
    if (status.includes(loc.pathname)) return ["/apache"];
    return [loc.pathname === "/" ? "/" : loc.pathname];
  }, [loc.pathname]);

  const menuItems = items
    .filter((i) => admin || (i.key !== "/monitoring" && i.key !== "/scan-logs" && i.key !== "/apache" && i.key !== "/tools"))
    .map((i) => ({
    ...i,
    label:
      i.key === "/software" && installing ? (
        <Badge count={queue?.active || 0} size="small" offset={[8, 0]}>
          {i.label}
        </Badge>
      ) : (
        i.label
      ),
  }));

  return (
    <AntLayout style={{ minHeight: "100vh" }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        breakpoint="lg"
        collapsedWidth={72}
        width={232}
        theme="dark"
      >
        <div
          style={{
            padding: collapsed ? "16px 8px" : "20px 20px 12px",
            color: "#e0e7ff",
            textAlign: collapsed ? "center" : "left",
            overflow: "hidden",
            whiteSpace: "nowrap",
          }}
        >
          {collapsed ? (
            <Typography.Title level={5} style={{ color: "#fff", margin: 0 }}>
              S
            </Typography.Title>
          ) : (
            <>
              <div style={{ fontSize: 11, letterSpacing: 1.4, textTransform: "uppercase", opacity: 0.75 }}>Control Panel</div>
              <Typography.Title level={4} style={{ color: "#fff", margin: "4px 0 0" }}>
                Siroc
              </Typography.Title>
              {version ? (
                <div style={{ fontSize: 11, opacity: 0.65, marginTop: 2 }}>v{version}</div>
              ) : null}
            </>
          )}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selected}
          items={menuItems}
          onClick={({ key }) => {
            if (String(key).startsWith("/")) nav(key);
          }}
        />
      </Sider>
      <AntLayout>
        <Header style={{ padding: "0 20px", borderBottom: "1px solid #f0f0f0" }}>
          <Flex align="center" justify="space-between" style={{ height: "100%" }}>
            <Typography.Text type="secondary">
              {({
                "/": "Dashboard",
                "/apache": "Apache status",
                "/nginx": "Nginx status",
                "/php-fpm": "PHP-FPM status",
                "/mysql": "MySQL status",
                "/mariadb": "MariaDB status",
                "/scan-logs": "Scan logs",
              } as Record<string, string>)[loc.pathname] || loc.pathname.slice(1)}
            </Typography.Text>
            <Flex align="center" gap={8}>
              <Typography.Text>{user}</Typography.Text>
              <Button type="text" icon={<LogoutOutlined />} onClick={logout}>
                Logout
              </Button>
            </Flex>
          </Flex>
        </Header>
        {installing ? (
          <Alert
            type="warning"
            showIcon
            banner
            message={
              current
                ? `Installing ${jobLabel(current)}${elapsed(current.startedAt) ? ` · ${elapsed(current.startedAt)}` : ""}`
                : "Install queue starting…"
            }
            description={waiting ? `${waiting} waiting · view queue` : "in progress · view queue"}
            onClick={() => nav("/software")}
            style={{ cursor: "pointer" }}
            action={<span style={{ display: "none" }}>{tick}</span>}
          />
        ) : null}
        <Content style={{ padding: 24 }}>
          <Outlet context={{ user, admin }} />
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
