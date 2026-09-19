import { useEffect, useState } from "react";
import { useOutletContext } from "react-router-dom";
import { App, Button, Select, Space, Typography } from "antd";
import { api } from "@/lib/api";
import { SSHTerminal } from "@/components/SSHTerminal";

type Account = { username: string };

export function TerminalPage() {
  const { message } = App.useApp();
  const { admin } = useOutletContext<{ user: string; admin?: boolean }>();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [user, setUser] = useState("");
  const [session, setSession] = useState(0);

  useEffect(() => {
    api
      .get<Account[]>("/api/accounts")
      .then((a) => {
        setAccounts(a);
        if (a[0]) setUser(a[0].username);
        else if (admin) setUser("root");
      })
      .catch((e) => message.error(e.message));
  }, [admin, message]);

  const options = [
    ...(admin ? [{ value: "root", label: "System (root)" }] : []),
    ...accounts.map((a) => ({ value: a.username, label: a.username })),
  ];

  return (
    <div className="cp-page" style={{ height: "calc(100vh - 112px)", minHeight: 480 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Terminal
          </Typography.Title>
          <Typography.Text type="secondary">SSH login shell as the selected Linux user. Copy and paste from the toolbar, right-click, or keyboard shortcuts.</Typography.Text>
        </div>
        <Space>
          <Select style={{ minWidth: 200 }} value={user || undefined} placeholder="Select account" onChange={setUser} options={options} />
          <Button onClick={() => setSession((n) => n + 1)} disabled={!user}>
            Reconnect
          </Button>
        </Space>
      </div>
      {user ? <SSHTerminal key={`${user}-${session}`} user={user} /> : <Typography.Text type="secondary">Create a hosting account first.</Typography.Text>}
    </div>
  );
}
