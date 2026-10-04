import { useEffect, useState } from "react";
import { Button, Input, InputNumber, Select, Space, Switch, Table, Tabs, Typography } from "antd";

export type ProxyRewrite = { from: string; to: string };
export type ProxyReplace = { from: string; to: string; rule?: string };
export type ProxySettingsValue = {
  showPath?: boolean;
  path?: string;
  target?: string;
  host?: string;
  rewrites?: ProxyRewrite[];
  remark?: string;
  websocket?: boolean;
  connectTimeout?: number;
  sendTimeout?: number;
  readTimeout?: number;
  config?: string;
  replacements?: ProxyReplace[];
  cache?: boolean;
  gzip?: boolean;
  black?: string[];
  white?: string[];
};

const RULES = [
  { value: "g", label: "g · replace all" },
  { value: "i", label: "i · case insensitive" },
  { value: "o", label: "o · first match only" },
  { value: "r", label: "r · regular expression" },
];

function hostOf(target: string) {
  try {
    return new URL(target).hostname;
  } catch {
    return "";
  }
}

export function ProxySettings({
  busy,
  target,
  value,
  onSave,
}: {
  busy: boolean;
  target: string;
  value?: ProxySettingsValue;
  onSave: (proxy: ProxySettingsValue, target: string) => void;
}) {
  const [showPath, setShowPath] = useState(true);
  const [path, setPath] = useState("/");
  const [url, setUrl] = useState("");
  const [host, setHost] = useState("");
  const [rewrites, setRewrites] = useState<ProxyRewrite[]>([{ from: "", to: "" }]);
  const [remark, setRemark] = useState("");
  const [websocket, setWebsocket] = useState(true);
  const [connectTimeout, setConnectTimeout] = useState(60);
  const [sendTimeout, setSendTimeout] = useState(600);
  const [readTimeout, setReadTimeout] = useState(600);
  const [config, setConfig] = useState("");
  const [replacements, setReplacements] = useState<ProxyReplace[]>([]);
  const [repFrom, setRepFrom] = useState("");
  const [repTo, setRepTo] = useState("");
  const [repRule, setRepRule] = useState("g");
  const [cache, setCache] = useState(false);
  const [gzip, setGzip] = useState(false);
  const [black, setBlack] = useState("");
  const [white, setWhite] = useState("");

  useEffect(() => {
    const p = value || {};
    const nextURL = p.target || target || "";
    const saved = !!p.target;
    setShowPath(saved ? !!p.showPath : true);
    setPath(p.path || "/");
    setUrl(nextURL);
    setHost(p.host || hostOf(nextURL));
    setRewrites(p.rewrites && p.rewrites.length ? p.rewrites : [{ from: "", to: "" }]);
    setRemark(p.remark || "");
    setWebsocket(saved ? !!p.websocket : true);
    setConnectTimeout(p.connectTimeout || 60);
    setSendTimeout(p.sendTimeout || 600);
    setReadTimeout(p.readTimeout || 600);
    setConfig(p.config || "");
    setReplacements(p.replacements || []);
    setCache(!!p.cache);
    setGzip(!!p.gzip);
    setBlack((p.black || []).join("\n"));
    setWhite((p.white || []).join("\n"));
  }, [target, value]);

  function payload(): ProxySettingsValue {
    return {
      showPath,
      path: path || "/",
      target: url.trim(),
      host: host.trim(),
      rewrites: rewrites.filter((r) => r.from.trim() || r.to.trim()),
      remark: remark.trim(),
      websocket,
      connectTimeout,
      sendTimeout,
      readTimeout,
      config,
      replacements,
      cache,
      gzip,
      black: black.split(/\s+/).map((s) => s.trim()).filter(Boolean),
      white: white.split(/\s+/).map((s) => s.trim()).filter(Boolean),
    };
  }

  function save() {
    onSave(payload(), url.trim());
  }

  const saveBtn = (
    <Button type="primary" loading={busy} onClick={save} style={{ marginTop: 16 }}>
      Save
    </Button>
  );

  return (
    <Tabs
      items={[
        {
          key: "proxy",
          label: "Reverse proxy",
          children: (
            <Space direction="vertical" size={12} style={{ width: "100%" }}>
              <Space>
                <span>Show proxy path</span>
                <Switch checked={showPath} onChange={setShowPath} />
              </Space>
              <label>
                <div>Proxy path</div>
                <Input value={path} onChange={(e) => setPath(e.target.value)} placeholder="/" disabled={!showPath} />
              </label>
              <label>
                <div>Target URL</div>
                <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com" />
              </label>
              <label>
                <div>Send host</div>
                <Input value={host} onChange={(e) => setHost(e.target.value)} placeholder="example.com" />
              </label>
              <div>
                <div>URL rewrite</div>
                {rewrites.map((rw, i) => (
                  <Space key={i} style={{ marginTop: 8 }} wrap>
                    <Input placeholder="/aaa" value={rw.from} onChange={(e) => setRewrites((list) => list.map((row, n) => (n === i ? { ...row, from: e.target.value } : row)))} />
                    <Input placeholder="/bbb" value={rw.to} onChange={(e) => setRewrites((list) => list.map((row, n) => (n === i ? { ...row, to: e.target.value } : row)))} />
                    <Button onClick={() => setRewrites((list) => (list.length === 1 ? [{ from: "", to: "" }] : list.filter((_, n) => n !== i)))}>−</Button>
                    <Button onClick={() => setRewrites((list) => [...list, { from: "", to: "" }])}>+</Button>
                  </Space>
                ))}
              </div>
              <label>
                <div>Remark</div>
                <Input value={remark} onChange={(e) => setRemark(e.target.value)} />
              </label>
              <Space>
                <span>Websocket support</span>
                <Switch checked={websocket} onChange={setWebsocket} />
              </Space>
              <Space wrap>
                <span>Connection timeout</span>
                <InputNumber min={1} max={3600} value={connectTimeout} onChange={(v) => setConnectTimeout(v || 60)} addonAfter="Sec" />
              </Space>
              <Space wrap>
                <span>Backend request timeout</span>
                <InputNumber min={1} max={86400} value={sendTimeout} onChange={(v) => setSendTimeout(v || 600)} addonAfter="Sec" />
              </Space>
              <Space wrap>
                <span>Proxy response timeout</span>
                <InputNumber min={1} max={86400} value={readTimeout} onChange={(v) => setReadTimeout(v || 600)} addonAfter="Sec" />
              </Space>
              {saveBtn}
            </Space>
          ),
        },
        {
          key: "config",
          label: "Config",
          children: (
            <>
              <Typography.Paragraph type="secondary">
                One Nginx directive per line, ending with a semicolon. Allowed: proxy_*, add_header, gzip, client_*, limit_rate.
              </Typography.Paragraph>
              <Input.TextArea rows={8} value={config} onChange={(e) => setConfig(e.target.value)} placeholder={'proxy_set_header X-Debug 1;\nclient_max_body_size 64m;'} />
              {saveBtn}
            </>
          ),
        },
        {
          key: "replace",
          label: "Replacement",
          children: (
            <>
              <Space wrap style={{ marginBottom: 12 }}>
                <Input placeholder="Original" value={repFrom} onChange={(e) => setRepFrom(e.target.value)} />
                <Input placeholder="Replacement" value={repTo} onChange={(e) => setRepTo(e.target.value)} />
                <Select style={{ width: 200 }} value={repRule} options={RULES} onChange={setRepRule} />
                <Button
                  type="primary"
                  onClick={() => {
                    if (!repFrom.trim()) return;
                    setReplacements((list) => [...list, { from: repFrom.trim(), to: repTo, rule: repRule }]);
                    setRepFrom("");
                    setRepTo("");
                  }}
                >
                  Add content replacement
                </Button>
              </Space>
              <Table
                size="small"
                rowKey={(r) => `${r.from}-${r.to}-${r.rule}`}
                pagination={false}
                dataSource={replacements}
                locale={{ emptyText: "No data" }}
                columns={[
                  { title: "Original keyword", dataIndex: "from" },
                  { title: "Alternative word", dataIndex: "to" },
                  { title: "Rule", dataIndex: "rule", width: 80 },
                  {
                    title: "Operate",
                    width: 90,
                    render: (_, row) => (
                      <Button type="link" danger onClick={() => setReplacements((list) => list.filter((r) => r !== row))}>
                        Delete
                      </Button>
                    ),
                  },
                ]}
              />
              <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                g replaces every match. o replaces the first match. Nginx matches the exact text, so i and r are kept on the rule but do not change the match.
              </Typography.Paragraph>
              {saveBtn}
            </>
          ),
        },
        {
          key: "cache",
          label: "Cache",
          children: (
            <>
              <Space>
                <span>Cache</span>
                <Switch checked={cache} onChange={setCache} />
              </Space>
              <Typography.Paragraph type="secondary">
                Caches successful responses for one hour. Set-Cookie, Cache-Control, Expires, and X-Accel-Expires from the backend are ignored. Requests with Authorization are not cached.
              </Typography.Paragraph>
              {saveBtn}
            </>
          ),
        },
        {
          key: "gzip",
          label: "Compressed",
          children: (
            <>
              <Space>
                <span>Compressed content</span>
                <Switch checked={gzip} onChange={setGzip} />
              </Space>
              <Typography.Paragraph type="secondary">Compresses text responses from this proxy. Level 5 balances speed and size.</Typography.Paragraph>
              {saveBtn}
            </>
          ),
        },
        {
          key: "black",
          label: "Black list",
          children: (
            <>
              <Input.TextArea rows={6} value={black} onChange={(e) => setBlack(e.target.value)} placeholder={"203.0.113.5\n192.0.2.0/24"} />
              <Typography.Paragraph type="secondary">One IP or CIDR per line. These addresses are denied.</Typography.Paragraph>
              {saveBtn}
            </>
          ),
        },
        {
          key: "white",
          label: "White list",
          children: (
            <>
              <Input.TextArea rows={6} value={white} onChange={(e) => setWhite(e.target.value)} placeholder={"203.0.113.8\n192.0.2.10"} />
              <Typography.Paragraph type="secondary">When this list is not empty, only these addresses can use the proxy.</Typography.Paragraph>
              {saveBtn}
            </>
          ),
        },
      ]}
    />
  );
}
