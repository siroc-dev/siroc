import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { OptionList, type CheckOption } from "@/components/OptionChecks";

type NFSOption = CheckOption;
type NFSChoice = { value: string; label: string; detail: string };
type NFSExport = { id: number; path: string; client: string; options: string[]; anonUid?: string; anonGid?: string; fsid?: string; extra?: string[] };
type NFSMount = {
  id: number;
  source: string;
  mountPoint: string;
  options: string[];
  nfsvers?: string;
  proto?: string;
  timeo?: number;
  retrans?: number;
  rsize?: number;
  wsize?: number;
  persisted: boolean;
  mounted: boolean;
};
type NFSStatus = {
  installed: boolean;
  clientInstalled: boolean;
  active: boolean;
  message?: string;
  exports?: NFSExport[];
  mounts?: NFSMount[];
  exportOptions?: NFSOption[];
  mountOptions?: NFSOption[];
  versions?: NFSChoice[];
  protos?: NFSChoice[];
};

type ExportDraft = { id: number; path: string; client: string; flags: string[]; anonUid: string; anonGid: string; fsid: string };
type MountDraft = {
  id: number;
  source: string;
  mountPoint: string;
  flags: string[];
  nfsvers: string;
  proto: string;
  timeo: number | null;
  retrans: number | null;
  rsize: number | null;
  wsize: number | null;
  persist: boolean;
  mountNow: boolean;
};

const page = { pageSize: 20, showSizeChanger: true, pageSizeOptions: ["10", "20", "50", "100"], showTotal: (n: number) => `Total ${n}` };

function exportText(row: NFSExport) {
  const parts = [...(row.options || [])];
  if (row.anonUid) parts.push(`anonuid=${row.anonUid}`);
  if (row.anonGid) parts.push(`anongid=${row.anonGid}`);
  if (row.fsid) parts.push(`fsid=${row.fsid}`);
  if (row.extra?.length) parts.push(...row.extra);
  return parts.join(", ") || "—";
}

function mountText(row: NFSMount) {
  const parts = [...(row.options || [])];
  if (row.nfsvers) parts.push(`nfsvers=${row.nfsvers}`);
  if (row.proto) parts.push(`proto=${row.proto}`);
  if (row.timeo) parts.push(`timeo=${row.timeo}`);
  if (row.retrans) parts.push(`retrans=${row.retrans}`);
  if (row.rsize) parts.push(`rsize=${row.rsize}`);
  if (row.wsize) parts.push(`wsize=${row.wsize}`);
  return parts.join(", ") || "defaults";
}

function choiceDetail(choices: NFSChoice[], value: string) {
  return choices.find((item) => item.value === value)?.detail || "";
}

export function NFSManager() {
  const { message, modal } = App.useApp();
  const [st, setSt] = useState<NFSStatus | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [share, setShare] = useState<ExportDraft | null>(null);
  const [mnt, setMnt] = useState<MountDraft | null>(null);

  async function load() {
    setSt(await api.get<NFSStatus>("/api/sysops/nfs"));
  }

  useEffect(() => {
    load()
      .catch((e) => message.error(e instanceof Error ? e.message : "Failed"))
      .finally(() => setReady(true));
  }, []);

  function take(data: NFSStatus, ok: string) {
    setSt(data);
    if (data.message) message.warning(data.message);
    else message.success(ok);
  }

  async function run(work: () => Promise<NFSStatus>, ok: string) {
    setBusy(true);
    try {
      take(await work(), ok);
      return true;
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function saveShare() {
    if (!share) return;
    const body = { path: share.path.trim(), client: share.client.trim(), options: share.flags, anonUid: share.anonUid.trim(), anonGid: share.anonGid.trim(), fsid: share.fsid.trim() };
    const ok = await run(
      () => (share.id > 0 ? api.put<NFSStatus>(`/api/sysops/nfs/exports/${share.id}`, body) : api.post<NFSStatus>("/api/sysops/nfs/exports", body)),
      share.id > 0 ? "Share updated" : "Share added",
    );
    if (ok) setShare(null);
  }

  async function saveMount() {
    if (!mnt) return;
    const body = {
      source: mnt.source.trim(),
      mountPoint: mnt.mountPoint.trim(),
      options: mnt.flags,
      nfsvers: mnt.nfsvers,
      proto: mnt.proto,
      timeo: mnt.timeo || 0,
      retrans: mnt.retrans || 0,
      rsize: mnt.rsize || 0,
      wsize: mnt.wsize || 0,
      persist: mnt.persist,
      mountNow: mnt.mountNow,
    };
    const ok = await run(
      () => (mnt.id > 0 ? api.put<NFSStatus>(`/api/sysops/nfs/mounts/${mnt.id}`, body) : api.post<NFSStatus>("/api/sysops/nfs/mounts", body)),
      mnt.id > 0 ? "Mount updated" : "Mount saved",
    );
    if (ok) setMnt(null);
  }

  function confirmRemove(title: string, content: string, work: () => Promise<NFSStatus>, ok: string) {
    modal.confirm({
      title,
      content,
      okText: "Delete",
      okButtonProps: { danger: true },
      onOk: () => run(work, ok),
    });
  }

  const versions = st?.versions || [];
  const protos = st?.protos || [];

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card
        title="NFS server"
        extra={
          <Space>
            {st?.installed ? <Tag color={st.active ? "success" : "default"}>{st.active ? "Running" : "Stopped"}</Tag> : <Tag>Not installed</Tag>}
            {st?.installed ? (
              <>
                <Button size="small" disabled={busy || st.active} onClick={() => void run(() => api.post<NFSStatus>("/api/sysops/nfs/service", { action: "start" }), "NFS started")}>
                  Start
                </Button>
                <Button size="small" disabled={busy || !st.active} onClick={() => void run(() => api.post<NFSStatus>("/api/sysops/nfs/service", { action: "restart" }), "NFS restarted")}>
                  Restart
                </Button>
                <Button
                  size="small"
                  danger
                  disabled={busy || !st.active}
                  onClick={() =>
                    modal.confirm({
                      title: "Stop NFS?",
                      content: "Clients lose this server until it starts again. Saved shares stay in /etc/exports.",
                      okText: "Stop",
                      okButtonProps: { danger: true },
                      onOk: () => run(() => api.post<NFSStatus>("/api/sysops/nfs/service", { action: "stop" }), "NFS stopped"),
                    })
                  }
                >
                  Stop
                </Button>
              </>
            ) : (
              <Button type="primary" loading={busy} disabled={!ready} onClick={() => void run(() => api.post<NFSStatus>("/api/sysops/nfs/install"), "NFS installed")}>
                Install NFS
              </Button>
            )}
          </Space>
        }
      >
        {!st?.installed && ready ? <Alert type="info" showIcon style={{ marginBottom: 12 }} message="Install NFS to publish shares and mount remote exports. The packages are nfs-kernel-server and nfs-common." /> : null}
        <Typography.Paragraph type="secondary">Each row is one client of a directory in /etc/exports. Saving publishes it with exportfs.</Typography.Paragraph>
        <Button
          type="primary"
          style={{ marginBottom: 12 }}
          disabled={!ready || busy}
          onClick={() => setShare({ id: 0, path: "", client: "", flags: ["rw", "sync", "root_squash", "no_subtree_check", "secure"], anonUid: "", anonGid: "", fsid: "" })}
        >
          Add share
        </Button>
        <Table
          size="small"
          rowKey="id"
          loading={!ready}
          dataSource={st?.exports || []}
          pagination={page}
          locale={{ emptyText: "No NFS shares" }}
          columns={[
            { title: "Directory", dataIndex: "path" },
            { title: "Client", dataIndex: "client", width: 180 },
            { title: "Options", render: (_, row) => exportText(row) },
            {
              title: "",
              width: 150,
              render: (_, row) => (
                <Space>
                  <Button size="small" disabled={busy} onClick={() => setShare({ id: row.id, path: row.path, client: row.client, flags: row.options || [], anonUid: row.anonUid || "", anonGid: row.anonGid || "", fsid: row.fsid || "" })}>
                    Edit
                  </Button>
                  <Button size="small" danger disabled={busy} onClick={() => confirmRemove(`Delete ${row.client}?`, `${row.path} will no longer be exported to this client.`, () => api.delete<NFSStatus>(`/api/sysops/nfs/exports/${row.id}`), "Share deleted")}>
                    Delete
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Card title="NFS mounts">
        <Typography.Paragraph type="secondary">Mount a remote export on this server. Options below are written to the mount command and, if you keep it, to /etc/fstab.</Typography.Paragraph>
        {!st?.clientInstalled && ready ? <Alert type="info" showIcon style={{ marginBottom: 12 }} message="The NFS client is not installed yet. You can still save an fstab line, and Mount now will work after Install NFS." /> : null}
        <Button
          type="primary"
          style={{ marginBottom: 12 }}
          disabled={!ready || busy}
          onClick={() => setMnt({ id: 0, source: "", mountPoint: "", flags: ["hard", "_netdev"], nfsvers: "4.2", proto: "tcp", timeo: null, retrans: null, rsize: null, wsize: null, persist: true, mountNow: true })}
        >
          Add mount
        </Button>
        <Table
          size="small"
          rowKey="id"
          loading={!ready}
          dataSource={st?.mounts || []}
          pagination={page}
          locale={{ emptyText: "No NFS mounts" }}
          columns={[
            { title: "Source", dataIndex: "source" },
            { title: "Mount point", dataIndex: "mountPoint" },
            { title: "Options", render: (_, row) => mountText(row) },
            {
              title: "State",
              width: 150,
              render: (_, row) => (
                <Space size={4} wrap>
                  <Tag color={row.mounted ? "success" : "default"}>{row.mounted ? "Mounted" : "Not mounted"}</Tag>
                  {row.persisted ? <Tag>fstab</Tag> : null}
                </Space>
              ),
            },
            {
              title: "",
              width: 250,
              render: (_, row) => (
                <Space wrap>
                  <Button
                    size="small"
                    disabled={busy}
                    onClick={() =>
                      setMnt({
                        id: row.id,
                        source: row.source,
                        mountPoint: row.mountPoint,
                        flags: row.options || [],
                        nfsvers: row.nfsvers || "",
                        proto: row.proto || "",
                        timeo: row.timeo || null,
                        retrans: row.retrans || null,
                        rsize: row.rsize || null,
                        wsize: row.wsize || null,
                        persist: row.persisted,
                        mountNow: row.mounted,
                      })
                    }
                  >
                    Edit
                  </Button>
                  {row.mounted ? (
                    <Button size="small" disabled={busy} onClick={() => void run(() => api.post<NFSStatus>(`/api/sysops/nfs/mounts/${row.id}/unmount`), "Unmounted")}>
                      Unmount
                    </Button>
                  ) : (
                    <Button size="small" disabled={busy} onClick={() => void run(() => api.post<NFSStatus>(`/api/sysops/nfs/mounts/${row.id}/mount`), "Mounted")}>
                      Mount
                    </Button>
                  )}
                  <Button size="small" danger disabled={busy} onClick={() => confirmRemove(`Delete ${row.mountPoint}?`, "This unmounts the share and removes its fstab line.", () => api.delete<NFSStatus>(`/api/sysops/nfs/mounts/${row.id}`), "Mount deleted")}>
                    Delete
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Modal title={share && share.id > 0 ? "Edit share" : "Add share"} open={!!share} onCancel={() => !busy && setShare(null)} onOk={() => void saveShare()} confirmLoading={busy} okText="Save" width={720} destroyOnHidden>
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <div>
            <Typography.Text>Directory</Typography.Text>
            <Input value={share?.path || ""} placeholder="/srv/share" onChange={(e) => setShare((d) => (d ? { ...d, path: e.target.value } : d))} />
            <span className="nfs-option-detail">Absolute directory on this server. System paths such as /etc are refused.</span>
          </div>
          <div>
            <Typography.Text>Client</Typography.Text>
            <Input value={share?.client || ""} placeholder="10.0.0.0/24" onChange={(e) => setShare((d) => (d ? { ...d, client: e.target.value } : d))} />
            <span className="nfs-option-detail">Who may mount it: an address, a network, a hostname, or * for everyone.</span>
          </div>
          <OptionList options={st?.exportOptions || []} value={share?.flags || []} onChange={(flags) => setShare((d) => (d ? { ...d, flags } : d))} />
          <div>
            <Typography.Text>Squash user id</Typography.Text>
            <Input value={share?.anonUid || ""} placeholder="65534" onChange={(e) => setShare((d) => (d ? { ...d, anonUid: e.target.value } : d))} />
            <span className="nfs-option-detail">Numeric user id used when a client user is squashed. Empty uses nobody.</span>
          </div>
          <div>
            <Typography.Text>Squash group id</Typography.Text>
            <Input value={share?.anonGid || ""} placeholder="65534" onChange={(e) => setShare((d) => (d ? { ...d, anonGid: e.target.value } : d))} />
            <span className="nfs-option-detail">Numeric group id used when a client user is squashed. Empty uses nobody.</span>
          </div>
          <div>
            <Typography.Text>FSID</Typography.Text>
            <Input value={share?.fsid || ""} placeholder="0" onChange={(e) => setShare((d) => (d ? { ...d, fsid: e.target.value } : d))} />
            <span className="nfs-option-detail">NFSv4 file system id. Use 0 or root for the pseudo root. Leave empty to omit it.</span>
          </div>
        </Space>
      </Modal>

      <Modal title={mnt && mnt.id > 0 ? "Edit mount" : "Add mount"} open={!!mnt} onCancel={() => !busy && setMnt(null)} onOk={() => void saveMount()} confirmLoading={busy} okText="Save" width={720} destroyOnHidden>
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <div>
            <Typography.Text>Source</Typography.Text>
            <Input value={mnt?.source || ""} placeholder="10.0.0.5:/srv/share" onChange={(e) => setMnt((d) => (d ? { ...d, source: e.target.value } : d))} />
            <span className="nfs-option-detail">Remote server and export, written as server:/path.</span>
          </div>
          <div>
            <Typography.Text>Mount point</Typography.Text>
            <Input value={mnt?.mountPoint || ""} placeholder="/mnt/share" onChange={(e) => setMnt((d) => (d ? { ...d, mountPoint: e.target.value } : d))} />
            <span className="nfs-option-detail">Local directory. It is created if it does not exist.</span>
          </div>
          <div>
            <Typography.Text>NFS version</Typography.Text>
            <Select style={{ width: "100%" }} value={mnt?.nfsvers ?? ""} options={versions.map((item) => ({ value: item.value, label: item.label }))} onChange={(value) => setMnt((d) => (d ? { ...d, nfsvers: value } : d))} />
            <span className="nfs-option-detail">{choiceDetail(versions, mnt?.nfsvers || "")}</span>
          </div>
          <div>
            <Typography.Text>Protocol</Typography.Text>
            <Select style={{ width: "100%" }} value={mnt?.proto ?? ""} options={protos.map((item) => ({ value: item.value, label: item.label }))} onChange={(value) => setMnt((d) => (d ? { ...d, proto: value } : d))} />
            <span className="nfs-option-detail">{choiceDetail(protos, mnt?.proto || "")}</span>
          </div>
          <OptionList options={st?.mountOptions || []} value={mnt?.flags || []} onChange={(flags) => setMnt((d) => (d ? { ...d, flags } : d))} />
          <Space wrap>
            <NumberField label="Timeout" detail="Tenths of a second to wait before a retry. 600 is 60 seconds. Empty uses the client default." value={mnt?.timeo ?? null} min={1} max={6000} onChange={(timeo) => setMnt((d) => (d ? { ...d, timeo } : d))} />
            <NumberField label="Retries" detail="How many times to retry before a soft mount fails, or a hard mount reports that the server is not responding." value={mnt?.retrans ?? null} min={1} max={100} onChange={(retrans) => setMnt((d) => (d ? { ...d, retrans } : d))} />
            <NumberField label="Read size" detail="Maximum bytes in one read. Empty uses the client default." value={mnt?.rsize ?? null} min={1024} max={1048576} onChange={(rsize) => setMnt((d) => (d ? { ...d, rsize } : d))} />
            <NumberField label="Write size" detail="Maximum bytes in one write. Empty uses the client default." value={mnt?.wsize ?? null} min={1024} max={1048576} onChange={(wsize) => setMnt((d) => (d ? { ...d, wsize } : d))} />
          </Space>
          <div>
            <Switch checked={!!mnt?.mountNow} onChange={(mountNow) => setMnt((d) => (d ? { ...d, mountNow } : d))} /> <Typography.Text>Mount now</Typography.Text>
            <span className="nfs-option-detail">Attach the share immediately.</span>
          </div>
          <div>
            <Switch checked={!!mnt?.persist} onChange={(persist) => setMnt((d) => (d ? { ...d, persist } : d))} /> <Typography.Text>Keep after reboot</Typography.Text>
            <span className="nfs-option-detail">Write a line in /etc/fstab so the share mounts at boot.</span>
          </div>
        </Space>
      </Modal>
    </Space>
  );
}

function NumberField({ label, detail, value, min, max, onChange }: { label: string; detail: string; value: number | null; min: number; max: number; onChange: (value: number | null) => void }) {
  return (
    <div style={{ width: 220 }}>
      <Typography.Text>{label}</Typography.Text>
      <InputNumber style={{ width: "100%" }} min={min} max={max} value={value ?? undefined} placeholder="Default" onChange={(next) => onChange(typeof next === "number" ? next : null)} />
      <span className="nfs-option-detail">{detail}</span>
    </div>
  );
}
