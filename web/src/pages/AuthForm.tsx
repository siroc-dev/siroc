import { useEffect, useState } from "react";
import { Alert, Button, Card, Checkbox, Form, Input, Typography } from "antd";
import { api, RequestError, type Captcha } from "@/lib/api";
import { Mark } from "@/components/Mark";
import { useBrand } from "@/components/ThemeProvider";

export type SetupStackPkg = { name: string; title: string; version: string };

const loginMemoryKey = "siroc.login";

type SavedLogin = { username: string; password: string };

export function readSavedLogin(): SavedLogin | null {
  try {
    const raw = localStorage.getItem(loginMemoryKey);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as SavedLogin;
    if (!parsed || typeof parsed.username !== "string" || typeof parsed.password !== "string") return null;
    if (!parsed.username || !parsed.password) return null;
    return { username: parsed.username, password: parsed.password };
  } catch {
    return null;
  }
}

export function writeSavedLogin(username: string, password: string) {
  localStorage.setItem(loginMemoryKey, JSON.stringify({ username, password }));
}

export function clearSavedLogin() {
  localStorage.removeItem(loginMemoryKey);
}

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
  const [remember, setRemember] = useState(true);
  const [captcha, setCaptcha] = useState<Captcha | null>(null);
  const [form] = Form.useForm();
  const { brand } = useBrand();
  const [savedLogin] = useState(() => (setup ? null : readSavedLogin()));

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
    const root = document.getElementById("siroc-sign-in");
    const typedUser = root?.querySelector<HTMLInputElement>('input[name="username"]')?.value ?? "";
    const typedPass = root?.querySelector<HTMLInputElement>('input[name="password"]')?.value ?? "";
    const username = typedUser || values.username;
    const password = typedPass || values.password;
    setBusy(true);
    setError("");
    try {
      await api.post(endpoint, {
        ...values,
        username,
        password,
        ...extra,
        captchaId: captcha?.id,
        captcha: values.captcha,
      });
      if (!setup) {
        if (remember) writeSavedLogin(username, password);
        else clearSavedLogin();
      }
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
        <Form id="siroc-sign-in" form={form} layout="vertical" onFinish={onFinish} requiredMark={false} autoComplete="on" method="post">
          {setup ? (
            <>
              <Form.Item name="username" label="Username" rules={[{ required: true }]}>
                <Input name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} />
              </Form.Item>
              <Form.Item name="password" label="Password" rules={[{ required: true, min: 8 }]}>
                <Input.Password name="password" autoComplete="new-password" />
              </Form.Item>
            </>
          ) : (
            <>
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
                    defaultValue={savedLogin?.username}
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
                    defaultValue={savedLogin?.password}
                    minLength={8}
                    required
                  />
                </div>
              </div>
            </>
          )}
          {setup ? null : (
            <Form.Item style={{ marginBottom: 12 }}>
              <Checkbox checked={remember} onChange={(e) => setRemember(e.target.checked)}>
                Remember username and password
              </Checkbox>
            </Form.Item>
          )}
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
