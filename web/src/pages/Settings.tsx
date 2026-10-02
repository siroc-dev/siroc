import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import {
  DesktopOutlined,
  MoonOutlined,
  PlayCircleFilled,
  SearchOutlined,
  SunOutlined,
} from "@ant-design/icons";
import { App, Button, Card, ColorPicker, Form, Input, Select, Space, Switch, Table, Typography } from "antd";
import { api } from "@/lib/api";
import { useBrand } from "@/components/ThemeProvider";
import { THEME_COLORS, type Brand, type ThemeColor, type ThemeStyle } from "@/lib/theme";

type Sys = {
  time: string;
  timezone: string;
  ntp: boolean;
  timezones?: string[];
};

type Svc = {
  name: string;
  title: string;
  version: string;
  active: boolean;
  monitor: boolean;
  manage?: string;
};

function timezoneOptions(st: Sys | null) {
  const extra = ["UTC", "Etc/UTC", "Asia/Bangkok", "Asia/Jakarta", "Asia/Ho_Chi_Minh", "Asia/Singapore"];
  const list = [...(st?.timezones || []), st?.timezone || "", ...extra].filter(Boolean);
  return [...new Set(list)].map((z) => ({ value: z, label: z }));
}

function StyleCard({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button type="button" className={`settings-choice${active ? " is-active" : ""}`} onClick={onClick}>
      <span className="settings-choice-icon">{icon}</span>
      <span>{label}</span>
    </button>
  );
}

export function Settings() {
  const { message } = App.useApp();
  const nav = useNavigate();
  const { brand, setBrand } = useBrand();
  const [sys, setSys] = useState<Sys | null>(null);
  const [svc, setSvc] = useState<Svc[]>([]);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState("");
  const [tzForm] = Form.useForm();
  const logoInput = useRef<HTMLInputElement>(null);
  const favInput = useRef<HTMLInputElement>(null);

  async function loadSys() {
    const data = await api.get<Sys>("/api/sysops");
    setSys(data);
    tzForm.setFieldsValue({ timezone: data.timezone, ntp: data.ntp });
  }

  async function loadSvc() {
    setSvc(await api.get<Svc[]>("/api/settings/services"));
  }

  useEffect(() => {
    loadSys().catch((e) => message.error(e.message));
    loadSvc().catch((e) => message.error(e.message));
  }, []);

  async function saveTheme(next: Brand) {
    setBrand(next);
    setBusy("theme");
    try {
      const saved = await api.put<Brand>("/api/settings/panel", {
        themeStyle: next.themeStyle,
        themeColor: next.themeColor,
        themeCustom: next.themeCustom || "",
      });
      setBrand({ ...next, ...saved });
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function saveTimezone(values: { timezone: string; ntp: boolean }) {
    setBusy("tz");
    try {
      const data = await api.post<Sys>("/api/sysops", { action: "timezone", ...values });
      setSys(data);
      message.success("Timezone saved");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function upload(kind: "logo" | "favicon", file: File) {
    setBusy(kind);
    try {
      const saved = await api.upload<Brand>(`/api/settings/brand/${kind}`, file);
      setBrand({ ...brand, ...saved });
      message.success(kind === "logo" ? "Logo updated" : "Favicon updated");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function resetBrand(kind: "logo" | "favicon") {
    setBusy(kind);
    try {
      const saved = await api.delete<Brand>(`/api/settings/brand/${kind}`);
      setBrand({ ...brand, ...saved });
      message.success("Reset to default");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function toggleMonitor(row: Svc, monitor: boolean) {
    setSvc((cur) => cur.map((s) => (s.name === row.name ? { ...s, monitor } : s)));
    try {
      await api.put("/api/settings/services/monitor", { name: row.name, monitor });
    } catch (err) {
      setSvc((cur) => cur.map((s) => (s.name === row.name ? { ...s, monitor: row.monitor } : s)));
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function restart(row: Svc) {
    setBusy("svc:" + row.name);
    try {
      await api.post("/api/software/service", { name: row.name, action: "restart" });
      message.success(`Restarted ${row.title}`);
      await loadSvc();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return svc;
    return svc.filter((s) => `${s.title} ${s.name} ${s.version}`.toLowerCase().includes(needle));
  }, [svc, q]);

  return (
    <div className="cp-page">
      <Typography.Title level={3} style={{ margin: 0 }}>
        Settings
      </Typography.Title>

      <Card title="Server Timezone">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          Current time {sys?.time || "—"}. Changing timezone updates the server clock used by logs and cron.
        </Typography.Paragraph>
        <Form form={tzForm} layout="inline" onFinish={saveTimezone}>
          <Form.Item name="timezone" label="Timezone">
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="Search timezone"
              style={{ minWidth: 280 }}
              options={timezoneOptions(sys)}
            />
          </Form.Item>
          <Form.Item name="ntp" label="NTP" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={busy === "tz"}>
            Save
          </Button>
        </Form>
      </Card>

      <Card title="Theme Settings">
        <div className="settings-row">
          <div className="settings-label">
            <div className="settings-label-title">Theme Style</div>
          </div>
          <div>
            <Space size={12}>
              <StyleCard
                active={brand.themeStyle === "auto"}
                icon={<DesktopOutlined />}
                label="Auto"
                onClick={() => saveTheme({ ...brand, themeStyle: "auto" as ThemeStyle })}
              />
              <StyleCard
                active={brand.themeStyle === "light"}
                icon={<SunOutlined />}
                label="Light"
                onClick={() => saveTheme({ ...brand, themeStyle: "light" })}
              />
              <StyleCard
                active={brand.themeStyle === "dark"}
                icon={<MoonOutlined />}
                label="Dark"
                onClick={() => saveTheme({ ...brand, themeStyle: "dark" })}
              />
            </Space>
            <div className="settings-help">Select the interface theme style, affecting the overall light and dark style.</div>
          </div>
        </div>

        <div className="settings-row">
          <div className="settings-label">
            <div className="settings-label-title">Theme Color</div>
          </div>
          <div>
            <Space size={12} wrap>
              {THEME_COLORS.map((c) => {
                const active = brand.themeColor === c.id;
                const swatch = c.id === "custom" ? brand.themeCustom || c.primary : c.primary;
                return (
                  <button
                    key={c.id}
                    type="button"
                    className={`settings-choice${active ? " is-active" : ""}`}
                    onClick={() =>
                      saveTheme({
                        ...brand,
                        themeColor: c.id,
                        themeCustom: c.id === "custom" ? brand.themeCustom || "#4f46e5" : brand.themeCustom,
                      })
                    }
                  >
                    <span className="settings-swatch" style={{ background: swatch }} />
                    <span>{c.label}</span>
                  </button>
                );
              })}
            </Space>
            <div className="settings-help">Select a preset theme color scheme or custom color, affecting the overall interface tone.</div>
            <Space style={{ marginTop: 12 }} wrap>
              {brand.themeColor === "custom" ? (
                <ColorPicker
                  value={brand.themeCustom || "#4f46e5"}
                  disabled={busy === "theme"}
                  onChangeComplete={(color) => saveTheme({ ...brand, themeColor: "custom" as ThemeColor, themeCustom: color.toHexString() })}
                />
              ) : null}
              <Button
                onClick={() => saveTheme({ ...brand, themeColor: "default", themeCustom: "" })}
                loading={busy === "theme"}
              >
                Restore Default
              </Button>
            </Space>
          </div>
        </div>
      </Card>

      <Card title="Logo Settings">
        <div className="settings-row">
          <div className="settings-label">
            <div className="settings-label-title">Logo</div>
          </div>
          <div>
            <div className="settings-preview">{brand.hasLogo && brand.logoUrl ? <img src={brand.logoUrl} alt="Logo" /> : <span>S</span>}</div>
            <div className="settings-help">Will be displayed at the top of the left menu bar (Recommended image size: 32×32px, SVG format recommended).</div>
            <Space style={{ marginTop: 12 }}>
              <input
                ref={logoInput}
                type="file"
                accept=".png,.jpg,.jpeg,.svg,.webp,.gif,image/*"
                hidden
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) upload("logo", file);
                }}
              />
              <Button onClick={() => logoInput.current?.click()} loading={busy === "logo"}>
                Upload Logo
              </Button>
              <Button onClick={() => resetBrand("logo")} disabled={!brand.hasLogo} loading={busy === "logo"}>
                Reset Default
              </Button>
            </Space>
          </div>
        </div>

        <div className="settings-row" style={{ borderBottom: 0, paddingBottom: 0 }}>
          <div className="settings-label">
            <div className="settings-label-title">Favicon</div>
          </div>
          <div>
            <div className="settings-preview settings-preview-sm">
              {brand.hasFavicon && brand.faviconUrl ? <img src={brand.faviconUrl} alt="Favicon" /> : <span>aa</span>}
            </div>
            <div className="settings-help">Will be displayed in the browser tab (Recommended image size: 16×16px, ICO, PNG or SVG format recommended).</div>
            <Space style={{ marginTop: 12 }}>
              <input
                ref={favInput}
                type="file"
                accept=".ico,.png,.svg,.jpg,.jpeg,.webp,image/*"
                hidden
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) upload("favicon", file);
                }}
              />
              <Button onClick={() => favInput.current?.click()} loading={busy === "favicon"}>
                Upload Favicon
              </Button>
              <Button onClick={() => resetBrand("favicon")} disabled={!brand.hasFavicon} loading={busy === "favicon"}>
                Reset Default
              </Button>
            </Space>
          </div>
        </div>
      </Card>

      <Card
        title={
          <Space>
            <span>Service Status</span>
            <Button onClick={() => loadSvc().catch((e) => message.error(e.message))}>Refresh</Button>
          </Space>
        }
        extra={
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="Please enter the service name"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 240 }}
          />
        }
      >
        <Table
          size="small"
          rowKey="name"
          dataSource={rows}
          pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (n) => `Total ${n}` }}
          columns={[
            { title: "Service Name", dataIndex: "title" },
            {
              title: "Monitor",
              width: 110,
              render: (_, row) => <Switch checked={row.monitor} onChange={(v) => toggleMonitor(row, v)} />,
            },
            { title: "Version", dataIndex: "version", width: 180, render: (v: string) => v || "—" },
            {
              title: "Status",
              width: 140,
              render: (_, row) =>
                row.active ? (
                  <Typography.Text type="success">
                    <PlayCircleFilled /> Running
                  </Typography.Text>
                ) : (
                  <Typography.Text type="secondary">Stopped</Typography.Text>
                ),
            },
            {
              title: "Operate",
              width: 180,
              render: (_, row) => (
                <Space>
                  <Button type="link" size="small" loading={busy === "svc:" + row.name} onClick={() => restart(row)}>
                    Restart
                  </Button>
                  {row.manage ? (
                    <Button type="link" size="small" onClick={() => nav(row.manage!)}>
                      Manage
                    </Button>
                  ) : null}
                </Space>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
