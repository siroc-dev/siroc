import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { api, RequestError, type Captcha } from "@/lib/api";
import { Mark } from "@/components/Mark";
import { useBrand } from "@/components/ThemeProvider";

export function AuthForm({
  title,
  subtitle,
  submitLabel,
  endpoint,
  extra,
  setup,
  onDone,
}: {
  title: string;
  subtitle: string;
  submitLabel: string;
  endpoint: string;
  extra?: Record<string, string>;
  setup?: boolean;
  onDone: () => void;
}) {
  const [error, setError] = useState(() => (setup ? "" : new URLSearchParams(window.location.search).get("error") || ""));
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
    localStorage.removeItem("siroc.login");
    loadCaptcha().catch(() => undefined);
  }, [setup]);

  async function onFinish(values: { username: string; password: string; leEmail?: string; captcha?: string }) {
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

  const brandBlock = (
    <div className="siroc-login-brand">
      {brand.hasLogo && brand.logoUrl ? <img src={brand.logoUrl} alt="" className="brand-logo" /> : <Mark />}
      <Typography.Title level={2}>Siroc</Typography.Title>
    </div>
  );

  if (!setup) {
    return (
      <div className="siroc-login">
        <Card className="siroc-login-card">
          {brandBlock}
          <p className="siroc-login-note">{subtitle}</p>
          <form id="siroc-sign-in" method="post" action="/api/login" autoComplete="on">
            <div className="ant-form-item">
              <div className="ant-form-item-label">
                <label htmlFor="username">Username</label>
              </div>
              <div className="ant-form-item-control">
                <input
                  id="username"
                  name="username"
                  type="text"
                  autoComplete="username"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  className="siroc-login-input"
                  required
                />
              </div>
            </div>
            <div className="ant-form-item">
              <div className="ant-form-item-label">
                <label htmlFor="password">Password</label>
              </div>
              <div className="ant-form-item-control">
                <input
                  id="password"
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  className="siroc-login-input"
                  minLength={8}
                  required
                />
              </div>
            </div>
            {captcha ? (
              <div className="login-captcha">
                <button type="button" className="login-captcha-img" onClick={() => void loadCaptcha()} title="Refresh captcha">
                  <img src={captcha.image} alt="Captcha" />
                </button>
                <div className="ant-form-item" style={{ flex: 1, marginBottom: 0 }}>
                  <div className="ant-form-item-label">
                    <label htmlFor="captcha">Captcha</label>
                  </div>
                  <input type="hidden" name="captchaId" value={captcha.id} />
                  <input id="captcha" name="captcha" autoComplete="off" className="siroc-login-input" placeholder="Letters and numbers" required />
                  <div className="ant-form-item-extra">Shown after 2 failed logins in 30 minutes. Click the image to refresh.</div>
                </div>
              </div>
            ) : null}
            {error ? <Alert type="error" message={error} style={{ marginBottom: 16 }} /> : null}
            <Button type="primary" htmlType="submit" block>
              {submitLabel}
            </Button>
          </form>
        </Card>
      </div>
    );
  }

  return (
    <div className="siroc-login">
      <Card className="siroc-login-card">
        {brandBlock}
        <p className="siroc-login-note">{title}</p>
        <p className="siroc-login-note">{subtitle}</p>
        <Form layout="vertical" onFinish={onFinish} requiredMark={false} autoComplete="on">
          <Form.Item name="username" label="Username" rules={[{ required: true }]}>
            <Input name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true, min: 8 }]}>
            <Input.Password name="password" autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="leEmail"
            label="Let's Encrypt email"
            extra="Registers a Let's Encrypt account used later to issue certificates."
            rules={[{ required: true, type: "email", message: "Enter a valid email" }]}
          >
            <Input type="email" autoComplete="email" placeholder="admin@example.com" />
          </Form.Item>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="Software"
            description="Nothing is installed during setup. The first sign-in recommends Nginx, Apache, MariaDB or MySQL, Redis, Let's Encrypt, and FTP."
          />
          {error ? <Alert type="error" message={error} style={{ marginBottom: 16 }} /> : null}
          <Button type="primary" htmlType="submit" loading={busy} block>
            {submitLabel}
          </Button>
        </Form>
      </Card>
    </div>
  );
}
