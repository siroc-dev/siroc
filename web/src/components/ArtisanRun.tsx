import { useMemo, useState } from "react";
import { AutoComplete, Button, Space, Typography } from "antd";

export type ArtisanCmd = { name: string; description?: string };

const ARTISAN_COMMON: ArtisanCmd[] = [
  { name: "about", description: "Application info" },
  { name: "migrate --force", description: "Run migrations" },
  { name: "migrate:status", description: "Migration status" },
  { name: "migrate:rollback", description: "Rollback last batch" },
  { name: "db:seed", description: "Seed the database" },
  { name: "cache:clear", description: "Clear cache" },
  { name: "config:clear", description: "Clear config cache" },
  { name: "config:cache", description: "Cache configuration" },
  { name: "route:list", description: "List routes" },
  { name: "route:clear", description: "Clear route cache" },
  { name: "view:clear", description: "Clear compiled views" },
  { name: "optimize:clear", description: "Clear cached bootstrap files" },
  { name: "storage:link", description: "Link public/storage" },
  { name: "queue:restart", description: "Restart queue workers" },
];

const ARTISAN_QUICK = ["migrate --force", "migrate:status", "cache:clear", "optimize:clear", "route:list", "storage:link"];

export const WPCLI_COMMON: ArtisanCmd[] = [
  { name: "cache flush", description: "Flush object cache" },
  { name: "rewrite flush", description: "Flush rewrite rules" },
  { name: "cron event list", description: "List cron events" },
  { name: "plugin list", description: "List plugins" },
  { name: "theme list", description: "List themes" },
  { name: "user list", description: "List users" },
  { name: "option get blogname", description: "Get site title" },
  { name: "db prefix", description: "Show table prefix" },
  { name: "db export", description: "Export database" },
  { name: "core version", description: "Show WordPress version" },
  { name: "core verify-checksums", description: "Verify core files" },
  { name: "plugin update --all", description: "Update all plugins" },
];

const WPCLI_QUICK = ["cache flush", "rewrite flush", "plugin list", "theme list", "user list", "db prefix", "core version"];

type Props = {
  kind?: "artisan" | "wp";
  disabled?: boolean;
  busy?: boolean;
  commands?: ArtisanCmd[];
  onRun: (command: string) => void;
};

export function ArtisanRun({ kind = "artisan", disabled, busy, commands, onRun }: Props) {
  const [cmd, setCmd] = useState("");
  const isWP = kind === "wp";
  const common = isWP ? WPCLI_COMMON : ARTISAN_COMMON;
  const quick = isWP ? WPCLI_QUICK : ARTISAN_QUICK;

  const options = useMemo(() => {
    const seen = new Set<string>();
    const all: ArtisanCmd[] = [];
    const add = (c: ArtisanCmd) => {
      const name = c.name.trim();
      if (!name || seen.has(name)) return;
      seen.add(name);
      all.push({ name, description: c.description });
    };
    common.forEach(add);
    (commands || []).forEach(add);
    const needle = cmd.trim().toLowerCase();
    return all
      .filter((c) => !needle || c.name.toLowerCase().includes(needle) || (c.description || "").toLowerCase().includes(needle))
      .slice(0, 80)
      .map((c) => ({
        value: c.name,
        label: c.description ? `${c.name} — ${c.description}` : c.name,
      }));
  }, [cmd, commands, common]);

  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      <Typography.Text type="secondary">{isWP ? "wp" : "php artisan"}</Typography.Text>
      <Space wrap>
        {quick.map((q) => (
          <Button key={q} size="small" disabled={disabled || busy} onClick={() => void onRun(q)}>
            {q}
          </Button>
        ))}
      </Space>
      <Space.Compact style={{ width: "100%" }}>
        <AutoComplete
          value={cmd}
          options={options}
          onChange={(v) => setCmd(typeof v === "string" ? v : "")}
          onSelect={(v) => setCmd(typeof v === "string" ? v : "")}
          disabled={disabled || busy}
          placeholder={isWP ? "plugin list, cache flush, option get siteurl" : "migrate --force, make:model Post -m, route:list"}
          style={{ flex: 1 }}
          filterOption={false}
          defaultActiveFirstOption={false}
          allowClear
        />
        <Button type="primary" loading={busy} disabled={disabled || !cmd.trim()} onClick={() => void onRun(cmd.trim())}>
          Run
        </Button>
      </Space.Compact>
    </Space>
  );
}
