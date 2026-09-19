import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { useOutletContext } from "react-router-dom";
import {
  App,
  Button,
  Card,
  Checkbox,
  Dropdown,
  Input,
  Modal,
  Progress,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
  Upload,
} from "antd";
import type { MenuProps } from "antd";
import {
  CloudDownloadOutlined,
  CopyOutlined,
  DeleteOutlined,
  DownloadOutlined,
  FileOutlined,
  FileZipOutlined,
  FolderAddOutlined,
  FolderOutlined,
  InboxOutlined,
  LaptopOutlined,
  LockOutlined,
  MoreOutlined,
  SearchOutlined,
  ScissorOutlined,
  SnippetsOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import { api } from "@/lib/api";
import { languageFromPath } from "@/lib/fileLang";
import { editorWorkspace, formatBytes } from "@/lib/usage";
import { SSHTerminal } from "@/components/SSHTerminal";

const MonacoFileEditor = lazy(() => import("@/components/MonacoFileEditor").then((m) => ({ default: m.MonacoFileEditor })));

type Account = { username: string };
type Entry = { name: string; path: string; isDir: boolean; size: number; mode: string; modTime: string };
type Listing = { path: string; absPath?: string; root?: boolean; entries: Entry[] };
type Clip = { path: string; name: string; isDir: boolean; op: "copy" | "move" };

const rootShortcuts = ["/", "/etc", "/home", "/opt/siroc", "/var/log", "/etc/nginx", "/etc/php"];

function isArchive(name: string) {
  const n = name.toLowerCase();
  return [".zip", ".rar", ".7z", ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".gz", ".bz2", ".xz"].some((ext) => n.endsWith(ext));
}

function joinPath(dir: string, name: string) {
  const base = dir.replace(/\/+$/, "");
  const leaf = name.replace(/^\/+/, "");
  if (!base || base === "/") return "/" + leaf;
  return `${base}/${leaf}`;
}

function parseMode(mode: string) {
  const s = (mode || "644").replace(/[^0-7]/g, "").slice(-3).padStart(3, "0");
  const n = parseInt(s, 8) || 0;
  return { u: (n >> 6) & 7, g: (n >> 3) & 7, o: n & 7, raw: s };
}

function bits(v: number, flag: number) {
  return (v & flag) === flag;
}

export function Files() {
  const { message, modal } = App.useApp();
  const { admin } = useOutletContext<{ user: string; admin?: boolean }>();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [user, setUser] = useState("");
  const [listing, setListing] = useState<Listing>({ path: "/", entries: [] });
  const [editPath, setEditPath] = useState("");
  const [absPath, setAbsPath] = useState("");
  const [content, setContent] = useState("");
  const [mkdir, setMkdir] = useState("");
  const [busy, setBusy] = useState(false);
  const [editorReady, setEditorReady] = useState(false);
  const [lspOn, setLspOn] = useState(false);
  const [upPct, setUpPct] = useState(0);
  const [upLabel, setUpLabel] = useState("");
  const [upOpen, setUpOpen] = useState(false);
  const [clip, setClip] = useState<Clip | null>(null);
  const [perm, setPerm] = useState<Entry | null>(null);
  const [modeBits, setModeBits] = useState({ u: 6, g: 4, o: 4 });
  const [moveItem, setMoveItem] = useState<Entry | null>(null);
  const [moveDest, setMoveDest] = useState("");
  const [fetchOpen, setFetchOpen] = useState(false);
  const [fetchUrl, setFetchUrl] = useState("");
  const [fetchName, setFetchName] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQ, setSearchQ] = useState("");
  const [searchHits, setSearchHits] = useState<{ path: string; line: number; text: string }[]>([]);
  const [termOpen, setTermOpen] = useState(false);
  const [termCwd, setTermCwd] = useState("");

  const root = user === "root";

  async function load(path = listing.path) {
    if (!user) return;
    const data = await api.get<Listing>(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`);
    setListing(data);
  }

  useEffect(() => {
    api.get<Account[]>("/api/accounts").then((a) => {
      setAccounts(a);
      if (a[0]) setUser(a[0].username);
      else if (admin) setUser("root");
    });
  }, [admin]);

  useEffect(() => {
    if (user) load("/").catch((e) => message.error(e.message));
  }, [user]);

  function parent() {
    const parts = listing.path.split("/").filter(Boolean);
    parts.pop();
    return "/" + parts.join("/");
  }

  async function open(e: Entry) {
    if (e.isDir) {
      await load(e.path);
      return;
    }
    if (isArchive(e.name)) {
      confirmExtract(e);
      return;
    }
    try {
      const data = await api.get<{ content: string; absPath?: string }>(
        `/api/files/content?user=${encodeURIComponent(user)}&path=${encodeURIComponent(e.path)}`,
      );
      setEditPath(e.path);
      setAbsPath(data.absPath || "");
      setContent(data.content);
      setLspOn(false);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot open");
    }
  }

  async function save() {
    if (!editPath) return;
    setBusy(true);
    try {
      await api.put("/api/files/content", { user, path: editPath, content });
      message.success("Saved");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Save failed");
    } finally {
      setBusy(false);
    }
  }

  async function makeDir() {
    const path = `${listing.path.replace(/\/$/, "")}/${mkdir}`;
    try {
      await api.post("/api/files/mkdir", { user, path });
      setMkdir("");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function remove(path: string) {
    try {
      await api.delete(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`);
      if (editPath === path) {
        setEditPath("");
        setContent("");
        setAbsPath("");
        setEditorReady(false);
      }
      if (clip?.path === path) setClip(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  function skipFile(name: string) {
    const n = name.toLowerCase();
    return n === ".ds_store" || n === "thumbs.db" || n === "desktop.ini";
  }

  function relOf(f: File) {
    const rel = (f as File & { webkitRelativePath?: string }).webkitRelativePath || "";
    return rel.replace(/^\/+/, "");
  }

  async function uploadMany(files: File[]) {
    const list = files.filter((f) => f && !skipFile(f.name));
    if (!list.length || !user) return;
    setBusy(true);
    setUpPct(0);
    setUpLabel(`Uploading 0/${list.length}`);
    let ok = 0;
    try {
      for (let i = 0; i < list.length; i++) {
        const f = list[i];
        const fd = new FormData();
        fd.append("user", user);
        fd.append("path", listing.path);
        const rel = relOf(f);
        if (rel) fd.append("relpath", rel);
        fd.append("file", f, f.name);
        const res = await fetch("/api/files/upload", { method: "POST", body: fd, credentials: "include" });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error((data as { error?: string }).error || `Upload failed: ${f.name}`);
        ok++;
        setUpPct(Math.round(((i + 1) / list.length) * 100));
        setUpLabel(`Uploading ${i + 1}/${list.length}`);
      }
      message.success(ok === 1 ? "Uploaded" : `Uploaded ${ok} files`);
      setUpOpen(false);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Upload failed");
      if (ok) await load();
    } finally {
      setBusy(false);
      setUpPct(0);
      setUpLabel("");
    }
  }

  async function filesFromDrop(dt: DataTransfer) {
    const out: File[] = [];
    const items = [...dt.items];
    const walked = items.some((it) => typeof it.webkitGetAsEntry === "function" && it.webkitGetAsEntry());
    if (walked) {
      for (const it of items) {
        const entry = it.webkitGetAsEntry?.();
        if (entry) await walkEntry(entry, out);
      }
    } else {
      out.push(...[...dt.files]);
    }
    return out;
  }

  async function walkEntry(entry: FileSystemEntry, out: File[]) {
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) => (entry as FileSystemFileEntry).file(resolve, reject));
      const rel = entry.fullPath.replace(/^\/+/, "");
      if (rel && !file.webkitRelativePath) {
        try {
          Object.defineProperty(file, "webkitRelativePath", { value: rel });
        } catch {
          (file as File & { webkitRelativePath?: string }).webkitRelativePath = rel;
        }
      }
      out.push(file);
      return;
    }
    if (!entry.isDirectory) return;
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    const readBatch = () => new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
    let batch = await readBatch();
    while (batch.length) {
      for (const child of batch) await walkEntry(child, out);
      batch = await readBatch();
    }
  }

  async function download(e: Entry) {
    try {
      const q = new URLSearchParams({ user, path: e.path });
      const res = await fetch(`/api/files/download?${q}`, { credentials: "include" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || "Download failed");
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      const cd = res.headers.get("Content-Disposition") || "";
      const m = /filename="?([^"]+)"?/.exec(cd);
      a.href = url;
      a.download = m?.[1] || (e.isDir ? `${e.name}.tar.gz` : e.name);
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Download failed");
    }
  }

  function confirmExtract(e: Entry) {
    modal.confirm({
      title: `Extract ${e.name}?`,
      content: e.size ? `${formatBytes(e.size)} archive. Unpack into a folder next to this file.` : "Unpack into a folder next to this file.",
      okText: "Extract",
      onOk: () => extract(e),
    });
  }

  async function extract(e: Entry) {
    setBusy(true);
    try {
      const out = await api.post<{ dest?: string }>("/api/files/extract", { user, path: e.path });
      message.success("Extracted");
      if (out.dest) await load(out.dest);
      else await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Extract failed");
      throw err;
    } finally {
      setBusy(false);
    }
  }

  async function remoteFetch() {
    if (!fetchUrl.trim()) return;
    setBusy(true);
    try {
      const out = await api.post<{ dest?: string }>("/api/files/fetch", { user, path: listing.path, url: fetchUrl.trim(), dest: fetchName.trim() });
      message.success("Downloaded " + (out.dest || fetchName || "file"));
      setFetchOpen(false);
      setFetchUrl("");
      setFetchName("");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Download failed");
    } finally {
      setBusy(false);
    }
  }

  async function runSearch() {
    if (!searchQ.trim()) return;
    setBusy(true);
    try {
      const out = await api.post<{ hits?: { path: string; line: number; text: string }[] }>("/api/files/search", { user, path: listing.path, query: searchQ.trim() });
      setSearchHits(out.hits || []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Search failed");
    } finally {
      setBusy(false);
    }
  }

  async function pasteInto(dir: string) {
    if (!clip) return;
    const dest = joinPath(dir, clip.name);
    setBusy(true);
    try {
      if (clip.op === "move") {
        await api.post("/api/files/rename", { user, path: clip.path, dest });
        message.success("Moved");
        setClip(null);
      } else {
        await api.post("/api/files/copy", { user, path: clip.path, dest });
        message.success("Copied");
      }
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Paste failed");
    } finally {
      setBusy(false);
    }
  }

  async function applyMove() {
    if (!moveItem || !moveDest) return;
    setBusy(true);
    try {
      await api.post("/api/files/rename", { user, path: moveItem.path, dest: moveDest });
      message.success("Moved");
      setMoveItem(null);
      if (clip?.path === moveItem.path) setClip(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Move failed");
    } finally {
      setBusy(false);
    }
  }

  async function savePerm() {
    if (!perm) return;
    const mode = ((modeBits.u << 6) | (modeBits.g << 3) | modeBits.o).toString(8).padStart(3, "0");
    setBusy(true);
    try {
      await api.post("/api/files/chmod", { user, path: perm.path, mode });
      message.success("Permissions updated");
      setPerm(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  function openPerm(e: Entry) {
    const p = parseMode(e.mode);
    setModeBits({ u: p.u, g: p.g, o: p.o });
    setPerm(e);
  }

  function toggleBit(who: "u" | "g" | "o", flag: number, on: boolean) {
    setModeBits((cur) => {
      const next = { ...cur };
      next[who] = on ? cur[who] | flag : cur[who] & ~flag;
      return next;
    });
  }

  const options = [
    ...(admin ? [{ value: "root", label: "System (root)" }] : []),
    ...accounts.map((a) => ({ value: a.username, label: a.username })),
  ];

  const workspace = editorWorkspace(absPath, listing.absPath);
  const rows = useMemo(
    () =>
      [...listing.entries].sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
      }),
    [listing.entries],
  );

  const permWho: { key: "u" | "g" | "o"; label: string }[] = [
    { key: "u", label: "Owner" },
    { key: "g", label: "Group" },
    { key: "o", label: "Others" },
  ];

  function folderAbs(rel?: string) {
    if (rel && rel.startsWith("/") && root) return rel;
    if (listing.absPath && (!rel || rel === listing.path)) return listing.absPath;
    if (root) return rel || listing.path || "/";
    const home = `/home/${user}`;
    const p = (rel || listing.path || "/").replace(/\/+$/, "") || "/";
    if (p === "/") return home;
    return `${home}${p.startsWith("/") ? p : "/" + p}`;
  }

  function openTerminal(path?: string) {
    setTermCwd(folderAbs(path));
    setTermOpen(true);
  }

  function fileActions(e: Entry): MenuProps["items"] {
    const items: MenuProps["items"] = [
      { key: "download", icon: <DownloadOutlined />, label: "Download" },
    ];
    if (e.isDir) {
      items.push({ key: "terminal", icon: <LaptopOutlined />, label: "Terminal" });
    }
    if (!e.isDir && isArchive(e.name)) {
      items.push({ key: "extract", icon: <FileZipOutlined />, label: "Extract", disabled: busy });
    }
    items.push(
      { key: "perm", icon: <LockOutlined />, label: "Permissions" },
      { key: "copy", icon: <CopyOutlined />, label: "Copy" },
      { key: "move", icon: <ScissorOutlined />, label: "Move" },
      {
        key: "paste",
        icon: <SnippetsOutlined />,
        label: "Paste",
        disabled: !clip || busy || (e.isDir && clip.path === e.path),
      },
      { type: "divider" },
      { key: "delete", icon: <DeleteOutlined />, label: "Delete", danger: true },
    );
    return items;
  }

  function onFileAction(e: Entry, key: string) {
    if (key === "download") void download(e);
    if (key === "terminal") openTerminal(e.path);
    if (key === "extract") confirmExtract(e);
    if (key === "perm") openPerm(e);
    if (key === "copy") setClip({ path: e.path, name: e.name, isDir: e.isDir, op: "copy" });
    if (key === "move") {
      setMoveItem(e);
      setMoveDest(joinPath(listing.path, e.name));
    }
    if (key === "paste") void pasteInto(e.isDir ? e.path : listing.path);
    if (key === "delete") {
      modal.confirm({
        title: `Delete ${e.name}?`,
        okText: "Delete",
        okButtonProps: { danger: true },
        onOk: () => remove(e.path),
      });
    }
  }

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          File manager
        </Typography.Title>
        <Select style={{ minWidth: 160, maxWidth: "100%", width: 220 }} value={user || undefined} placeholder="Select account" onChange={setUser} options={options} />
      </div>
      {!user ? <Typography.Text type="secondary">Create a hosting account first.</Typography.Text> : null}
      <Card
        className="file-card"
        title={
          <Space wrap size={8} style={{ maxWidth: "100%" }}>
            <Typography.Text code className="file-path">
              {listing.path}
            </Typography.Text>
            {root ? <Tag color="red">root</Tag> : null}
            {listing.absPath && listing.absPath !== listing.path ? (
              <Typography.Text type="secondary" className="file-path">
                {listing.absPath}
              </Typography.Text>
            ) : null}
          </Space>
        }
      >
        <div className="file-toolbar">
          <Button onClick={() => load(parent())} disabled={listing.path === "/"}>
            Up
          </Button>
          {root
            ? rootShortcuts.map((p) => (
                <Button key={p} size="small" onClick={() => load(p)}>
                  {p}
                </Button>
              ))
            : null}
          <Input.Search placeholder="new folder" value={mkdir} onChange={(e) => setMkdir(e.target.value)} onSearch={makeDir} enterButton="Mkdir" />
          <Button type="primary" icon={<UploadOutlined />} onClick={() => setUpOpen(true)} disabled={!user}>
            Upload
          </Button>
          <Button icon={<CloudDownloadOutlined />} onClick={() => setFetchOpen(true)} disabled={!user || busy}>
            Remote download
          </Button>
          <Button icon={<SearchOutlined />} onClick={() => setSearchOpen(true)} disabled={!user || busy}>
            Search
          </Button>
          <Button icon={<LaptopOutlined />} onClick={() => openTerminal()} disabled={!user}>
            Terminal
          </Button>
          <Tooltip title="Paste here">
            <Button icon={<SnippetsOutlined />} disabled={!clip || busy} onClick={() => void pasteInto(listing.path)}>
              Paste
            </Button>
          </Tooltip>
        </div>
        {clip ? (
          <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
            {clip.op === "move" ? "Moving" : "Copied"} <Typography.Text code>{clip.name}</Typography.Text> — choose a folder, then Paste.
            <Button type="link" size="small" onClick={() => setClip(null)}>
              Cancel
            </Button>
          </Typography.Paragraph>
        ) : null}
        <div className="cp-table-wrap file-table">
          <Table
            size="small"
            rowKey="path"
            pagination={false}
            dataSource={rows}
            tableLayout="auto"
            columns={[
              {
                title: "Name",
                ellipsis: true,
                render: (_, e) => (
                  <Button type="link" className="file-name" onClick={() => open(e)}>
                    {e.isDir ? <FolderOutlined /> : isArchive(e.name) ? <FileZipOutlined /> : <FileOutlined />}
                    <span>{e.name}</span>
                  </Button>
                ),
              },
              {
                title: "Size",
                width: 110,
                align: "right" as const,
                render: (_, e: Entry) => (e.isDir ? "—" : formatBytes(e.size || 0)),
                sorter: (a: Entry, b: Entry) => {
                  if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
                  return (a.size || 0) - (b.size || 0);
                },
              },
              { title: "Mode", dataIndex: "mode", width: 80, responsive: ["sm"] },
              {
                title: "",
                width: 48,
                align: "right" as const,
                render: (_, e) => (
                  <Dropdown
                    trigger={["click"]}
                    placement="bottomRight"
                    getPopupContainer={() => document.body}
                    menu={{
                      items: fileActions(e),
                      onClick: ({ key, domEvent }) => {
                        domEvent.stopPropagation();
                        onFileAction(e, key);
                      },
                    }}
                  >
                    <Button type="text" size="small" icon={<MoreOutlined />} onClick={(ev) => ev.stopPropagation()} />
                  </Dropdown>
                ),
              },
            ]}
          />
        </div>
      </Card>

      <Modal
        title={`Upload to ${listing.path}`}
        open={upOpen}
        onCancel={() => !busy && setUpOpen(false)}
        footer={null}
        destroyOnHidden
      >
        <Space wrap style={{ marginBottom: 12 }}>
          <Upload
            showUploadList={false}
            multiple
            beforeUpload={(file, fileList) => {
              if (file === fileList[0]) void uploadMany(fileList as unknown as File[]);
              return Upload.LIST_IGNORE;
            }}
          >
            <Button icon={<UploadOutlined />} disabled={busy}>
              Upload files
            </Button>
          </Upload>
          <Upload
            showUploadList={false}
            directory
            multiple
            beforeUpload={(file, fileList) => {
              if (file === fileList[0]) void uploadMany(fileList as unknown as File[]);
              return Upload.LIST_IGNORE;
            }}
          >
            <Button icon={<FolderAddOutlined />} disabled={busy}>
              Upload folder
            </Button>
          </Upload>
        </Space>
        {upLabel ? <Progress percent={upPct} size="small" style={{ marginBottom: 12 }} format={() => upLabel} /> : null}
        <Upload.Dragger
          openFileDialogOnClick={false}
          showUploadList={false}
          multiple
          disabled={!user || busy}
          beforeUpload={() => false}
          customRequest={() => undefined}
          onDrop={(e) => {
            e.preventDefault();
            void filesFromDrop(e.dataTransfer).then((files) => uploadMany(files));
          }}
        >
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p className="ant-upload-text">Drop files or folders here</p>
          <p className="ant-upload-hint">Keeps folder structure. Current folder: {listing.path}</p>
        </Upload.Dragger>
      </Modal>

      <Modal
        title={`Move ${moveItem?.name || ""}`}
        open={!!moveItem}
        onCancel={() => setMoveItem(null)}
        onOk={() => void applyMove()}
        confirmLoading={busy}
        okText="Move"
      >
        <Typography.Paragraph type="secondary">Destination path (folder + name)</Typography.Paragraph>
        <Input value={moveDest} onChange={(e) => setMoveDest(e.target.value)} placeholder="/path/name" />
      </Modal>

      <Modal title={`Permissions · ${perm?.name || ""}`} open={!!perm} onCancel={() => setPerm(null)} onOk={() => void savePerm()} confirmLoading={busy} okText="Save">
        <Space direction="vertical" style={{ width: "100%" }}>
          {permWho.map((w) => (
            <div key={w.key}>
              <Typography.Text strong>{w.label}</Typography.Text>
              <div>
                <Checkbox checked={bits(modeBits[w.key], 4)} onChange={(e) => toggleBit(w.key, 4, e.target.checked)}>
                  Read
                </Checkbox>
                <Checkbox checked={bits(modeBits[w.key], 2)} onChange={(e) => toggleBit(w.key, 2, e.target.checked)}>
                  Write
                </Checkbox>
                <Checkbox checked={bits(modeBits[w.key], 1)} onChange={(e) => toggleBit(w.key, 1, e.target.checked)}>
                  Execute
                </Checkbox>
              </div>
            </div>
          ))}
          <Typography.Text type="secondary">
            Mode {(modeBits.u << 6 | modeBits.g << 3 | modeBits.o).toString(8).padStart(3, "0")}
          </Typography.Text>
        </Space>
      </Modal>

      <Modal
        title={
          <Space>
            <span>{editPath || "Edit file"}</span>
            {lspOn ? <Tag color="success">LSP</Tag> : null}
          </Space>
        }
        open={!!editPath}
        onCancel={() => {
          setEditPath("");
          setContent("");
          setAbsPath("");
          setEditorReady(false);
          setLspOn(false);
        }}
        onOk={save}
        confirmLoading={busy}
        okText="Save"
        width="90vw"
        destroyOnHidden
        afterOpenChange={setEditorReady}
        styles={{ body: { paddingTop: 12 } }}
      >
        {editorReady && editPath ? (
          <div
            style={{ height: "70vh" }}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
                e.preventDefault();
                void save();
              }
            }}
          >
            <Suspense fallback={<Spin style={{ display: "block", margin: "120px auto" }} />}>
              <MonacoFileEditor
                path={editPath}
                absPath={absPath}
                workspace={workspace}
                user={user}
                value={content}
                language={languageFromPath(editPath)}
                onChange={setContent}
                onSave={save}
                onLsp={setLspOn}
              />
            </Suspense>
          </div>
        ) : (
          <div style={{ height: "70vh" }} />
        )}
      </Modal>
      <Modal
        title="Remote download"
        open={fetchOpen}
        onCancel={() => setFetchOpen(false)}
        onOk={() => void remoteFetch()}
        confirmLoading={busy}
        okText="Download"
      >
        <Typography.Paragraph type="secondary">Fetches an HTTP(S) URL into {listing.path}</Typography.Paragraph>
        <Input placeholder="https://example.com/file.zip" value={fetchUrl} onChange={(e) => setFetchUrl(e.target.value)} style={{ marginBottom: 8 }} />
        <Input placeholder="optional filename" value={fetchName} onChange={(e) => setFetchName(e.target.value)} />
      </Modal>
      <Modal title="Search file content" open={searchOpen} onCancel={() => setSearchOpen(false)} footer={null} width={720}>
        <Input.Search placeholder="text or regex (ripgrep if installed)" value={searchQ} onChange={(e) => setSearchQ(e.target.value)} onSearch={() => void runSearch()} enterButton="Search" loading={busy} />
        <Table
          size="small"
          style={{ marginTop: 12 }}
          rowKey={(r) => r.path + ":" + r.line + r.text}
          dataSource={searchHits}
          pagination={{ pageSize: 8 }}
          locale={{ emptyText: "No matches." }}
          columns={[
            { title: "File", dataIndex: "path", ellipsis: true },
            { title: "Line", dataIndex: "line", width: 70 },
            { title: "Text", dataIndex: "text", ellipsis: true },
          ]}
        />
      </Modal>
      <Modal
        title={termCwd ? `Terminal · ${termCwd}` : "Terminal"}
        open={termOpen}
        onCancel={() => setTermOpen(false)}
        footer={null}
        width="90vw"
        destroyOnHidden
        styles={{ body: { paddingTop: 8 } }}
      >
        {termOpen && user ? (
          <div style={{ height: "62vh", minHeight: 360, display: "flex", flexDirection: "column" }}>
            <SSHTerminal user={user} cwd={termCwd} />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
