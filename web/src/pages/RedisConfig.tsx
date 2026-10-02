import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, InputNumber, Popconfirm, Select, Space, Switch, Typography } from "antd";
import { api } from "@/lib/api";

export type RedisSettings = {
  installed: boolean;
  version?: string;
  bind: string;
  port: number;
  hasPassword: boolean;
  protectedMode: boolean;
  maxMemory: string;
  maxMemoryPolicy: string;
  appendOnly: boolean;
  timeout: number;
  databases: number;
  confPath?: string;
  conf?: string;
  message?: string;
};

export function RedisConfigPanel() {
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [st, setSt] = useState<RedisSettings | null>(null);
  const [conf, setConf] = useState("");
  const [err, setErr] = useState("");
  const [formBusy, setFormBusy] = useState(false);
  const [fileBusy, setFileBusy] = useState(false);

  async function load() {
    setErr("");
    try {
      const data = await api.get<RedisSettings>("/api/software/redis");
      setSt(data);
      setConf(data.conf || "");
      form.setFieldsValue({
        bind: data.bind || "127.0.0.1",
        port: data.port || 6379,
        password: "",
        clearPassword: false,
        protectedMode: data.protectedMode,
        maxMemory: data.maxMemory === "0" ? "" : data.maxMemory,
        maxMemoryPolicy: data.maxMemoryPolicy || "noeviction",
        appendOnly: data.appendOnly,
        timeout: data.timeout || 0,
        databases: data.databases || 16,
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to load Redis config");
    }
  }

  useEffect(() => {
    load().catch(() => undefined);
  }, []);

  async function saveForm(values: {
    bind: string;
    port: number;
    password?: string;
    clearPassword?: boolean;
    protectedMode: boolean;
    maxMemory?: string;
    maxMemoryPolicy: string;
    appendOnly: boolean;
    timeout: number;
    databases: number;
  }) {
    setFormBusy(true);
    try {
      const next = await api.put<RedisSettings>("/api/software/redis", {
        ...values,
        maxMemory: values.maxMemory?.trim() || "0",
      });
      setSt(next);
      setConf(next.conf || "");
      form.setFieldValue("password", "");
      form.setFieldValue("clearPassword", false);
      message.success("Redis settings saved and restarted");
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Failed");
    } finally {
      setFormBusy(false);
    }
  }

  async function saveFile() {
    setFileBusy(true);
    try {
      const next = await api.put<RedisSettings>("/api/software/redis/conf", { conf });
      setSt(next);
      setConf(next.conf || conf);
      form.setFieldsValue({
        bind: next.bind || "127.0.0.1",
        port: next.port || 6379,
        protectedMode: next.protectedMode,
        maxMemory: next.maxMemory === "0" ? "" : next.maxMemory,
        maxMemoryPolicy: next.maxMemoryPolicy || "noeviction",
        appendOnly: next.appendOnly,
        timeout: next.timeout || 0,
        databases: next.databases || 16,
        password: "",
        clearPassword: false,
      });
      message.success("redis.conf saved and Redis restarted");
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Failed");
    } finally {
      setFileBusy(false);
    }
  }

  if (err && !st) return <Alert type="error" showIcon message={err} />;
  if (!st) return <Typography.Text type="secondary">Loading Redis config…</Typography.Text>;
  if (!st.installed) return <Alert type="info" showIcon message="Install Redis from Software first." />;

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card size="small" title="Redis settings" extra={st.version ? <Typography.Text type="secondary">{st.version}</Typography.Text> : null}>
        <Form form={form} layout="vertical" onFinish={saveForm} requiredMark={false}>
          <Form.Item name="bind" label="Bind" extra="127.0.0.1 keeps Redis local. Use 0.0.0.0 only with a password.">
            <Input placeholder="127.0.0.1" />
          </Form.Item>
          <Form.Item name="port" label="Port">
            <InputNumber min={1} max={65535} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item name="password" label="Password" extra={st.hasPassword ? "A password is set. Leave blank to keep it." : "Leave blank for no password."}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="clearPassword" label="Remove password" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="protectedMode" valuePropName="checked" label="Protected mode">
            <Switch />
          </Form.Item>
          <Form.Item name="maxMemory" label="Max memory" extra="Examples: 256mb, 1gb. Empty means unlimited.">
            <Input placeholder="256mb" />
          </Form.Item>
          <Form.Item name="maxMemoryPolicy" label="Eviction policy">
            <Select
              options={[
                { value: "noeviction", label: "noeviction" },
                { value: "allkeys-lru", label: "allkeys-lru" },
                { value: "allkeys-lfu", label: "allkeys-lfu" },
                { value: "volatile-lru", label: "volatile-lru" },
                { value: "volatile-lfu", label: "volatile-lfu" },
                { value: "allkeys-random", label: "allkeys-random" },
                { value: "volatile-ttl", label: "volatile-ttl" },
              ]}
            />
          </Form.Item>
          <Form.Item name="appendOnly" valuePropName="checked" label="AOF persistence">
            <Switch />
          </Form.Item>
          <Form.Item name="timeout" label="Idle timeout (seconds)">
            <InputNumber min={0} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item name="databases" label="Logical databases">
            <InputNumber min={1} max={1024} style={{ width: "100%" }} />
          </Form.Item>
          <Popconfirm title="Save settings and restart Redis?" onConfirm={() => form.submit()}>
            <Button type="primary" htmlType="button" loading={formBusy}>
              Save settings and restart
            </Button>
          </Popconfirm>
        </Form>
      </Card>
      <Card
        size="small"
        title="redis.conf"
        extra={st.confPath ? <Typography.Text type="secondary">{st.confPath}</Typography.Text> : null}
      >
        <Input.TextArea
          value={conf}
          onChange={(e) => setConf(e.target.value)}
          spellCheck={false}
          autoSize={{ minRows: 16, maxRows: 32 }}
          style={{ fontFamily: "ui-monospace, Menlo, Consolas, monospace", fontSize: 12 }}
        />
        <Space wrap style={{ marginTop: 12 }}>
          <Popconfirm title="Write redis.conf and restart Redis?" onConfirm={() => void saveFile()}>
            <Button type="primary" loading={fileBusy} disabled={!conf.trim()}>
              Save file and restart
            </Button>
          </Popconfirm>
          <Button onClick={() => void load()}>Reload file</Button>
        </Space>
      </Card>
    </Space>
  );
}
