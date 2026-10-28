import { useEffect, useState } from "react";
import { App, Card, Flex, Select, Space, Switch, Typography } from "antd";
import { api } from "@/lib/api";
import { SiteWAFRules } from "@/components/SiteWAFRules";

type Account = { username: string; wafEnabled?: boolean };
type Site = { id: number; username: string; domain: string; displayName?: string };

function siteLabel(s: { domain: string; displayName?: string }) {
  return s.displayName || s.domain;
}

export function AccountWAF({ admin, onChanged }: { admin?: boolean; onChanged?: () => void }) {
  const { message } = App.useApp();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [sites, setSites] = useState<Site[]>([]);
  const [ruleSiteId, setRuleSiteId] = useState(0);
  const [wafBusy, setWafBusy] = useState("");

  async function load() {
    const [a, s] = await Promise.all([api.get<Account[]>("/api/accounts"), api.get<Site[]>("/api/sites")]);
    setAccounts(Array.isArray(a) ? a : []);
    setSites(Array.isArray(s) ? s : []);
  }

  useEffect(() => {
    load().catch((e) => message.error(e instanceof Error ? e.message : "Failed"));
  }, []);

  async function setAccountWAF(username: string, enabled: boolean) {
    setWafBusy(`acc:${username}`);
    try {
      await api.put(`/api/accounts/${encodeURIComponent(username)}/waf`, { enabled });
      message.success(enabled ? "ModSecurity enabled for this account" : "ModSecurity disabled for this account");
      await load();
      onChanged?.();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setWafBusy("");
    }
  }

  const ruleSite = sites.find((s) => s.id === ruleSiteId) || sites[0];

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card title="ModSecurity WAF">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          {admin
            ? "Blocks common attacks on Apache (SQL injection, XSS). The global engine stays under Security. Turn WAF off for a whole account, or disable it on a single website in the table."
            : "Blocks common attacks on Apache (SQL injection, XSS). Turn WAF off for your whole account, or disable one rule on a single website."}
        </Typography.Paragraph>
        {accounts.length ? (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            {accounts.map((a) => (
              <Flex key={a.username} align="center" justify="space-between" gap={16}>
                <div>
                  <Typography.Text strong>{admin ? a.username : "Protect my websites"}</Typography.Text>
                  <div>
                    <Typography.Text type="secondary">
                      {a.wafEnabled !== false ? "On for this account" : "Off for this account — site switches are disabled"}
                    </Typography.Text>
                  </div>
                </div>
                <Switch
                  checked={a.wafEnabled !== false}
                  loading={wafBusy === `acc:${a.username}`}
                  onChange={(v) => void setAccountWAF(a.username, v)}
                />
              </Flex>
            ))}
          </Space>
        ) : (
          <Typography.Text type="secondary">Create a hosting account first.</Typography.Text>
        )}
      </Card>
      <Card title="Rule exceptions">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          Disable a rule on one website when it blocks a real request. The same rule stays on for every other site.
        </Typography.Paragraph>
        <Select
          style={{ minWidth: 260, marginBottom: 12 }}
          placeholder="Select a website"
          value={ruleSite?.id}
          options={sites.map((s) => ({ value: s.id, label: siteLabel(s) }))}
          onChange={setRuleSiteId}
        />
        {ruleSite ? <SiteWAFRules siteId={ruleSite.id} domain={ruleSite.domain} /> : <Typography.Text type="secondary">Add a website first.</Typography.Text>}
      </Card>
    </Space>
  );
}
