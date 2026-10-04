import { FormEvent, useEffect, useState } from "react";
import { App, Button, Input, Select, Space, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";

type Rule = { id: number; msg: string; file: string; pack: string; disabled: boolean };
type Resp = { rules?: Rule[]; total?: number; disabledIds?: number[] };

export function SiteWAFRules({ siteId, domain }: { siteId: number; domain?: string }) {
  const { message } = App.useApp();
  const [rules, setRules] = useState<Rule[]>([]);
  const [total, setTotal] = useState(0);
  const [off, setOff] = useState<number[]>([]);
  const [q, setQ] = useState("");
  const [pack, setPack] = useState("");
  const [newId, setNewId] = useState("");
  const [busy, setBusy] = useState(false);

  async function load(query = q, nextPack = pack) {
    const data = await api.get<Resp>(
      `/api/sites/${siteId}/waf/rules?q=${encodeURIComponent(query)}&pack=${encodeURIComponent(nextPack)}`,
    );
    setRules(data.rules || []);
    setTotal(data.total || 0);
    setOff(data.disabledIds || []);
  }

  useEffect(() => {
    load().catch((e) => message.error(e instanceof Error ? e.message : "Failed to load rules"));
  }, [siteId]);

  async function save(ids: number[]) {
    setBusy(true);
    try {
      const st = await api.put<{ wafDisabledIds?: number[] }>(`/api/sites/${siteId}/waf/rules`, { disabledIds: ids });
      setOff(st.wafDisabledIds || ids);
      await load();
      message.success(domain ? `Rule exceptions saved for ${domain}` : "Rule exceptions saved");
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Failed to save rules");
    } finally {
      setBusy(false);
    }
  }

  function toggle(id: number, disabled: boolean) {
    const next = disabled ? Array.from(new Set([...off, id])).sort((a, b) => a - b) : off.filter((x) => x !== id);
    return save(next);
  }

  function addId(e: FormEvent) {
    e.preventDefault();
    const id = Number(newId.trim());
    if (!Number.isInteger(id) || id < 100) {
      message.error("Enter a numeric rule ID");
      return;
    }
    setNewId("");
    return toggle(id, true);
  }

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
        Turn off individual ModSecurity rules for this website only. Other websites keep the global rules.
      </Typography.Paragraph>
      <Space wrap style={{ marginBottom: 12 }}>
        <Input
          style={{ width: 140 }}
          value={newId}
          placeholder="942100"
          onChange={(e) => setNewId(e.target.value)}
          onPressEnter={(e) => addId(e as unknown as FormEvent)}
        />
        <Button disabled={busy} onClick={(e) => addId(e as unknown as FormEvent)}>
          Disable ID
        </Button>
      </Space>
      {off.length ? (
        <Space wrap style={{ marginBottom: 12 }}>
          {off.map((id) => (
            <Tag key={id} closable onClose={() => void toggle(id, false)}>
              {id}
            </Tag>
          ))}
        </Space>
      ) : (
        <Typography.Paragraph type="secondary">No rules disabled on this website.</Typography.Paragraph>
      )}
      <Space wrap style={{ marginBottom: 12 }}>
        <Input
          style={{ width: 240 }}
          value={q}
          placeholder="id, message, xss, sql…"
          onChange={(e) => setQ(e.target.value)}
          onPressEnter={() => load().catch((e) => message.error(e.message))}
        />
        <Select
          style={{ width: 180 }}
          value={pack}
          onChange={(v) => {
            setPack(v);
            load(q, v).catch((e) => message.error(e.message));
          }}
          options={[
            { value: "", label: "All packs" },
            { value: "sqli", label: "SQL injection" },
            { value: "xss", label: "Cross-site scripting" },
            { value: "rce", label: "Remote code execution" },
            { value: "lfi", label: "Local file inclusion" },
            { value: "php", label: "PHP attacks" },
            { value: "protocol", label: "Protocol" },
            { value: "scanner", label: "Scanner detection" },
          ]}
        />
        <Button onClick={() => load().catch((e) => message.error(e.message))}>Search</Button>
      </Space>
      <Table
        size="small"
        rowKey="id"
        pagination={false}
        scroll={{ y: 320 }}
        dataSource={rules}
        locale={{ emptyText: "No matching rules. Install OWASP CRS, or disable a rule by ID." }}
        columns={[
          { title: "ID", dataIndex: "id", width: 100, render: (id: number) => <Typography.Text code>{id}</Typography.Text> },
          {
            title: "Message",
            render: (_, r) => (
              <div>
                <div>{r.msg || "—"}</div>
                <Typography.Text type="secondary">{r.file}</Typography.Text>
              </div>
            ),
          },
          {
            title: "",
            width: 120,
            align: "right" as const,
            render: (_, r) => {
              const disabled = off.includes(r.id);
              return (
                <Button size="small" type={disabled ? "primary" : "default"} disabled={busy} onClick={() => void toggle(r.id, !disabled)}>
                  {disabled ? "Off here" : "On"}
                </Button>
              );
            },
          },
        ]}
      />
      <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
        Showing {rules.length} of {total} rules.
      </Typography.Paragraph>
    </div>
  );
}
