import { useEffect, useMemo, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { Alert, App, Badge, Button, Flex, Form, Input, Layout as AntLayout, Menu, Modal } from "antd";
import { api } from "@/lib/api";
import { elapsed, jobLabel, type InstallQueue } from "@/lib/jobs";
import { canUsePath, spaClick } from "@/lib/nav";
import { Mark } from "@/components/Mark";
import { useBrand } from "@/components/ThemeProvider";

const { Sider, Content, Header } = AntLayout;

const items = [
  { key: "/", label: "Dashboard", blurb: "Host load, accounts, and the stack on this machine." },
  { key: "/accounts", label: "Accounts", blurb: "Linux users, home, and quota." },
  { key: "/sites", label: "Websites", blurb: "Domains, PHP, and Git deploy." },
  { key: "/php", label: "PHP", blurb: "FPM pool and php.ini for one account." },
  { key: "/files", label: "Files", blurb: "Fifty files a page, path jump, preview, rename." },
  { key: "/terminal", label: "Terminal", blurb: "Shell as the selected account." },
  { key: "/software", label: "Software", blurb: "Install and the live job log." },
  { key: "/security", label: "Security", blurb: "Firewall, WAF, and Fail2ban whitelist." },
  { key: "/logs", label: "Logs", blurb: "Clear one file or every log for a site." },
  { key: "/databases", label: "Databases", blurb: "MySQL users, import, and Redis." },
  { key: "/backup", label: "Backup", blurb: "Destinations and cron schedules." },
  { key: "/rclone", label: "rclone", blurb: "Remotes, copy, sync, and move." },
  { key: "/tools", label: "System tools", blurb: "DNS, time, disk, and network." },
  { key: "/os", label: "OS updates", blurb: "apt update and upgrade. The list is checked every day." },
  { key: "/settings", label: "Settings", blurb: "Theme, logo, and services." },
  { key: "/monitoring", label: "Monitoring", blurb: "Load history and alerts." },
  { key: "/apache", label: "Status", blurb: "Nginx, Apache, PHP-FPM, and the database." },
];

export function Layout({ user, admin, version }: { user: string; admin?: boolean; version?: string }) {
  const nav = useNavigate();
  const loc = useLocation();
  const { message } = App.useApp();
  const { brand } = useBrand();
  const [queue, setQueue] = useState<InstallQueue | null>(null);
  const [tick, setTick] = useState(0);
  const [collapsed, setCollapsed] = useState(false);
  const [passOpen, setPassOpen] = useState(false);
  const [passBusy, setPassBusy] = useState(false);
  const [passForm] = Form.useForm();

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

  async function changePassword(values: { current: string; next: string }) {
    setPassBusy(true);
    try {
      await api.post("/api/me/password", values);
      message.success("Password updated");
      setPassOpen(false);
      passForm.resetFields();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setPassBusy(false);
    }
  }

  const current = queue?.current;
  const waiting = queue?.queue?.length || 0;
  const installing = !!current || waiting > 0;
  const selected = useMemo(() => {
    const status = ["/apache", "/nginx", "/php-fpm", "/mysql", "/mariadb", "/redis"];
    if (status.includes(loc.pathname)) return ["/apache"];
    if (loc.pathname === "/scan-logs" || loc.pathname === "/logs") return ["/logs"];
    if (loc.pathname.startsWith("/sites")) return ["/sites"];
    return [loc.pathname === "/" ? "/" : loc.pathname];
  }, [loc.pathname]);

  const page = items.find((i) => selected[0] === i.key) || { label: loc.pathname.slice(1) || "Dashboard", blurb: "" };
  const menuItems = items
    .filter((i) => canUsePath(admin, i.key))
    .map((i) => {
      const n = String(items.indexOf(i) + 1).padStart(2, "0");
      const text =
        i.key === "/software" && installing ? (
          <Badge count={queue?.active || 0} size="small" offset={[8, 0]}>
            {i.label}
          </Badge>
        ) : (
          i.label
        );
      return {
        key: i.key,
        icon: <span className="siroc-nav-no">{n}</span>,
        label: (
          <Link to={i.key} className="cp-nav-link">
            {text}
          </Link>
        ),
      };
    });

  return (
    <AntLayout className="siroc-shell" style={{ minHeight: "100vh" }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        breakpoint="lg"
        collapsedWidth={72}
        width={232}
        theme="dark"
      >
        <div className={collapsed ? "siroc-brand is-collapsed" : "siroc-brand"}>
          {brand.hasLogo && brand.logoUrl ? (
            <img src={brand.logoUrl} alt="" className="brand-logo" />
          ) : (
            <Mark size={collapsed ? 28 : 36} />
          )}
          {collapsed ? null : (
            <div>
              <small>Control panel</small>
              <strong>Siroc</strong>
              {version ? <em>v{version}</em> : null}
            </div>
          )}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selected}
          items={menuItems}
          onClick={({ key, domEvent }) => {
            spaClick(String(key), nav)(domEvent);
          }}
        />
      </Sider>
      <AntLayout>
        <Header className="cp-header" style={{ padding: "0 28px", height: 58, lineHeight: "normal" }}>
          <Flex align="center" justify="space-between" style={{ height: "100%" }}>
            <div className="siroc-head-copy">
              <h1>{page.label}</h1>
              <p>{page.blurb}</p>
            </div>
            <Flex align="center" gap={8} className="siroc-who">
              <span>{user}</span>
              <Button className="siroc-ghost" onClick={() => setPassOpen(true)}>
                Password
              </Button>
              <Button className="siroc-ghost" onClick={logout}>
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
            description={
              <Link to="/software">{waiting ? `${waiting} waiting · view queue` : "in progress · view queue"}</Link>
            }
            style={{ cursor: "pointer" }}
            action={<span style={{ display: "none" }}>{tick}</span>}
          />
        ) : null}
        <Content className="siroc-stage" key={loc.pathname} style={{ padding: "26px 28px 48px" }}>
          <Outlet context={{ user, admin }} />
        </Content>
      </AntLayout>
      <Modal
        title="Change password"
        open={passOpen}
        onCancel={() => {
          setPassOpen(false);
          passForm.resetFields();
        }}
        onOk={() => passForm.submit()}
        confirmLoading={passBusy}
        destroyOnHidden
        okText="Update password"
      >
        <Form form={passForm} layout="vertical" onFinish={changePassword} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item name="current" label="Current password" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item name="next" label="New password" rules={[{ required: true, min: 8 }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </AntLayout>
  );
}
