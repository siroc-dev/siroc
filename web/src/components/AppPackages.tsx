import { useEffect, useMemo, useRef, useState } from "react";
import { AutoComplete, Button, Space, Table, Tag, Typography } from "antd";

export type PkgRow = {
  name: string;
  section?: string;
  constraint?: string;
  installed?: string;
  wanted?: string;
  latest?: string;
  update?: string;
};

export type PkgHit = { name: string; description?: string; version?: string };

type Props = {
  kind: "composer" | "npm";
  disabled?: boolean;
  busy?: boolean;
  packages: PkgRow[];
  scripts?: string[];
  onRun: (command: string) => Promise<void> | void;
  onSearch: (query: string) => Promise<PkgHit[]>;
  onUpdate?: (name: string) => void;
};

const COMPOSER_BASE = [
  { value: "install", label: "install" },
  { value: "update", label: "update" },
  { value: "dump-autoload", label: "dump-autoload" },
  { value: "require ", label: "require …" },
  { value: "remove ", label: "remove …" },
];

function npmBase(scripts?: string[]) {
  const out = [
    { value: "install", label: "install" },
    { value: "update", label: "update" },
    { value: "uninstall ", label: "uninstall …" },
  ];
  for (const s of scripts || []) {
    out.push({ value: `run ${s}`, label: `run ${s}` });
  }
  return out;
}

function searchNeedle(kind: "composer" | "npm", raw: string) {
  const s = raw.trim();
  const lower = s.toLowerCase();
  if (kind === "composer") {
    const m = /^(?:require|remove|update|rm)\s+(\S+)/i.exec(s);
    if (m) return m[1];
    if (s.includes("/") && !lower.startsWith("install") && !lower.startsWith("dump")) return s;
    return "";
  }
  const m = /^(?:install|i|add|uninstall|remove|rm|update)\s+(\S+)/i.exec(s);
  if (m) return m[1].replace(/^@/, "@");
  return "";
}

function updateTag(row: PkgRow) {
  if (!row.update) return <Tag>current</Tag>;
  const label = row.update === "major" ? "major" : row.update === "minor" ? "minor" : "update";
  return <Tag color={row.update === "major" ? "warning" : "gold"}>{label}</Tag>;
}

export function AppPackages({ kind, disabled, busy, packages, scripts, onRun, onSearch, onUpdate }: Props) {
  const [cmd, setCmd] = useState("");
  const [hits, setHits] = useState<PkgHit[]>([]);
  const timer = useRef<number>(0);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const prefixOpts = useMemo(() => (kind === "composer" ? COMPOSER_BASE : npmBase(scripts)), [kind, scripts]);

  const options = useMemo(() => {
    const q = searchNeedle(kind, cmd);
    if (q && hits.length) {
      const before = cmd.replace(/\S+$/, "");
      return hits.map((h) => ({
        value: `${before}${h.name}`,
        label: `${h.name}${h.version ? " · " + h.version : ""}${h.description ? " — " + h.description : ""}`,
      }));
    }
    const needle = cmd.trim().toLowerCase();
    return prefixOpts.filter((o) => !needle || o.value.toLowerCase().includes(needle) || o.label.toLowerCase().includes(needle));
  }, [kind, cmd, hits, prefixOpts]);

  function onChange(v: string) {
    const next = typeof v === "string" ? v : "";
    setCmd(next);
    const q = searchNeedle(kind, next);
    window.clearTimeout(timer.current);
    if (q.length < 2) {
      setHits([]);
      return;
    }
    timer.current = window.setTimeout(() => {
      void onSearch(q).then((list) => setHits(list || [])).catch(() => setHits([]));
    }, 280);
  }

  const updatable = packages.filter((p) => p.update).length;

  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      <Space.Compact style={{ width: "100%" }}>
        <AutoComplete
          value={cmd}
          options={options}
          onChange={onChange}
          disabled={disabled || busy}
          placeholder={kind === "composer" ? "install, require laravel/horizon, update vendor/pkg" : "install, install lodash, run build, update"}
          style={{ flex: 1 }}
          filterOption={false}
          defaultActiveFirstOption={false}
          allowClear
          onSelect={(v) => setCmd(typeof v === "string" ? v : "")}
        />
        <Button type="primary" loading={busy} disabled={disabled || !cmd.trim()} onClick={() => void onRun(cmd.trim())}>
          Run
        </Button>
      </Space.Compact>
      <Typography.Text type="secondary">
        {packages.length ? `${packages.length} packages` : "No lockfile yet"}
        {updatable ? ` · ${updatable} can be updated` : ""}
      </Typography.Text>
      <Table
        size="small"
        rowKey="name"
        pagination={packages.length > 12 ? { pageSize: 12 } : false}
        dataSource={packages}
        locale={{ emptyText: kind === "composer" ? "No composer.json packages" : "No package.json packages" }}
        columns={[
          { title: "Package", dataIndex: "name", ellipsis: true },
          { title: "Type", dataIndex: "section", width: 120, responsive: ["md"] },
          { title: "Constraint", dataIndex: "constraint", width: 110, ellipsis: true, responsive: ["lg"] },
          { title: "Installed", dataIndex: "installed", width: 110, render: (v: string) => v || "—" },
          {
            title: "Compare",
            width: 180,
            render: (_, r) =>
              r.latest && r.installed && r.latest !== r.installed ? (
                <span>
                  {r.installed} → <Typography.Text strong>{r.latest}</Typography.Text>
                </span>
              ) : (
                r.latest || r.installed || "—"
              ),
          },
          { title: "", width: 90, render: (_, r) => updateTag(r) },
          {
            title: "",
            width: 88,
            align: "right" as const,
            render: (_, r) =>
              r.update ? (
                <Button size="small" disabled={busy} onClick={() => onUpdate?.(r.name)}>
                  Update
                </Button>
              ) : null,
          },
        ]}
      />
    </Space>
  );
}
