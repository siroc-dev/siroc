import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, App, Button, Card, Flex, Input, Modal, Popconfirm, Progress, Select, Space, Table, Tag, Typography } from "antd";
import { InstallLog } from "@/components/InstallLog";
import { api } from "@/lib/api";
import { elapsed, jobLabel, type InstallJob, type InstallQueue } from "@/lib/jobs";
import { spaClick } from "@/lib/nav";

type Pkg = {
  name: string;
  title: string;
  description?: string;
  installed: boolean;
  version: string;
  service: string;
  active: boolean;
  versions?: string[];
  installedVersions?: string[];
  cliVersion?: string;
  exclusiveOf?: string;
};

type InstallKind = "install" | "update";

export function Software() {
  const nav = useNavigate();
  const { message } = App.useApp();
  const [list, setList] = useState<Pkg[]>([]);
  const [jobs, setJobs] = useState<InstallJob[]>([]);
  const [current, setCurrent] = useState<InstallJob | null>(null);
  const [queue, setQueue] = useState<InstallJob[]>([]);
  const [busy, setBusy] = useState("");
  const [tick, setTick] = useState(0);
  const [cliPick, setCliPick] = useState<Record<string, string>>({});
  const [locking, setLocking] = useState<string[]>([]);
  const [q, setQ] = useState("");
  const [modal, setModal] = useState<{ pkg: Pkg; kind: InstallKind } | null>(null);
  const [modalVer, setModalVer] = useState("");
  const [pinnedLog, setPinnedLog] = useState("");

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

  useEffect(() => {
    if (current?.name) setPinnedLog("");
  }, [current?.id]);

  function jobKey(name: string, version = "") {
    return `${name}@${version}`;
  }

  function isQueued(name: string, version = "") {
    if (locking.includes(jobKey(name, version)) || locking.includes(jobKey(name, ""))) return true;
    const items = current ? [current, ...queue] : queue;
    return items.some((j) => j.name === name && (!version || !j.version || j.version === version));
  }

  function openInstall(pkg: Pkg, kind: InstallKind) {
    const versions = pkg.versions || [];
    setModalVer(kind === "update" ? pkg.cliVersion || pkg.installedVersions?.[0] || versions[0] || "" : versions[0] || "");
    setModal({ pkg, kind });
  }

  async function queueJob(name: string, version = "") {
    const key = jobKey(name, version);
    if (isQueued(name, version)) return;
    setLocking((cur) => (cur.includes(key) ? cur : [...cur, key]));
    try {
      await api.post("/api/software/install", { name, version });
      message.success(`Queued ${name} ${version}`.trim());
      setModal(null);
      await loadJobs();
    } catch (err) {
      setLocking((cur) => cur.filter((k) => k !== key));
      message.error(err instanceof Error ? err.message : "Queue failed");
    }
  }

  async function confirmModal() {
    if (!modal) return;
    if (modal.kind === "update") {
      const ver = modalVer ? `upgrade:${modalVer}` : "upgrade";
      await queueJob(modal.pkg.name, ver);
      return;
    }
    await queueJob(modal.pkg.name, modalVer);
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

  const recent = jobs.filter((j) => j.status !== "queued" && j.status !== "running").slice(0, 8);
  const active = !!current || queue.length > 0;
  const logName = pinnedLog || current?.name || "";
  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return list;
    return list.filter((p) => `${p.title} ${p.name} ${p.description || ""}`.toLowerCase().includes(needle));
  }, [list, q]);

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
            <Popconfirm title="Clear finished queue history and old system logs?" okText="Clean log" onConfirm={() => void cleanLog()}>
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
          <div>
            <Flex justify="space-between" align="center" style={{ marginBottom: 8 }}>
              <Typography.Text strong>{logName ? `Install log · ${titles[logName] || logName}` : "Install log"}</Typography.Text>
              {current && pinnedLog && pinnedLog !== current.name ? (
                <Button size="small" type="link" onClick={() => setPinnedLog("")}>
                  Follow {titles[current.name] || current.name}
                </Button>
              ) : null}
            </Flex>
            <InstallLog name={logName} live={!!current && current.name === logName} />
          </div>
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
          {!active ? <Typography.Text type="secondary">Nothing installing. Queue a package from the table.</Typography.Text> : null}
          {recent.map((j) => (
            <Flex key={j.id} justify="space-between" align="center" gap={8}>
              <Typography.Text type={j.status === "error" ? "danger" : "secondary"} ellipsis style={{ flex: 1 }}>
                {jobLabel(j)} — {j.status}
                {j.message && j.status === "error" ? `: ${j.message.split("\n")[0].slice(0, 180)}` : ""}
              </Typography.Text>
              <Button size="small" type="link" onClick={() => setPinnedLog(j.name)}>
                Log
              </Button>
            </Flex>
          ))}
        </Space>
      </Card>
      <Card
        title="Packages"
        extra={
          <Input
            allowClear
            placeholder="Search packages"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 240 }}
          />
        }
      >
        <Table
          size="small"
          rowKey="name"
          dataSource={rows}
          pagination={{ pageSize: 15, showSizeChanger: true, showTotal: (n) => `Total ${n}` }}
          columns={[
            {
              title: "Package",
              render: (_, p) => (
                <div>
                  <Typography.Text strong>{p.title}</Typography.Text>
                  {p.description ? (
                    <div>
                      <Typography.Text type="secondary">{p.description}</Typography.Text>
                    </div>
                  ) : null}
                  {p.exclusiveOf ? <Typography.Text type="secondary">Conflicts with {p.exclusiveOf}</Typography.Text> : null}
                </div>
              ),
            },
            {
              title: "Version",
              width: 220,
              render: (_, p) => (
                <div>
                  <div>{p.version || "—"}</div>
                  {p.cliVersion ? <Typography.Text type="secondary">CLI {p.cliVersion}</Typography.Text> : null}
                  {(p.installedVersions || []).length > 1 ? (
                    <Space wrap style={{ marginTop: 8 }}>
                      <Select
                        size="small"
                        value={cliPick[p.name] || p.cliVersion || ""}
                        style={{ width: 120 }}
                        onChange={(v) => setCliPick((cur) => ({ ...cur, [p.name]: v }))}
                        options={p.installedVersions!.map((v) => ({ value: v, label: `${v}${p.cliVersion === v ? " (current)" : ""}` }))}
                      />
                      <Button size="small" loading={busy === "cli-" + p.name} onClick={() => setCLI(p.name, cliPick[p.name] || p.installedVersions![0])}>
                        Set CLI
                      </Button>
                    </Space>
                  ) : null}
                </div>
              ),
            },
            {
              title: "Status",
              width: 130,
              render: (_, p) => {
                const pending = isQueued(p.name);
                const installing = current?.name === p.name;
                const tone = pending ? "warning" : p.installed ? (p.active || !p.service ? "success" : "warning") : "default";
                const status = installing ? "Installing" : pending ? "Queued" : !p.installed ? "Missing" : p.service ? (p.active ? "Running" : "Stopped") : "Installed";
                return <Tag color={tone}>{status}</Tag>;
              },
            },
            {
              title: "Operate",
              width: 360,
              render: (_, p) => {
                const pending = isQueued(p.name);
                return (
                  <Space wrap>
                    {!p.installed ? (
                      <Button type="primary" size="small" disabled={pending} onClick={() => openInstall(p, "install")}>
                        Install
                      </Button>
                    ) : (
                      <Button size="small" disabled={pending} onClick={() => openInstall(p, "update")}>
                        Update
                      </Button>
                    )}
                    {p.installed && p.service ? (
                      <>
                        <Button size="small" onClick={() => svc(p.name, "start")}>
                          Start
                        </Button>
                        <Button size="small" onClick={() => svc(p.name, "stop")}>
                          Stop
                        </Button>
                        <Button size="small" onClick={() => svc(p.name, "restart")}>
                          Restart
                        </Button>
                      </>
                    ) : null}
                    <Button size="small" onClick={() => setPinnedLog(p.name)}>
                      Log
                    </Button>
                    {p.name === "redis" && p.installed ? (
                      <>
                        <Button size="small" href="/databases?tab=redis" onClick={spaClick("/databases?tab=redis", nav)}>
                          Status
                        </Button>
                        <Button size="small" href="/databases?tab=config" onClick={spaClick("/databases?tab=config", nav)}>
                          Config
                        </Button>
                      </>
                    ) : null}
                  </Space>
                );
              },
            },
          ]}
        />
      </Card>
      <Modal
        title={modal?.kind === "update" ? `Update ${modal.pkg.title}` : `Install ${modal?.pkg.title || ""}`}
        open={!!modal}
        onCancel={() => setModal(null)}
        onOk={() => void confirmModal()}
        okText={modal?.kind === "update" ? "Queue update" : "Queue install"}
        confirmLoading={!!modal && isQueued(modal.pkg.name, modal.kind === "update" ? (modalVer ? `upgrade:${modalVer}` : "upgrade") : modalVer)}
        destroyOnHidden
      >
        {modal ? (
          <Space direction="vertical" style={{ width: "100%", marginTop: 8 }} size="middle">
            {modal.pkg.description ? <Typography.Paragraph type="secondary">{modal.pkg.description}</Typography.Paragraph> : null}
            {modal.pkg.exclusiveOf ? <Alert type="warning" showIcon message={`Conflicts with ${modal.pkg.exclusiveOf}`} /> : null}
            {(modal.pkg.versions || []).length > 0 ? (
              <div>
                <Typography.Text type="secondary">Version</Typography.Text>
                <Select
                  style={{ width: "100%", marginTop: 8 }}
                  value={modalVer}
                  onChange={setModalVer}
                  options={(modal.pkg.versions || []).map((v) => ({
                    value: v,
                    label: modal.pkg.installedVersions?.includes(v) ? `${v} (installed)` : v,
                  }))}
                />
              </div>
            ) : (
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                This package has a single install path. Confirm to add it to the queue.
              </Typography.Paragraph>
            )}
          </Space>
        ) : null}
      </Modal>
    </div>
  );
}
