import { useEffect, useState } from "react";
import { DownOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Col, Dropdown, Flex, Form, Input, InputNumber, Modal, Popconfirm, Progress, Row, Select, Space, Switch, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { elapsed, jobLabel, type InstallJob, type InstallQueue } from "@/lib/jobs";

type Pkg = {
  name: string;
  title: string;
  installed: boolean;
  version: string;
  service: string;
  active: boolean;
  versions?: string[];
  installedVersions?: string[];
  cliVersion?: string;
  exclusiveOf?: string;
};

export function Software() {
  const { message } = App.useApp();
  const [list, setList] = useState<Pkg[]>([]);
  const [jobs, setJobs] = useState<InstallJob[]>([]);
  const [current, setCurrent] = useState<InstallJob | null>(null);
  const [queue, setQueue] = useState<InstallJob[]>([]);
  const [busy, setBusy] = useState("");
  const [tick, setTick] = useState(0);
  const [cliPick, setCliPick] = useState<Record<string, string>>({});
  const [locking, setLocking] = useState<string[]>([]);
  const [redisOpen, setRedisOpen] = useState(false);
  const [redisBusy, setRedisBusy] = useState(false);
  const [redisForm] = Form.useForm();

  const titles = Object.fromEntries(list.map((p) => [p.name, p.title]));
  const label = (job: InstallJob) => (job.name === "php-ext" ? jobLabel(job) : `${titles[job.name] || job.name}${job.version ? ` ${job.version}` : ""}`);

  async function loadPkgs() {
    const pkgs = await api.get<Pkg[]>("/api/software");
    setList(pkgs);
    setCliPick((cur) => {
      const next = { ...cur };
      for (const p of pkgs) {
        if (p.cliVersion) next[p.name] = p.cliVersion;
        else if (p.installedVersions?.[0]) next[p.name] = p.installedVersions[0];
      }
      return next;
    });
  }

  async function loadJobs() {
    const data = await api.get<InstallQueue>("/api/software/jobs");
    const nextCurrent = data.current || null;
    const nextQueue = data.queue || [];
    setJobs(data.jobs || []);
    setCurrent(nextCurrent);
    setQueue(nextQueue);
    const active = nextCurrent ? [nextCurrent, ...nextQueue] : nextQueue;
    setLocking((cur) =>
      cur.filter((key) => {
        const [name, version = ""] = key.split("@");
        return active.some((j) => j.name === name && (!version || !j.version || j.version === version));
      }),
    );
    return data.active || 0;
  }

  useEffect(() => {
    loadPkgs().catch((e) => message.error(e.message));
    loadJobs().catch((e) => message.error(e.message));
    const t = setInterval(() => {
      loadJobs()
        .then((active) => {
          if (active === 0) loadPkgs().catch(() => undefined);
        })
        .catch(() => undefined);
    }, 2000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (!current) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [current?.id]);

  function jobKey(name: string, version = "") {
    return `${name}@${version}`;
  }

  function isQueued(name: string, version = "") {
    if (locking.includes(jobKey(name, version)) || locking.includes(jobKey(name, ""))) return true;
    const items = current ? [current, ...queue] : queue;
    return items.some((j) => j.name === name && (!version || !j.version || j.version === version));
  }

  async function update(name: string, version = "") {
    const ver = version ? `upgrade:${version}` : "upgrade";
    const key = jobKey(name, ver);
    if (isQueued(name, ver) || isQueued(name, version) || isQueued(name, "upgrade")) return;
    setLocking((cur) => (cur.includes(key) ? cur : [...cur, key]));
    try {
      await api.post("/api/software/install", { name, version: ver });
      message.success(`Queued update ${name} ${version}`.trim());
      await loadJobs();
    } catch (err) {
      setLocking((cur) => cur.filter((k) => k !== key));
      message.error(err instanceof Error ? err.message : "Queue failed");
    }
  }

  async function install(name: string, version = "") {
    const key = jobKey(name, version);
    if (isQueued(name, version)) return;
    setLocking((cur) => (cur.includes(key) ? cur : [...cur, key]));
    try {
      await api.post("/api/software/install", { name, version });
      message.success(`Queued ${name} ${version}`.trim());
      await loadJobs();
    } catch (err) {
      setLocking((cur) => cur.filter((k) => k !== key));
      message.error(err instanceof Error ? err.message : "Queue failed");
    }
  }

  async function cancel(id: number) {
    try {
      await api.delete(`/api/software/jobs/${id}`);
      await loadJobs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot cancel");
    }
  }

  async function cleanTemp() {
    setBusy("clean-temp");
    try {
      const out = await api.post<{ message?: string }>("/api/software/clean-temp");
      message.success(out.message || "Temp cleaned");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Clean temp failed");
    } finally {
      setBusy("");
    }
  }

  async function cleanLog() {
    setBusy("clean-log");
    try {
      const out = await api.post<{ message?: string }>("/api/software/clean-log");
      message.success(out.message || "Logs cleaned");
      await loadJobs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Clean log failed");
    } finally {
      setBusy("");
    }
  }

  async function setCLI(name: string, version: string) {
    setBusy("cli-" + name);
    try {
      await api.post("/api/software/cli", { name, version });
      message.success(`${name} CLI → ${version}`);
      await loadPkgs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function svc(name: string, action: string) {
    try {
      await api.post("/api/software/service", { name, action });
      await loadPkgs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function openRedis() {
    setRedisBusy(true);
    try {
      const st = await api.get<{
        installed: boolean;
        bind: string;
        port: number;
        hasPassword: boolean;
        protectedMode: boolean;
        maxMemory: string;
        maxMemoryPolicy: string;
        appendOnly: boolean;
        timeout: number;
        databases: number;
      }>("/api/software/redis");
      redisForm.setFieldsValue({
        bind: st.bind || "127.0.0.1",
        port: st.port || 6379,
        password: "",
        clearPassword: false,
        protectedMode: st.protectedMode,
        maxMemory: st.maxMemory === "0" ? "" : st.maxMemory,
        maxMemoryPolicy: st.maxMemoryPolicy || "noeviction",
        appendOnly: st.appendOnly,
        timeout: st.timeout || 0,
        databases: st.databases || 16,
      });
      setRedisOpen(true);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setRedisBusy(false);
    }
  }

  async function saveRedis(values: {
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
    setRedisBusy(true);
    try {
      await api.put("/api/software/redis", {
        ...values,
        maxMemory: values.maxMemory?.trim() || "0",
      });
      message.success("Redis settings saved");
      setRedisOpen(false);
      await loadPkgs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setRedisBusy(false);
    }
  }

  const recent = jobs.filter((j) => j.status !== "queued" && j.status !== "running").slice(0, 8);
  const active = !!current || queue.length > 0;

  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Software
        </Typography.Title>
        <Typography.Paragraph type="secondary">Queue installs and updates. They run one at a time.</Typography.Paragraph>
      </div>
      <Card
        title="Install queue"
        extra={
          <Space>
            <Button size="small" loading={busy === "clean-temp"} onClick={() => void cleanTemp()}>
              Clean temp
            </Button>
            <Popconfirm
              title="Clear finished queue history and old system logs?"
              okText="Clean log"
              onConfirm={() => void cleanLog()}
            >
              <Button size="small" loading={busy === "clean-log"}>
                Clean log
              </Button>
            </Popconfirm>
          </Space>
        }
      >
        <Space direction="vertical" style={{ width: "100%" }} size="middle">
          {current ? (
            <Alert
              type="warning"
              showIcon
              message={`Now installing: ${label(current)}`}
              description={`Running ${elapsed(current.startedAt) || "Starting…"}${tick ? "" : ""}`}
            />
          ) : null}
          {current ? <Progress percent={35} status="active" showInfo={false} /> : null}
          {queue.map((j, i) => (
            <Flex key={j.id} justify="space-between" align="center">
              <Space>
                <Typography.Text type="secondary">{i + 1}.</Typography.Text>
                <Typography.Text strong>{label(j)}</Typography.Text>
                <Tag>Queued</Tag>
              </Space>
              <Button size="small" onClick={() => cancel(j.id)}>
                Cancel
              </Button>
            </Flex>
          ))}
          {!active ? <Typography.Text type="secondary">Nothing installing. Queue a package below.</Typography.Text> : null}
          {recent.map((j) => (
            <Typography.Text key={j.id} type={j.status === "error" ? "danger" : "secondary"} style={{ display: "block" }}>
              {jobLabel(j)} — {j.status}
              {j.message && j.status === "error" ? `: ${j.message}` : ""}
            </Typography.Text>
          ))}
        </Space>
      </Card>
      <Row gutter={[16, 16]}>
        {list.map((p) => {
          const versions = p.versions || [];
          const anyPending = isQueued(p.name);
          const installing = current?.name === p.name;
          const tone = anyPending ? "warning" : p.installed ? (p.active || !p.service ? "success" : "warning") : "default";
          const status = installing ? "Installing" : anyPending ? "Queued" : !p.installed ? "Missing" : p.service ? (p.active ? "Running" : "Stopped") : "Installed";
          return (
            <Col xs={24} md={12} key={p.name}>
              <Card
                title={p.title}
                extra={<Tag color={tone}>{status}</Tag>}
              >
                <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
                  {p.version || "Not installed"}
                  {p.cliVersion ? ` · CLI ${p.cliVersion}` : ""}
                </Typography.Paragraph>
                {versions.length > 0 ? (
                  <Space direction="vertical" style={{ width: "100%" }}>
                    <Dropdown
                      trigger={["click"]}
                      disabled={versions.every((v) => isQueued(p.name, v))}
                      menu={{
                        items: versions.map((v) => ({
                          key: v,
                          label: p.installedVersions?.includes(v) ? `${v} (installed)` : v,
                          disabled: isQueued(p.name, v),
                        })),
                        onClick: ({ key }) => install(p.name, key),
                      }}
                    >
                      <Button type={p.installed ? "default" : "primary"}>
                        {anyPending ? (installing ? "Installing…" : "In queue") : "Install"} <DownOutlined />
                      </Button>
                    </Dropdown>
                    {(p.installedVersions || []).length > 0 ? (
                      <Space wrap>
                        <Select
                          value={cliPick[p.name] || p.cliVersion || ""}
                          style={{ width: 140 }}
                          onChange={(v) => setCliPick((cur) => ({ ...cur, [p.name]: v }))}
                          options={p.installedVersions!.map((v) => ({ value: v, label: `${v}${p.cliVersion === v ? " (current)" : ""}` }))}
                        />
                        <Button loading={busy === "cli-" + p.name} onClick={() => setCLI(p.name, cliPick[p.name] || p.installedVersions![0])}>
                          Set CLI
                        </Button>
                      </Space>
                    ) : null}
                  </Space>
                ) : !p.installed ? (
                  <Button type="primary" disabled={anyPending} onClick={() => install(p.name)}>
                    {anyPending ? (installing ? "Installing…" : "In queue") : "Install"}
                  </Button>
                ) : null}
                {p.installed ? (
                  <Space wrap style={{ marginTop: 12 }}>
                    <Button disabled={anyPending} onClick={() => update(p.name, cliPick[p.name] || p.cliVersion || "")}>
                      Update
                    </Button>
                    {p.service ? (
                      <>
                        <Button onClick={() => svc(p.name, "start")}>Start</Button>
                        <Button onClick={() => svc(p.name, "stop")}>Stop</Button>
                        <Button onClick={() => svc(p.name, "restart")}>Restart</Button>
                      </>
                    ) : null}
                    {p.name === "redis" ? (
                      <Button loading={redisBusy} onClick={() => void openRedis()}>
                        Settings
                      </Button>
                    ) : null}
                  </Space>
                ) : null}
                {p.exclusiveOf ? (
                  <Typography.Text type="secondary" style={{ display: "block", marginTop: 8 }}>
                    Conflicts with {p.exclusiveOf}
                  </Typography.Text>
                ) : null}
              </Card>
            </Col>
          );
        })}
      </Row>
      <Modal
        title="Redis settings"
        open={redisOpen}
        onCancel={() => setRedisOpen(false)}
        onOk={() => redisForm.submit()}
        confirmLoading={redisBusy}
        destroyOnHidden
        okText="Save and restart"
      >
        <Form form={redisForm} layout="vertical" onFinish={saveRedis} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item name="bind" label="Bind" extra="127.0.0.1 keeps Redis local. Use 0.0.0.0 only with a password.">
            <Input placeholder="127.0.0.1" />
          </Form.Item>
          <Form.Item name="port" label="Port">
            <InputNumber min={1} max={65535} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item name="password" label="Password" extra="Leave blank to keep the current password.">
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
          <Form.Item name="databases" label="Databases">
            <InputNumber min={1} max={1024} style={{ width: "100%" }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
