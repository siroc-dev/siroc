import type { ReactNode } from "react";
import { Input, Select, Space, Switch, Typography } from "antd";
import { formatBytes } from "@/lib/usage";
import { PathPicker } from "@/components/PathPicker";

export type CachePreset = "public" | "wordpress" | "app" | "long" | "custom";

export type NginxCacheValue = {
  on: boolean;
  preset: CachePreset;
  valid: string;
  path: string;
  size: string;
  idle: string;
  stale: boolean;
  refresh: boolean;
  honor: boolean;
  cookies: string;
  skip: string;
};

type Recipe = {
  label: string;
  blurb: string;
  valid: string;
  size: string;
  idle: string;
  stale: boolean;
  refresh: boolean;
  honor: boolean;
  cookies: string;
  skip: string;
};

const RECIPES: Record<Exclude<CachePreset, "custom">, Recipe> = {
  public: {
    label: "Public pages",
    blurb: "Successful pages are stored for one hour and shared with later visitors. A cookie from the application is ignored, so a public page can be stored. A request that sends Authorization is not stored.",
    valid: "1h",
    size: "1g",
    idle: "1d",
    stale: false,
    refresh: false,
    honor: false,
    cookies: "",
    skip: "",
  },
  wordpress: {
    label: "WordPress",
    blurb: "Anonymous pages are shared. A logged-in user, the cart, checkout, and wp-admin always reach WordPress. If WordPress is briefly down, visitors still get the last saved page while a new one is fetched.",
    valid: "1h",
    size: "1g",
    idle: "7d",
    stale: true,
    refresh: true,
    honor: false,
    cookies: "wordpress_logged_in wp-postpass_ comment_author_ woocommerce_items_in_cart",
    skip: "/wp-admin /wp-login.php /xmlrpc.php /wp-json /cart /checkout /my-account",
  },
  app: {
    label: "Respect the app",
    blurb: "Cache-Control and Expires from the application decide the lifetime. The time below is used only when the application does not say. A response that sets a cookie is not stored.",
    valid: "10m",
    size: "1g",
    idle: "1d",
    stale: true,
    refresh: false,
    honor: true,
    cookies: "",
    skip: "",
  },
  long: {
    label: "Long cache",
    blurb: "Successful pages stay for 7 days, up to 10 GB. Visitors keep the saved page while a new copy is fetched in the background.",
    valid: "7d",
    size: "10g",
    idle: "30d",
    stale: true,
    refresh: true,
    honor: false,
    cookies: "",
    skip: "",
  },
};

const CUSTOM: Recipe = {
  label: "Custom",
  blurb: "The values below are used as written. Changing a field on another preset switches here.",
  valid: "1h",
  size: "1g",
  idle: "1d",
  stale: false,
  refresh: false,
  honor: false,
  cookies: "",
  skip: "",
};

export const CACHE_TIMES = [
  { value: "10m", label: "10 minutes" },
  { value: "30m", label: "30 minutes" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "12h", label: "12 hours" },
  { value: "1d", label: "1 day" },
  { value: "7d", label: "7 days" },
];

const CACHE_IDLES = [
  { value: "1h", label: "1 hour" },
  { value: "1d", label: "1 day" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

const PRESET_OPTIONS: { value: CachePreset; label: string }[] = [
  ...(Object.keys(RECIPES) as Exclude<CachePreset, "custom">[]).map((value) => ({ value, label: RECIPES[value].label })),
  { value: "custom", label: CUSTOM.label },
];

export function cacheDir(domain: string) {
  const host = domain.trim().toLowerCase().replace(/\.$/, "");
  return host ? `/var/cache/nginx/siroc/${host}` : "/var/cache/nginx/siroc";
}

export function cacheFromSaved(
  cache: {
    on?: boolean;
    enabled?: boolean;
    valid?: string;
    path?: string;
    preset?: string;
    size?: string;
    idle?: string;
    stale?: boolean;
    refresh?: boolean;
    honor?: boolean;
    cookies?: string;
    skip?: string;
  } | undefined,
  domain: string,
): NginxCacheValue {
  const on = cache?.enabled === true || cache?.on === true;
  const value: NginxCacheValue = {
    on,
    preset: "custom",
    valid: cache?.valid || "1h",
    path: cache?.path || cacheDir(domain),
    size: cache?.size || "1g",
    idle: cache?.idle || "1d",
    stale: !!cache?.stale,
    refresh: !!cache?.refresh,
    honor: !!cache?.honor,
    cookies: cache?.cookies || "",
    skip: cache?.skip || "",
  };
  const named = cache?.preset as CachePreset | undefined;
  if (named && (named === "custom" || named in RECIPES)) {
    value.preset = named === "public" ? "public" : named;
  } else {
    value.preset = matchingPreset(value);
  }
  return value;
}

export function applyCachePreset(current: NginxCacheValue, preset: CachePreset): NginxCacheValue {
  if (preset === "custom") return { ...current, preset };
  const recipe = RECIPES[preset];
  return {
    ...current,
    preset,
    valid: recipe.valid,
    size: recipe.size,
    idle: recipe.idle,
    stale: recipe.stale,
    refresh: recipe.refresh,
    honor: recipe.honor,
    cookies: recipe.cookies,
    skip: recipe.skip,
  };
}

export function cacheSizeToken(raw: string): string | null {
  const s = raw.toLowerCase().replace(/\s+/g, "");
  if (s === "" || s === "1g" || s === "1gb") return "";
  const m = /^(\d+)(k|m|g)b?$/.exec(s);
  if (!m || m[1].length > 7) return null;
  const n = Number(m[1]);
  if (!Number.isSafeInteger(n) || n <= 0) return null;
  const mul = m[2] === "g" ? 1024 ** 3 : m[2] === "m" ? 1024 ** 2 : 1024;
  const bytes = n * mul;
  if (!Number.isSafeInteger(bytes) || bytes <= 0) return null;
  if (bytes === 1024 ** 3) return "";
  if (bytes % 1024 ** 3 === 0) return `${bytes / 1024 ** 3}g`;
  if (bytes % 1024 ** 2 === 0) return `${bytes / 1024 ** 2}m`;
  return `${bytes / 1024}k`;
}

export function cacheSizeBytes(raw: string): number {
  const token = cacheSizeToken(raw);
  if (token === null) return 0;
  if (token === "") return 1024 ** 3;
  const m = /^(\d+)(k|m|g)$/.exec(token);
  if (!m) return 0;
  const n = Number(m[1]);
  const mul = m[2] === "g" ? 1024 ** 3 : m[2] === "m" ? 1024 ** 2 : 1024;
  return n * mul;
}

export function cachePayload(c: NginxCacheValue) {
  const sizeToken = cacheSizeToken(c.size);
  return {
    enabled: c.on,
    on: c.on,
    preset: c.preset === "public" ? "" : c.preset,
    valid: c.valid === "1h" ? "" : c.valid,
    path: c.path.trim(),
    size: sizeToken === null ? c.size.trim() : sizeToken,
    idle: c.idle === "1d" ? "" : c.idle,
    stale: c.stale || undefined,
    refresh: c.refresh || undefined,
    honor: c.honor || undefined,
    cookies: c.cookies.trim(),
    skip: c.skip.trim(),
  };
}

function matchingPreset(value: NginxCacheValue): CachePreset {
  for (const name of Object.keys(RECIPES) as Exclude<CachePreset, "custom">[]) {
    const recipe = RECIPES[name];
    if (
      value.valid === recipe.valid &&
      cacheSizeToken(value.size) !== null &&
      cacheSizeBytes(value.size) === cacheSizeBytes(recipe.size) &&
      value.idle === recipe.idle &&
      value.stale === recipe.stale &&
      value.refresh === recipe.refresh &&
      value.honor === recipe.honor &&
      value.cookies.trim() === recipe.cookies &&
      value.skip.trim() === recipe.skip
    ) {
      return name;
    }
  }
  return "custom";
}

function edit(value: NginxCacheValue, patch: Partial<NginxCacheValue>): NginxCacheValue {
  const next = { ...value, ...patch, preset: "custom" as const };
  return { ...next, preset: matchingPreset(next) };
}

export function NginxCacheForm({
  value,
  onChange,
  maxBytes = 0,
}: {
  value: NginxCacheValue;
  onChange: (next: NginxCacheValue) => void;
  maxBytes?: number;
}) {
  const recipe = value.preset === "custom" ? CUSTOM : RECIPES[value.preset];
  const off = !value.on;
  const sizeBytes = cacheSizeBytes(value.size);
  const sizeInvalid = !off && value.size.trim() !== "" && cacheSizeToken(value.size) === null;
  const sizeOver = !off && maxBytes > 0 && sizeBytes > maxBytes;
  const cap =
    maxBytes > 0
      ? `At most ${formatBytes(maxBytes)}, 90% of the disk that holds this folder.`
      : "At most 90% of the disk that holds this folder.";
  const sizeWarn = sizeInvalid
    ? "Use a size such as 500m or 2g."
    : sizeOver
      ? `At most ${formatBytes(maxBytes)}, which is 90% of this disk.`
      : "";
  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      <div>
        <Typography.Text>Preset</Typography.Text>
        <Select
          style={{ width: "100%", marginTop: 4 }}
          value={value.preset}
          options={PRESET_OPTIONS}
          disabled={off}
          onChange={(preset) => onChange(applyCachePreset(value, preset))}
        />
        <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
          {recipe.blurb}
        </Typography.Paragraph>
      </div>
      <Space wrap size={16} align="start">
        <Field label="Keep a successful page" help="Pages with status 200, 301, and 302 are stored for this long. Error pages are not stored.">
          <Select style={{ width: 160 }} value={value.valid} options={CACHE_TIMES} disabled={off} onChange={(valid) => onChange(edit(value, { valid }))} />
        </Field>
        <Field
          label="Disk limit"
          help={`The oldest saved files are deleted after the folder reaches this size. Type a size such as 500m or 2g. ${cap}`}
          warn={sizeWarn}
        >
          <Input
            style={{ width: 160 }}
            value={value.size}
            disabled={off}
            placeholder="1g"
            onChange={(e) => onChange(edit(value, { size: e.target.value }))}
          />
        </Field>
        <Field label="Drop unused files" help="A saved file that nobody requests is deleted after this time, even when the folder is still small.">
          <Select style={{ width: 140 }} value={value.idle} options={CACHE_IDLES} disabled={off} onChange={(idle) => onChange(edit(value, { idle }))} />
        </Field>
      </Space>
      <Toggle
        checked={value.stale}
        disabled={off}
        label="Serve the saved page when the origin fails"
        help="Visitors get the last saved page if the origin times out or returns 502, 503, or 504."
        onChange={(stale) => onChange(edit(value, { stale }))}
      />
      <Toggle
        checked={value.refresh}
        disabled={off}
        label="Refresh in the background"
        help="One request fetches a new copy while everyone else keeps receiving the saved page."
        onChange={(refresh) => onChange(edit(value, { refresh }))}
      />
      <Toggle
        checked={value.honor}
        disabled={off}
        label="Let the application set the lifetime"
        help="Cache-Control and Expires from the application are kept. The time above is only the fallback. Turn this off to store every successful page for that time anyway."
        onChange={(honor) => onChange(edit(value, { honor }))}
      />
      <Field label="Never cache these paths" help="A request under one of these addresses is not stored. /wp-admin also covers /wp-admin/anything. Separate paths with spaces.">
        <Input
          value={value.skip}
          disabled={off}
          placeholder="/wp-admin /cart"
          onChange={(e) => onChange(edit(value, { skip: e.target.value }))}
        />
      </Field>
      <Field label="Never cache these cookies" help="If the request cookie contains one of these names, the page is not stored and is not served from cache. Separate names with spaces.">
        <Input
          value={value.cookies}
          disabled={off}
          placeholder="wordpress_logged_in"
          onChange={(e) => onChange(edit(value, { cookies: e.target.value }))}
        />
      </Field>
      <Field label="Cache directory" help="Filled in for this site. Clearing the cache deletes only the files in this folder. The response header X-Cache-Status says HIT, MISS, BYPASS, EXPIRED, STALE, or UPDATING.">
        <PathPicker user="root" dirsOnly value={value.path} disabled={off} onChange={(path) => onChange(edit(value, { path }))} />
      </Field>
    </Space>
  );
}

function Field({ label, help, warn, children }: { label: string; help: string; warn?: string; children: ReactNode }) {
  return (
    <div style={{ flex: "1 1 220px" }}>
      <Typography.Text>{label}</Typography.Text>
      <div style={{ marginTop: 4 }}>{children}</div>
      <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
        {help}
      </Typography.Paragraph>
      {warn ? (
        <Typography.Paragraph type="danger" style={{ marginTop: 4, marginBottom: 0 }}>
          {warn}
        </Typography.Paragraph>
      ) : null}
    </div>
  );
}

function Toggle({
  checked,
  disabled,
  label,
  help,
  onChange,
}: {
  checked: boolean;
  disabled: boolean;
  label: string;
  help: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div>
      <Space>
        <Switch checked={checked} disabled={disabled} onChange={onChange} />
        <span>{label}</span>
      </Space>
      <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
        {help}
      </Typography.Paragraph>
    </div>
  );
}
