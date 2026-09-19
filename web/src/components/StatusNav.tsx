import { useLocation, useNavigate } from "react-router-dom";
import { Segmented, Space, Switch, Tag, Typography } from "antd";
import type { ReactNode } from "react";

export const statusRoutes = [
  { path: "/apache", label: "Apache" },
  { path: "/nginx", label: "Nginx" },
  { path: "/php-fpm", label: "PHP-FPM" },
  { path: "/mysql", label: "MySQL" },
  { path: "/mariadb", label: "MariaDB" },
];

export function fmtNum(n: number, digits = 2) {
  if (!Number.isFinite(n)) return "0";
  if (Math.abs(n) >= 100) return n.toFixed(0);
  if (Math.abs(n) >= 10) return n.toFixed(1);
  return n.toFixed(digits);
}

export function fmtUptime(sec: number) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m ${s}s`;
  if (m) return `${m}m ${s}s`;
  return `${s}s`;
}

export function StatusNav() {
  const loc = useLocation();
  const nav = useNavigate();
  return (
    <div className="status-nav">
      <Segmented
        value={statusRoutes.some((r) => r.path === loc.pathname) ? loc.pathname : "/apache"}
        onChange={(v) => nav(String(v))}
        options={statusRoutes.map((r) => ({ label: r.label, value: r.path }))}
      />
    </div>
  );
}

export function StatusHero({
  tone,
  kicker,
  icon,
  title,
  subtitle,
  live,
  setLive,
  active,
  extra,
}: {
  tone: string;
  kicker: string;
  icon: ReactNode;
  title: string;
  subtitle: string;
  live: boolean;
  setLive: (v: boolean) => void;
  active: boolean;
  extra?: ReactNode;
}) {
  return (
    <div className={`apache-hero svc-hero svc-hero-${tone}`}>
      <div>
        <div className="apache-kicker">
          {icon} {kicker}
        </div>
        <Typography.Title level={3} style={{ margin: "4px 0 6px", color: "#fff" }}>
          {title}
        </Typography.Title>
        <Typography.Text style={{ color: "rgba(255,255,255,0.78)" }}>{subtitle}</Typography.Text>
      </div>
      <Space wrap align="center">
        <span className={live && active ? "apache-live" : "apache-live is-off"}>
          <i />
          {live && active ? "Live" : active ? "Paused" : "Offline"}
        </span>
        <Tag color={active ? "success" : "error"}>{active ? "Running" : "Stopped"}</Tag>
        {extra}
        <Switch checked={live} onChange={setLive} checkedChildren="Live" unCheckedChildren="Paused" />
      </Space>
    </div>
  );
}
