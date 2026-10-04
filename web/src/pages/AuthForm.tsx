import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { api, RequestError, type Captcha } from "@/lib/api";
import { Mark } from "@/components/Mark";
import { useBrand } from "@/components/ThemeProvider";

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
  const [captcha, setCaptcha] = useState<Captcha | null>(null);
  const [form] = Form.useForm();
  const { brand } = useBrand();

  function applyCaptcha(next?: Captcha | null) {
    setCaptcha(next || null);
    form.setFieldValue("captcha", "");
  }

  async function loadCaptcha() {
    if (setup) return;
    try {
      const st = await api.get<{ required: boolean; captcha?: Captcha }>("/api/login/captcha");
      applyCaptcha(st.required ? st.captcha || null : null);
    } catch {
      /* keep current */
    }
  }

  useEffect(() => {
    loadCaptcha().catch(() => undefined);
  }, [setup]);

  async function onFinish(values: { username: string; password: string; leEmail?: string; captcha?: string }) {
    setBusy(true);
    setError("");
    try {
      await api.post(endpoint, {
        ...values,
        ...extra,
        captchaId: captcha?.id,
        captcha: values.captcha,
      });
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
      if (err instanceof RequestError && (err.captchaRequired || err.captcha)) {
        applyCaptcha(err.captcha || null);
        if (!err.captcha) loadCaptcha().catch(() => undefined);
      } else if (!setup) {
        loadCaptcha().catch(() => undefined);
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="siroc-login">
      <Card className="siroc-login-card">
        <div className="siroc-login-brand">
          {brand.hasLogo && brand.logoUrl ? <img src={brand.logoUrl} alt="" className="brand-logo" /> : <Mark />}
          <Typography.Title level={2}>Siroc</Typography.Title>
        </div>
        <p className="siroc-login-note">{title === "Sign in" ? subtitle : title}</p>
        {title === "Sign in" ? null : <p className="siroc-login-note">{subtitle}</p>}
        <Form form={form} layout="vertical" onFinish={onFinish} requiredMark={false}>
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
          {captcha ? (
            <>
              <div className="login-captcha">
                <button type="button" className="login-captcha-img" onClick={() => void loadCaptcha()} title="Refresh captcha">
                  <img src={captcha.image} alt="Captcha" />
                </button>
                <Form.Item
                  name="captcha"
                  label="Captcha"
                  extra="Shown after 2 failed logins in 30 minutes. Click the image to refresh."
                  rules={[{ required: true, message: "Enter the captcha" }]}
                  style={{ flex: 1, marginBottom: 0 }}
                >
                  <Input autoComplete="off" placeholder="Letters and numbers" />
                </Form.Item>
              </div>
            </>
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
