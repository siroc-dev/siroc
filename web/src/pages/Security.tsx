import { FormEvent, useEffect, useState } from "react";
import { Alert, App, AutoComplete, Button, Card, Checkbox, Col, Input, Popconfirm, Row, Select, Space, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { ScanSummary, type ScanResult } from "@/components/ScanSummary";
import { useNavigate } from "react-router-dom";

type Rule = { id: number; action: string; port: string; proto: string; source: string };
type FW = { installed: boolean; active: boolean; defaultIncoming: string; defaultPorts?: string[]; rules: Rule[]; message?: string };
type WAFPack = { id: string; title: string; description: string; enabled: boolean; available: boolean };
type WAF = {
  installed: boolean;
  enabled: boolean;
  mode: string;
  version?: string;
  crs?: boolean;
  paranoia: number;
  inboundThreshold: number;
  outboundThreshold: number;
  audit: string;
  packs?: WAFPack[];
  disabledIds?: number[];
  message?: string;
};
type CRSRule = { id: number; msg: string; file: string; pack: string; disabled: boolean };
type AV = { installed: boolean; version?: string; daemonActive: boolean; freshclamActive: boolean; signatures?: string; lastScan?: string };
type Scan = { ok: boolean; path: string; infected: number; summary: string };
type ScanTool = { id: string; title: string; installed: boolean };
type Scanners = { tools: ScanTool[] };
type SiteOpt = { id: number; domain: string; aliases?: string[] };

export function Security() {
  const { message } = App.useApp();
  const nav = useNavigate();
  const [fw, setFw] = useState<FW | null>(null);
  const [waf, setWaf] = useState<WAF | null>(null);
  const [av, setAv] = useState<AV | null>(null);
  const [port, setPort] = useState("8080");
  const [proto, setProto] = useState("tcp");
  const [action, setAction] = useState("allow");
  const [source, setSource] = useState("");
  const [scanPath, setScanPath] = useState("/home");
  const [scan, setScan] = useState<Scan | null>(null);
  const [scanners, setScanners] = useState<Scanners | null>(null);
  const [sites, setSites] = useState<SiteOpt[]>([]);
  const [scanTarget, setScanTarget] = useState("http://127.0.0.1:80");
  const [scanOut, setScanOut] = useState<ScanResult | null>(null);
  const [busy, setBusy] = useState("");

  async function load() {
    const [f, w, a, sc, siteList] = await Promise.all([
      api.get<FW>("/api/security/firewall"),
      api.get<WAF>("/api/security/waf"),
      api.get<AV>("/api/security/av"),
      api.get<Scanners>("/api/security/scanners").catch(() => ({ tools: [] })),
      api.get<SiteOpt[]>("/api/sites").catch(() => [] as SiteOpt[]),
    ]);
    setFw(f);
    setWaf(w);
    setAv(a);
    setScanners(sc);
    setSites(siteList);
    if (siteList[0] && scanTarget === "http://127.0.0.1:80") {
      setScanTarget(`http://${siteList[0].domain}:8080`);
    }
  }
  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  async function run(name: string, fn: () => Promise<void>) {
    setBusy(name);
    try {
      await fn();
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Security
        </Typography.Title>
        <Typography.Paragraph type="secondary">
          Install UFW, ModSecurity, ClamAV, Nikto, OWASP ZAP, and OpenVAS from Software first. Enabling the firewall always allows SSH 22, FTP 21, HTTP 80, HTTPS 443, and the Siroc web port. Vulnerability scanners can target a hosted site or any other http(s) URL.
        </Typography.Paragraph>
      </div>

      <Card
        title="UFW firewall"
        extra={<Tag color={!fw?.installed ? "default" : fw.active ? "success" : "warning"}>{!fw?.installed ? "Not installed" : fw.active ? "Active" : "Inactive"}</Tag>}
      >
        {fw?.installed ? (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <Space>
              <Button type="primary" loading={busy === "fw"} onClick={() => run("fw", () => api.post("/api/security/firewall/enable", { enable: true }))}>
                Enable
              </Button>
              <Button loading={busy === "fw"} onClick={() => run("fw", () => api.post("/api/security/firewall/enable", { enable: false }))}>
                Disable
              </Button>
            </Space>
            <Space wrap>
              <Typography.Text type="secondary">Default allow:</Typography.Text>
              {(fw.defaultPorts || ["22", "21", "80", "443", "8443"]).map((p) => (
                <Button
                  key={p}
                  size="small"
                  disabled={!!busy}
                  onClick={() => {
                    setPort(p);
                    setProto("tcp");
                    setAction("allow");
                    run("rule", () => api.post("/api/security/firewall/rules", { action: "allow", port: p, proto: "tcp" }));
                  }}
                >
                  {p === "22" ? "22 SSH" : p === "21" ? "21 FTP" : p === "80" ? "80 HTTP" : p === "443" ? "443 HTTPS" : `Siroc ${p}`}
                </Button>
              ))}
            </Space>
            <Space wrap>
              <Select value={action} onChange={setAction} options={[{ value: "allow", label: "allow" }, { value: "deny", label: "deny" }]} />
              <Input style={{ width: 100 }} value={port} onChange={(e) => setPort(e.target.value)} placeholder="Port" />
              <Select value={proto} onChange={setProto} options={[{ value: "tcp" }, { value: "udp" }, { value: "any" }]} />
              <Input style={{ width: 160 }} value={source} onChange={(e) => setSource(e.target.value)} placeholder="Source CIDR" />
              <Button type="primary" loading={busy === "rule"} onClick={() => run("rule", () => api.post("/api/security/firewall/rules", { action, port, proto, source }))}>
                Add rule
              </Button>
            </Space>
            <Table
              size="small"
              rowKey="id"
              pagination={false}
              dataSource={fw.rules || []}
              locale={{ emptyText: "No numbered rules yet." }}
              columns={[
                { title: "#", dataIndex: "id", width: 60 },
                { title: "Action", dataIndex: "action" },
                { title: "Port", render: (_, r) => `${r.port}/${r.proto}` },
                { title: "From", dataIndex: "source" },
                {
                  title: "",
                  align: "right",
                  render: (_, r) => (
                    <Popconfirm title="Delete this rule?" onConfirm={() => run("del", () => api.post("/api/security/firewall/delete", { id: r.id }))}>
                      <Button size="small" type="link" danger>
                        Delete
                      </Button>
                    </Popconfirm>
                  ),
                },
              ]}
            />
          </Space>
        ) : (
          <Typography.Text type="secondary">Install UFW from Software.</Typography.Text>
        )}
      </Card>

      <WAFCard waf={waf} busy={busy} run={run} />

      <Card
        title="ClamAV antivirus"
        extra={<Tag color={!av?.installed ? "default" : av.daemonActive ? "success" : "warning"}>{!av?.installed ? "Not installed" : av.daemonActive ? "Daemon running" : "Installed"}</Tag>}
      >
        {av?.installed ? (
          <Space direction="vertical" style={{ width: "100%" }}>
            <Typography.Text type="secondary">{av.signatures || av.version}</Typography.Text>
            {av.lastScan ? (
              <Typography.Paragraph code style={{ whiteSpace: "pre-wrap" }}>
                {av.lastScan}
              </Typography.Paragraph>
            ) : null}
            <Space wrap>
              <Button loading={busy === "fresh"} onClick={() => run("fresh", () => api.post("/api/security/av/update"))}>
                Update signatures
              </Button>
              <Input style={{ width: 240 }} value={scanPath} onChange={(e) => setScanPath(e.target.value)} />
              <Button
                type="primary"
                loading={busy === "scan"}
                onClick={async () => {
                  setBusy("scan");
                  try {
                    const res = await api.post<Scan>("/api/security/av/scan", { path: scanPath });
                    setScan(res);
                    message.success(res.infected ? `Found ${res.infected} infected files` : "Scan clean");
                    await load();
                  } catch (err) {
                    message.error(err instanceof Error ? err.message : "Scan failed");
                  } finally {
                    setBusy("");
                  }
                }}
              >
                Scan
              </Button>
            </Space>
            {scan ? (
              <Alert type={scan.infected ? "error" : "success"} message={<pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>{scan.summary}</pre>} />
            ) : null}
          </Space>
        ) : (
          <Typography.Text type="secondary">Install ClamAV from Software.</Typography.Text>
        )}
      </Card>

      <Card
        title="Vulnerability scanners"
        extra={
          <Button type="link" onClick={() => nav("/scan-logs")}>
            Scan logs
          </Button>
        }
      >
        <Space direction="vertical" style={{ width: "100%" }} size="middle">
          <Typography.Text type="secondary">
            Type any http(s) URL or pick a hosted site. Scanners run on this server. Results are stored under Scan logs.
          </Typography.Text>
          <AutoComplete
            style={{ minWidth: 360, width: "min(100%, 560px)" }}
            value={scanTarget}
            options={[
              { value: "http://127.0.0.1:80", label: "http://127.0.0.1:80" },
              ...sites.flatMap((s) => {
                const names = [s.domain, ...(s.aliases || [])];
                return names.flatMap((d) => [
                  { value: `http://${d}:8080`, label: `http://${d}:8080` },
                  { value: `https://${d}:8444`, label: `https://${d}:8444` },
                  { value: `http://${d}`, label: `http://${d}` },
                  { value: `https://${d}`, label: `https://${d}` },
                ]);
              }),
            ]}
            placeholder="https://example.com"
            filterOption={(input, option) => String(option?.value || "").toLowerCase().includes(input.toLowerCase())}
            onChange={(v) => setScanTarget(v)}
          />
          <Space wrap>
            {(scanners?.tools || [
              { id: "nikto", title: "Nikto", installed: false },
              { id: "zap", title: "OWASP ZAP", installed: false },
              { id: "openvas", title: "OpenVAS", installed: false },
            ]).map((t) => (
              <Button
                key={t.id}
                type={t.id === "zap" ? "primary" : "default"}
                disabled={!t.installed}
                loading={busy === "scan-" + t.id}
                onClick={async () => {
                  const target = scanTarget.trim();
                  if (!target) {
                    message.warning("Enter a URL to scan");
                    return;
                  }
                  setBusy("scan-" + t.id);
                  try {
                    const res = await api.post<ScanResult>("/api/security/scan", { tool: t.id, target });
                    setScanOut(res);
                    message.success(`${t.title} finished`);
                  } catch (err) {
                    message.error(err instanceof Error ? err.message : "Scan failed");
                  } finally {
                    setBusy("");
                  }
                }}
              >
                {t.installed ? `Run ${t.title}` : `Install ${t.title} first`}
              </Button>
            ))}
          </Space>
          {scanOut ? (
            <ScanSummary
              scan={scanOut}
              extra={
                <Space>
                  {scanOut.report ? (
                    <Typography.Link href={scanOut.report} target="_blank" rel="noreferrer">
                      Open original report
                    </Typography.Link>
                  ) : null}
                  {scanOut.id ? (
                    <Typography.Link onClick={() => nav(`/scan-logs?id=${encodeURIComponent(scanOut.id)}`)}>
                      View in Scan logs
                    </Typography.Link>
                  ) : null}
                </Space>
              }
            />
          ) : null}
        </Space>
      </Card>
    </div>
  );
}

function WAFCard({
  waf,
  busy,
  run,
}: {
  waf: WAF | null;
  busy: string;
  run: (name: string, fn: () => Promise<void>) => Promise<void>;
}) {
  const { message } = App.useApp();
  const [paranoia, setParanoia] = useState(1);
  const [inbound, setInbound] = useState(5);
  const [outbound, setOutbound] = useState(4);
  const [audit, setAudit] = useState("RelevantOnly");
  const [packs, setPacks] = useState<string[]>([]);
  const [disabledIds, setDisabledIds] = useState<number[]>([]);
  const [crsRules, setCrsRules] = useState<CRSRule[]>([]);
  const [crsTotal, setCrsTotal] = useState(0);
  const [ruleQ, setRuleQ] = useState("");
  const [rulePack, setRulePack] = useState("");
  const [newId, setNewId] = useState("");

  useEffect(() => {
    if (!waf) return;
    setParanoia(waf.paranoia || 1);
    setInbound(waf.inboundThreshold || 5);
    setOutbound(waf.outboundThreshold || 4);
    setAudit(waf.audit || "RelevantOnly");
    setPacks((waf.packs || []).filter((p) => p.enabled).map((p) => p.id));
    setDisabledIds(waf.disabledIds || []);
  }, [waf]);

  async function loadRules(q = ruleQ, pack = rulePack) {
    const data = await api.get<{ rules: CRSRule[]; total: number }>(`/api/security/waf/rules?q=${encodeURIComponent(q)}&pack=${encodeURIComponent(pack)}`);
    setCrsRules(data.rules || []);
    setCrsTotal(data.total || 0);
  }

  useEffect(() => {
    if (!waf?.installed) return;
    loadRules().catch(() => undefined);
  }, [waf?.installed]);

  function save(extra: Partial<{ mode: string; paranoia: number; inboundThreshold: number; outboundThreshold: number; audit: string; packs: string[]; disabledIds: number[] }> = {}) {
    const ids = extra.disabledIds ?? disabledIds;
    return run("waf", async () => {
      await api.post("/api/security/waf", {
        mode: extra.mode || waf?.mode,
        paranoia: extra.paranoia ?? paranoia,
        inboundThreshold: extra.inboundThreshold ?? inbound,
        outboundThreshold: extra.outboundThreshold ?? outbound,
        audit: extra.audit ?? audit,
        packs: extra.packs ?? packs,
        disabledIds: ids,
      });
      await loadRules();
    });
  }

  function profile(name: "monitor" | "balanced" | "strict" | "maximum") {
    const all = (waf?.packs || []).filter((p) => p.available !== false).map((p) => p.id);
    const next = {
      monitor: { mode: "DetectionOnly", paranoia: 1, inboundThreshold: 5, outboundThreshold: 4, packs: all },
      balanced: { mode: "On", paranoia: 1, inboundThreshold: 5, outboundThreshold: 4, packs: all },
      strict: { mode: "On", paranoia: 2, inboundThreshold: 5, outboundThreshold: 4, packs: all },
      maximum: { mode: "On", paranoia: 3, inboundThreshold: 3, outboundThreshold: 3, packs: all },
    }[name];
    setParanoia(next.paranoia);
    setInbound(next.inboundThreshold);
    setOutbound(next.outboundThreshold);
    setPacks(next.packs);
    return save(next);
  }

  function toggle(id: string) {
    setPacks((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));
  }

  function setRuleDisabled(id: number, disabled: boolean) {
    const next = disabled ? Array.from(new Set([...disabledIds, id])).sort((a, b) => a - b) : disabledIds.filter((x) => x !== id);
    setDisabledIds(next);
    return save({ disabledIds: next });
  }

  function addRuleId(e: FormEvent) {
    e.preventDefault();
    const id = Number(newId.trim());
    if (!Number.isInteger(id) || id < 100) {
      message.error("Enter a numeric rule ID");
      return;
    }
    setNewId("");
    return setRuleDisabled(id, true);
  }

  return (
    <Card
      title="ModSecurity WAF"
      extra={<Tag color={!waf?.installed ? "default" : waf.mode === "On" ? "success" : waf.mode === "DetectionOnly" ? "warning" : "default"}>{waf?.installed ? waf.mode : "Not installed"}</Tag>}
    >
      {waf?.installed ? (
        <Space direction="vertical" style={{ width: "100%" }} size="large">
          <div>
            <Typography.Text type="secondary">Engine</Typography.Text>
            <div style={{ marginTop: 8 }}>
              <Space wrap>
                {(["Off", "DetectionOnly", "On"] as const).map((mode) => (
                  <Button key={mode} type={waf.mode === mode ? "primary" : "default"} disabled={!!busy} onClick={() => save({ mode })}>
                    {mode === "DetectionOnly" ? "Detection only" : mode}
                  </Button>
                ))}
              </Space>
            </div>
          </div>
          <div>
            <Typography.Text type="secondary">Presets</Typography.Text>
            <div style={{ marginTop: 8 }}>
              <Space wrap>
                <Button disabled={!!busy} onClick={() => profile("monitor")}>Monitor</Button>
                <Button disabled={!!busy} onClick={() => profile("balanced")}>Balanced</Button>
                <Button disabled={!!busy} onClick={() => profile("strict")}>Strict</Button>
                <Button disabled={!!busy} onClick={() => profile("maximum")}>Maximum</Button>
              </Space>
            </div>
            <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              Monitor = detect only. Balanced = block at paranoia 1. Strict = paranoia 2. Maximum = paranoia 3 and lower anomaly scores.
            </Typography.Paragraph>
          </div>
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Typography.Text type="secondary">Paranoia level</Typography.Text>
              <Select
                style={{ width: "100%", marginTop: 4 }}
                value={paranoia}
                onChange={setParanoia}
                options={[
                  { value: 1, label: "1 — default, few false positives" },
                  { value: 2, label: "2 — extra checks" },
                  { value: 3, label: "3 — strict" },
                  { value: 4, label: "4 — very strict" },
                ]}
              />
            </Col>
            <Col xs={24} md={12}>
              <Typography.Text type="secondary">Audit log</Typography.Text>
              <Select
                style={{ width: "100%", marginTop: 4 }}
                value={audit}
                onChange={setAudit}
                options={[
                  { value: "Off", label: "Off" },
                  { value: "RelevantOnly", label: "Relevant only" },
                  { value: "On", label: "Full" },
                ]}
              />
            </Col>
            <Col xs={24} md={12}>
              <Typography.Text type="secondary">Inbound anomaly score</Typography.Text>
              <Select
                style={{ width: "100%", marginTop: 4 }}
                value={inbound}
                onChange={setInbound}
                options={[
                  { value: 10, label: "10 — low (fewer blocks)" },
                  { value: 5, label: "5 — CRS default" },
                  { value: 3, label: "3 — high" },
                ]}
              />
            </Col>
            <Col xs={24} md={12}>
              <Typography.Text type="secondary">Outbound anomaly score</Typography.Text>
              <Select
                style={{ width: "100%", marginTop: 4 }}
                value={outbound}
                onChange={setOutbound}
                options={[
                  { value: 8, label: "8 — low" },
                  { value: 4, label: "4 — CRS default" },
                  { value: 3, label: "3 — high" },
                ]}
              />
            </Col>
          </Row>
          <div>
            <Typography.Text type="secondary">OWASP CRS rule packs</Typography.Text>
            {waf.message ? <Alert type="warning" message={waf.message} style={{ margin: "8px 0" }} /> : null}
            <Row gutter={[8, 8]} style={{ marginTop: 8 }}>
              {(waf.packs || []).map((p) => (
                <Col xs={24} md={12} key={p.id}>
                  <Checkbox checked={packs.includes(p.id)} disabled={!p.available} onChange={() => toggle(p.id)}>
                    <Typography.Text strong>{p.title}</Typography.Text>
                    <div>
                      <Typography.Text type="secondary">{p.description}</Typography.Text>
                    </div>
                  </Checkbox>
                </Col>
              ))}
            </Row>
          </div>
          <div>
            <Typography.Text type="secondary">Disable rule IDs</Typography.Text>
            <Typography.Paragraph type="secondary">Turn off individual CRS rules without disabling the whole pack.</Typography.Paragraph>
            <Space wrap style={{ marginBottom: 12 }}>
              <Input style={{ width: 140 }} value={newId} onChange={(e) => setNewId(e.target.value)} placeholder="941100" onPressEnter={(e) => addRuleId(e as unknown as FormEvent)} />
              <Button disabled={!!busy} onClick={(e) => addRuleId(e as unknown as FormEvent)}>
                Disable ID
              </Button>
            </Space>
            {disabledIds.length ? (
              <Space wrap style={{ marginBottom: 12 }}>
                {disabledIds.map((id) => (
                  <Tag key={id} closable onClose={() => setRuleDisabled(id, false)}>
                    {id}
                  </Tag>
                ))}
              </Space>
            ) : (
              <Typography.Paragraph type="secondary">No rule IDs disabled.</Typography.Paragraph>
            )}
            <Space wrap style={{ marginBottom: 12 }}>
              <Input
                style={{ width: 220 }}
                value={ruleQ}
                onChange={(e) => setRuleQ(e.target.value)}
                placeholder="id, message, xss, sql…"
                onPressEnter={() => loadRules(ruleQ, rulePack).catch((err) => message.error(err.message))}
              />
              <Select
                style={{ width: 180 }}
                value={rulePack}
                onChange={(v) => {
                  setRulePack(v);
                  loadRules(ruleQ, v).catch((err) => message.error(err.message));
                }}
                options={[{ value: "", label: "all packs" }, ...(waf.packs || []).map((p) => ({ value: p.id, label: p.title }))]}
              />
              <Button onClick={() => loadRules().catch((err) => message.error(err.message))}>Search</Button>
            </Space>
            <Table
              size="small"
              rowKey="id"
              pagination={false}
              scroll={{ y: 320 }}
              dataSource={crsRules}
              locale={{ emptyText: "No matching rules. Install OWASP CRS or try another search." }}
              columns={[
                { title: "ID", dataIndex: "id", width: 100, render: (id: number) => <Typography.Text code>{id}</Typography.Text> },
                {
                  title: "Message",
                  render: (_, r) => (
                    <div>
                      <div>{r.msg}</div>
                      <Typography.Text type="secondary">{r.file}</Typography.Text>
                    </div>
                  ),
                },
                {
                  title: "",
                  width: 110,
                  align: "right",
                  render: (_, r) => {
                    const off = disabledIds.includes(r.id);
                    return (
                      <Button size="small" type={off ? "primary" : "default"} disabled={!!busy} onClick={() => setRuleDisabled(r.id, !off)}>
                        {off ? "Disabled" : "Enabled"}
                      </Button>
                    );
                  },
                },
              ]}
            />
            <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              Showing {crsRules.length} of {crsTotal} rules.
            </Typography.Paragraph>
          </div>
          <Button type="primary" loading={busy === "waf"} onClick={() => save()}>
            Save WAF settings
          </Button>
        </Space>
      ) : (
        <Typography.Text type="secondary">Install ModSecurity WAF from Software (Apache should be installed first). This also installs OWASP CRS.</Typography.Text>
      )}
    </Card>
  );
}
