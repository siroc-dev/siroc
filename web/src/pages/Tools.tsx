import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, InputNumber, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { PanelUpdate } from "@/components/PanelUpdate";

type Status = {
  hostname: string;
  time: string;
  timezone: string;
  ntp: boolean;
  timezones?: string[];
  dns?: string[];
  search?: string;
  swap?: { totalMB: number; usedMB: number; file?: string };
  ips?: { iface: string; address: string; family: string }[];
  routes?: string[];
  ifaces?: string[];
  mounts?: { device: string; mountPoint: string; fsType: string; size?: string; used?: string; avail?: string; usePct?: string }[];
  fstab?: { device: string; mountPoint: string; fsType: string; options: string }[];
  fail2ban?: { installed: boolean; active: boolean; jails?: { name: string; banned?: string[]; failed?: number }[]; message?: string };
  threats?: { failedLogins?: { ip: string; detail?: string }[]; banned?: { ip: string; source?: string }[]; topSources?: { ip: string; count: number }[] };
  ffmpeg?: { installed: boolean; version?: string; codecs?: string[]; output?: string };
  memcached?: { installed: boolean; active: boolean; memoryMB: number; listen: string; port: number; stats?: Record<string, string> };
  quota?: boolean;
  message?: string;
};

type Disk = {
  filesystems?: Status["mounts"];
  trees?: { path: string; size: number }[];
  largest?: { path: string; size: number }[];
  message?: string;
};

function timezoneOptions(st: Status | null) {
  const extra = ["UTC", "Etc/UTC", "Asia/Bangkok", "Asia/Jakarta", "Asia/Ho_Chi_Minh", "Asia/Singapore"];
  const list = [...(st?.timezones || []), st?.timezone || "", ...extra].filter(Boolean) as string[];
  return [...new Set(list)].map((z) => ({ value: z, label: z }));
}

export function Tools() {
  const { message } = App.useApp();
  const [st, setSt] = useState<Status | null>(null);
  const [disk, setDisk] = useState<Disk | null>(null);
  const [busy, setBusy] = useState(false);
  const [dnsForm] = Form.useForm();
  const [tzForm] = Form.useForm();
  const [swapForm] = Form.useForm();
  const [ipForm] = Form.useForm();
  const [mntForm] = Form.useForm();
  const [ffForm] = Form.useForm();
  const [mcForm] = Form.useForm();

  async function load() {
    const data = await api.get<Status>("/api/sysops");
    setSt(data);
    dnsForm.setFieldsValue({ nameservers: (data.dns || []).join("\n"), searchDomain: data.search || "" });
    tzForm.setFieldsValue({ timezone: data.timezone, ntp: data.ntp });
    swapForm.setFieldsValue({ swapMB: data.swap?.totalMB || 0 });
    mcForm.setFieldsValue({ memoryMB: data.memcached?.memoryMB || 64, listen: data.memcached?.listen, port: data.memcached?.port });
  }
  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  async function apply(body: Record<string, unknown>) {
    setBusy(true);
    try {
      const data = await api.post<Status>("/api/sysops", body);
      setSt(data);
      message.success("Applied");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function loadDisk() {
    try {
      setDisk(await api.get<Disk>("/api/sysops/disk"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  return (
    <div className="cp-page">
      <Typography.Title level={3} style={{ margin: 0 }}>
        System tools
      </Typography.Title>
      {st?.message ? <Alert type="warning" showIcon message={st.message} /> : null}
      <Tabs
        items={[
          {
            key: "updates",
            label: "Updates",
            children: <PanelUpdate />,
          },
          {
            key: "os",
            label: "OS",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card>
                  <Typography.Paragraph>
                    Host <Typography.Text code>{st?.hostname}</Typography.Text> · {st?.time}
                  </Typography.Paragraph>
                  <Form form={tzForm} layout="inline" onFinish={(v) => apply({ action: "timezone", ...v })}>
                    <Form.Item name="timezone" label="Timezone">
                      <Select
                        showSearch
                        optionFilterProp="label"
                        placeholder="Search timezone"
                        style={{ minWidth: 280 }}
                        options={timezoneOptions(st)}
                      />
                    </Form.Item>
                    <Form.Item name="ntp" label="NTP" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" loading={busy}>
                      Save
                    </Button>
                  </Form>
                </Card>
                <Card title="DNS">
                  <Form form={dnsForm} layout="vertical" onFinish={(v) => apply({ action: "dns", nameservers: String(v.nameservers || "").split(/\s+/).filter(Boolean), searchDomain: v.searchDomain })}>
                    <Form.Item name="nameservers" label="Nameservers" extra="One IP per line">
                      <Input.TextArea rows={4} />
                    </Form.Item>
                    <Form.Item name="searchDomain" label="Search domain">
                      <Input />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" loading={busy}>
                      Write resolv.conf
                    </Button>
                  </Form>
                </Card>
                <Card title="Swap">
                  <Typography.Paragraph type="secondary">
                    {st?.swap ? `${st.swap.usedMB} / ${st.swap.totalMB} MB ${st.swap.file || ""}` : "No swap"}
                  </Typography.Paragraph>
                  <Form form={swapForm} layout="inline" onFinish={(v) => apply({ action: "swap", swapMB: v.swapMB })}>
                    <Form.Item name="swapMB" label="Size (MB)">
                      <InputNumber min={0} max={32768} />
                    </Form.Item>
                    <Button htmlType="submit" loading={busy}>
                      Apply /swapfile
                    </Button>
                  </Form>
                </Card>
                <Card title="IP addresses">
                  <Table
                    size="small"
                    rowKey={(r) => r.iface + r.address}
                    pagination={false}
                    dataSource={st?.ips || []}
                    columns={[
                      { title: "Interface", dataIndex: "iface", width: 120 },
                      { title: "Address", dataIndex: "address" },
                      {
                        title: "",
                        width: 90,
                        render: (_, row) =>
                          row.iface !== "lo" ? (
                            <Button size="small" danger onClick={() => apply({ action: "ip-del", interface: row.iface, address: row.address })}>
                              Remove
                            </Button>
                          ) : null,
                      },
                    ]}
                  />
                  <Form form={ipForm} layout="inline" style={{ marginTop: 12 }} onFinish={(v) => apply({ action: "ip-add", ...v })}>
                    <Form.Item name="interface" rules={[{ required: true }]}>
                      <Select placeholder="iface" style={{ width: 140 }} options={(st?.ifaces || []).map((i) => ({ value: i, label: i }))} />
                    </Form.Item>
                    <Form.Item name="address" rules={[{ required: true }]}>
                      <Input placeholder="10.0.0.5/24" style={{ width: 200 }} />
                    </Form.Item>
                    <Button htmlType="submit" loading={busy}>
                      Add
                    </Button>
                  </Form>
                  <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                    {(st?.routes || []).join(" · ") || "No routes"}
                  </Typography.Paragraph>
                </Card>
              </Space>
            ),
          },
          {
            key: "disk",
            label: "Disk",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card title="Mounts">
                  <Table
                    size="small"
                    rowKey={(r) => r.mountPoint}
                    pagination={false}
                    dataSource={st?.mounts || []}
                    columns={[
                      { title: "Device", dataIndex: "device" },
                      { title: "Mount", dataIndex: "mountPoint" },
                      { title: "Type", dataIndex: "fsType", width: 100 },
                      { title: "Size", dataIndex: "size", width: 90 },
                      { title: "Used", dataIndex: "usePct", width: 80 },
                      {
                        title: "",
                        width: 90,
                        render: (_, m) =>
                          m.mountPoint !== "/" ? (
                            <Button size="small" onClick={() => apply({ action: "umount", mountPoint: m.mountPoint })}>
                              Unmount
                            </Button>
                          ) : null,
                      },
                    ]}
                  />
                  <Form form={mntForm} layout="inline" style={{ marginTop: 12 }} onFinish={(v) => apply({ action: "mount", persist: true, ...v })}>
                    <Form.Item name="device" rules={[{ required: true }]}>
                      <Input placeholder="/dev/sdb1" />
                    </Form.Item>
                    <Form.Item name="mountPoint" rules={[{ required: true }]}>
                      <Input placeholder="/mnt/data" />
                    </Form.Item>
                    <Form.Item name="fsType">
                      <Input placeholder="ext4" style={{ width: 100 }} />
                    </Form.Item>
                    <Button htmlType="submit" loading={busy}>
                      Mount + fstab
                    </Button>
                  </Form>
                </Card>
                <Card
                  title="Analysis"
                  extra={
                    <Button onClick={() => void loadDisk()}>Scan</Button>
                  }
                >
                  {disk?.message ? <Alert type="warning" message={disk.message} style={{ marginBottom: 12 }} /> : null}
                  <Typography.Title level={5}>Largest directories</Typography.Title>
                  <Table
                    size="small"
                    rowKey="path"
                    pagination={false}
                    dataSource={disk?.trees || []}
                    columns={[
                      { title: "Path", dataIndex: "path" },
                      { title: "Size", render: (_, r) => formatBytes(r.size), width: 120 },
                    ]}
                  />
                  <Typography.Title level={5}>Largest files</Typography.Title>
                  <Table
                    size="small"
                    rowKey="path"
                    pagination={false}
                    dataSource={disk?.largest || []}
                    columns={[
                      { title: "Path", dataIndex: "path" },
                      { title: "Size", render: (_, r) => formatBytes(r.size), width: 120 },
                    ]}
                  />
                </Card>
              </Space>
            ),
          },
          {
            key: "sec",
            label: "Threats",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card title="Fail2ban">
                  {!st?.fail2ban?.installed ? (
                    <Alert type="info" showIcon message="Install Fail2ban from Software first." />
                  ) : (
                    <Table
                      size="small"
                      rowKey="name"
                      pagination={false}
                      dataSource={st.fail2ban.jails || []}
                      columns={[
                        { title: "Jail", dataIndex: "name" },
                        { title: "Failed", dataIndex: "failed", width: 90 },
                        {
                          title: "Banned",
                          render: (_, j) =>
                            (j.banned || []).length ? (
                              <Space wrap>
                                {(j.banned || []).map((ip) => (
                                  <Tag key={ip} closable onClose={() => apply({ action: "unban", jail: j.name, ip })}>
                                    {ip}
                                  </Tag>
                                ))}
                              </Space>
                            ) : (
                              "—"
                            ),
                        },
                      ]}
                    />
                  )}
                </Card>
                <Card title="Network threat detection">
                  <Typography.Title level={5}>Failed logins</Typography.Title>
                  <Table
                    size="small"
                    pagination={{ pageSize: 8 }}
                    dataSource={(st?.threats?.failedLogins || []).map((e, i) => ({ ...e, key: i }))}
                    columns={[
                      { title: "IP", dataIndex: "ip", width: 160 },
                      { title: "Detail", dataIndex: "detail", ellipsis: true },
                    ]}
                  />
                  <Typography.Title level={5}>Top sources</Typography.Title>
                  <Table
                    size="small"
                    pagination={false}
                    dataSource={st?.threats?.topSources || []}
                    rowKey="ip"
                    columns={[
                      { title: "IP", dataIndex: "ip" },
                      { title: "Count", dataIndex: "count", width: 90 },
                    ]}
                  />
                </Card>
              </Space>
            ),
          },
          {
            key: "svc",
            label: "Services",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card title="Memcached">
                  {!st?.memcached?.installed ? (
                    <Alert type="info" showIcon message="Install Memcached from Software." />
                  ) : (
                    <>
                      <Tag color={st.memcached.active ? "success" : "default"}>{st.memcached.active ? "Running" : "Stopped"}</Tag>
                      <Form form={mcForm} layout="inline" style={{ marginTop: 12 }} onFinish={(v) => apply({ action: "memcached", ...v })}>
                        <Form.Item name="memoryMB" label="RAM MB">
                          <InputNumber min={16} max={8192} />
                        </Form.Item>
                        <Form.Item name="listen" label="Listen">
                          <Input style={{ width: 140 }} />
                        </Form.Item>
                        <Form.Item name="port" label="Port">
                          <InputNumber min={1} max={65535} />
                        </Form.Item>
                        <Button htmlType="submit" loading={busy}>
                          Save
                        </Button>
                      </Form>
                      {st.memcached.stats ? (
                        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                          items {st.memcached.stats.curr_items || 0} · hits {st.memcached.stats.get_hits || 0} · bytes {st.memcached.stats.bytes || 0}
                        </Typography.Paragraph>
                      ) : null}
                    </>
                  )}
                </Card>
                <Card title="FFmpeg">
                  {!st?.ffmpeg?.installed ? (
                    <Alert type="info" showIcon message="Install FFmpeg from Software." />
                  ) : (
                    <>
                      <Typography.Paragraph type="secondary">{st.ffmpeg.version}</Typography.Paragraph>
                      <Form form={ffForm} layout="vertical" onFinish={(v) => apply({ action: "ffmpeg", ...v })}>
                        <Form.Item name="src" label="Source file" rules={[{ required: true }]}>
                          <Input placeholder="/home/user/video.mp4" />
                        </Form.Item>
                        <Form.Item name="dest" label="Destination" rules={[{ required: true }]}>
                          <Input placeholder="/home/user/out.mp4" />
                        </Form.Item>
                        <Form.Item name="extra" label="Extra args">
                          <Input placeholder="-c:v libx264 -crf 23" />
                        </Form.Item>
                        <Button type="primary" htmlType="submit" loading={busy}>
                          Convert
                        </Button>
                      </Form>
                      {st.ffmpeg.output ? (
                        <pre style={{ marginTop: 12, maxHeight: 240, overflow: "auto" }}>{st.ffmpeg.output}</pre>
                      ) : null}
                    </>
                  )}
                </Card>
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
