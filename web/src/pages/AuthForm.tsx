import { useState } from "react";
import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { api } from "@/lib/api";

export type SetupStackPkg = { name: string; title: string; version: string };

export function AuthForm({
  title,
  subtitle,
  submitLabel,
  endpoint,
  extra,
  setup,
  stack,
  onDone,
}: {
  title: string;
  subtitle: string;
  submitLabel: string;
  endpoint: string;
  extra?: Record<string, string>;
  setup?: boolean;
  stack?: SetupStackPkg[];
  onDone: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onFinish(values: { username: string; password: string; leEmail?: string }) {
    setBusy(true);
    setError("");
    try {
      await api.post(endpoint, { ...values, ...extra });
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: "grid", placeItems: "center", minHeight: "100vh", padding: 24 }}>
      <Card style={{ width: "100%", maxWidth: 420 }}>
        <Typography.Title level={3} style={{ marginTop: 0 }}>
          {title}
        </Typography.Title>
        <Typography.Paragraph type="secondary">{subtitle}</Typography.Paragraph>
        <Form layout="vertical" onFinish={onFinish} requiredMark={false}>
          <Form.Item name="username" label="Username" rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true, min: 8 }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          {setup ? (
            <Form.Item
              name="leEmail"
              label="Let's Encrypt email"
              extra="Registers a Let's Encrypt account used later to issue certificates."
              rules={[{ required: true, type: "email", message: "Enter a valid email" }]}
            >
              <Input type="email" autoComplete="email" placeholder="admin@example.com" />
            </Form.Item>
          ) : null}
          {setup ? (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message="Required stack"
              description={
                stack?.length
                  ? `After setup, Siroc installs ${stack
                      .map((p) => (p.version ? `${p.title} ${p.version}` : p.title))
                      .join(", ")}.`
                  : "After setup, Siroc installs Nginx, Apache, PHP-FPM 8.4, and MariaDB 11."
              }
            />
          ) : null}
          {error ? <Alert type="error" message={error} style={{ marginBottom: 16 }} /> : null}
          <Button type="primary" htmlType="submit" loading={busy} block>
            {submitLabel}
          </Button>
        </Form>
      </Card>
    </div>
  );
}
