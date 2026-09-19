import { useState } from "react";
import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { api } from "@/lib/api";

export function AuthForm({
  title,
  subtitle,
  submitLabel,
  endpoint,
  extra,
  onDone,
}: {
  title: string;
  subtitle: string;
  submitLabel: string;
  endpoint: string;
  extra?: Record<string, string>;
  onDone: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onFinish(values: { username: string; password: string }) {
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
          {error ? <Alert type="error" message={error} style={{ marginBottom: 16 }} /> : null}
          <Button type="primary" htmlType="submit" loading={busy} block>
            {submitLabel}
          </Button>
        </Form>
      </Card>
    </div>
  );
}
