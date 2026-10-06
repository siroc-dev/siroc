import { lazy, Suspense, useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { useOutletContext, useSearchParams } from "react-router-dom";
import {
  App,
  Breadcrumb,
  Button,
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
  Tree,
  Typography,
  Upload,
} from "antd";
import type { MenuProps } from "antd";
import type { DataNode } from "antd/es/tree";
import {
  CloudDownloadOutlined,
  CopyOutlined,
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  EyeOutlined,
  FileAddOutlined,
  FileOutlined,
  FileZipOutlined,
  FolderAddOutlined,
  FolderOutlined,
  HomeOutlined,
  InboxOutlined,
  LaptopOutlined,
  LockOutlined,
  MoreOutlined,
  ReloadOutlined,
  SearchOutlined,
  ScissorOutlined,
  SnippetsOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import { api } from "@/lib/api";
import { PageSkeleton } from "@/components/PageSkeleton";
import { languageFromPath } from "@/lib/fileLang";
import { isSystemPath, joinPath, moveDestinations, normalizeFileJump } from "@/lib/filePaths";
import { editorWorkspace, formatBytes } from "@/lib/usage";
import { SSHTerminal } from "@/components/SSHTerminal";

const MonacoFileEditor = lazy(() => import("@/components/MonacoFileEditor").then((m) => ({ default: m.MonacoFileEditor })));

type Account = { username: string };
type Entry = { name: string; path: string; isDir: boolean; size: number; mode: string; modTime: string };
type Listing = { path: string; absPath?: string; root?: boolean; entries: Entry[] };
type Clip = { items: { path: string; name: string; isDir: boolean }[]; op: "copy" | "move" };

const rootShortcuts = ["/", "/etc", "/home", "/opt/siroc", "/var/log", "/etc/nginx", "/etc/php"];

function isArchive(name: string) {
  const n = name.toLowerCase();
  return [".zip", ".rar", ".7z", ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".gz", ".bz2", ".xz"].some((ext) => n.endsWith(ext));
}

const imageTypes: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  gif: "image/gif",
  webp: "image/webp",
  bmp: "image/bmp",
  ico: "image/x-icon",
  avif: "image/avif",
  svg: "image/svg+xml",
};

function imageExt(name: string) {
  const n = name.toLowerCase();
  const dot = n.lastIndexOf(".");
  return dot >= 0 ? n.slice(dot + 1) : "";
}

function isImage(name: string) {
  return imageExt(name) in imageTypes;
}

function parseMode(mode: string) {
  const s = (mode || "644").replace(/[^0-7]/g, "").slice(-3).padStart(3, "0");
  const n = parseInt(s, 8) || 0;
  return { u: (n >> 6) & 7, g: (n >> 3) & 7, o: n & 7, raw: s };
}

function bits(v: number, flag: number) {
  return (v & flag) === flag;
}

function fmtTime(s: string) {
  if (!s) return "—";
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

function crumbParts(path: string) {
  const parts = path.split("/").filter(Boolean);
  const items: { title: string; path: string }[] = [{ title: "Home directory", path: "/" }];
  let cur = "";
  for (const part of parts) {
    cur += "/" + part;
    items.push({ title: part, path: cur });
  }
  return items;
}

function isSafeName(name: string) {
  const n = name.trim();
  return n !== "" && !n.includes("/") && !n.includes("\\") && n !== "." && n !== "..";
}

export function Files() {
  const { message, modal } = App.useApp();
  const { admin } = useOutletContext<{ user: string; admin?: boolean }>();
  const [search] = useSearchParams();
  const wantUser = (search.get("user") || "").trim();
  const wantPath = (search.get("path") || "").trim() || "/";
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [user, setUser] = useState("");
  const [ready, setReady] = useState(false);
  const [listing, setListing] = useState<Listing>({ path: "/", entries: [] });
  const [editPath, setEditPath] = useState("");
  const [absPath, setAbsPath] = useState("");
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);
  const [editorReady, setEditorReady] = useState(false);
  const [lspOn, setLspOn] = useState(false);
  const [upPct, setUpPct] = useState(0);
  const [upLabel, setUpLabel] = useState("");
  const [upOpen, setUpOpen] = useState(false);
  const [clip, setClip] = useState<Clip | null>(null);
  const [perm, setPerm] = useState<Entry | null>(null);
  const [modeBits, setModeBits] = useState({ u: 6, g: 4, o: 4 });
  const [moveItems, setMoveItems] = useState<Entry[]>([]);
  const [moveDest, setMoveDest] = useState("");
  const [fetchOpen, setFetchOpen] = useState(false);
  const [fetchUrl, setFetchUrl] = useState("");
  const [fetchName, setFetchName] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQ, setSearchQ] = useState("");
  const [searchHits, setSearchHits] = useState<{ path: string; line: number; text: string }[]>([]);
  const [termOpen, setTermOpen] = useState(false);
  const [termCwd, setTermCwd] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [ctxOpen, setCtxOpen] = useState(false);
  const [ctxItems, setCtxItems] = useState<Entry[]>([]);
  const [ctxPoint, setCtxPoint] = useState({ x: 0, y: 0 });
  const [filter, setFilter] = useState("");
  const [filePage, setFilePage] = useState(1);
  const [pathDraft, setPathDraft] = useState("/");
  const pendingJump = useRef<string | null>(null);
  const [treeKids, setTreeKids] = useState<Record<string, Entry[]>>({});
  const [expanded, setExpanded] = useState<string[]>(["/"]);
  const [fileOpen, setFileOpen] = useState(false);
  const [folderOpen, setFolderOpen] = useState(false);
  const [renameItem, setRenameItem] = useState<Entry | null>(null);
  const [renameName, setRenameName] = useState("");
  const [preview, setPreview] = useState<{ name: string; url: string } | null>(null);
  const [newFile, setNewFile] = useState("");
  const [newFolder, setNewFolder] = useState("");

  const root = user === "root";

  async function load(path = listing.path) {
    if (!user) return;
    const data = await api.get<Listing>(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`);
    setListing(data);
    setTreeKids((prev) => ({ ...prev, [data.path]: data.entries }));
    setSelected([]);
    const keys = ["/"];
    let cur = "";
    for (const part of data.path.split("/").filter(Boolean)) {
      cur += "/" + part;
      keys.push(cur);
    }
    setExpanded((prev) => Array.from(new Set([...prev, ...keys])));
    void prefetchTree(data.path);
  }

  async function prefetchTree(path: string) {
    if (!user) return;
    const parts = path.split("/").filter(Boolean);
    let cur = "/";
    const next: Record<string, Entry[]> = {};
    for (let i = 0; i <= parts.length; i++) {
      if (treeKids[cur]) {
        if (i < parts.length) cur = joinPath(cur, parts[i]);
        continue;
      }
      try {
        const data = await api.get<Listing>(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(cur)}`);
        next[data.path] = data.entries;
      } catch {
        break;
      }
      if (i < parts.length) cur = joinPath(cur, parts[i]);
    }
    if (Object.keys(next).length) setTreeKids((prev) => ({ ...prev, ...next }));
  }

  async function loadTreeNode(key: string) {
    if (!user || treeKids[key]) return;
    const data = await api.get<Listing>(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(key)}`);
    setTreeKids((prev) => ({ ...prev, [data.path]: data.entries }));
  }

  useEffect(() => {
    api
      .get<Account[]>("/api/accounts")
      .then((a) => {
        setAccounts(a);
        const allowed = wantUser === "root" ? !!admin : !wantUser || a.some((x) => x.username === wantUser);
        let next = "";
        if (wantUser && allowed) next = wantUser;
        else if (a[0]) next = a[0].username;
        else if (admin) next = "root";
        if (next) setUser(next);
        else setReady(true);
      })
      .catch((e) => {
        message.error(e.message);
        setReady(true);
      });
  }, [admin, wantUser]);

  useEffect(() => {
    if (!user) return;
    setTreeKids({});
    setExpanded(["/"]);
    const start = pendingJump.current ?? (wantUser && user === wantUser ? wantPath : "/");
    pendingJump.current = null;
    load(start)
      .catch((e) => message.error(e.message))
      .finally(() => setReady(true));
  }, [user, wantUser, wantPath]);

  useEffect(() => {
    setPathDraft(listing.path || "/");
  }, [listing.path]);

  async function goPath() {
    const path = normalizeFileJump(pathDraft);
    setPathDraft(path);
    if (admin && user !== "root" && isSystemPath(path)) {
      pendingJump.current = path;
      setUser("root");
      return;
    }
    try {
      await load(path);
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Could not open that path");
    }
  }

  function parent() {
    const parts = listing.path.split("/").filter(Boolean);
    parts.pop();
    return "/" + parts.join("/");
  }

  function closePreview() {
    setPreview((cur) => {
      if (cur?.url) URL.revokeObjectURL(cur.url);
      return null;
    });
  }

  async function previewImage(e: Entry) {
    if (e.size > 25 * 1024 * 1024) {
      message.error("Image is larger than 25MB. Download it instead.");
      return;
    }
    try {
      const q = new URLSearchParams({ user, path: e.path });
      const res = await fetch(`/api/files/download?${q}`, { credentials: "include" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || "Preview failed");
      }
      const raw = await res.blob();
      const type = imageTypes[imageExt(e.name)] || raw.type || "application/octet-stream";
      const blob = raw.type === type ? raw : new Blob([raw], { type });
      const url = URL.createObjectURL(blob);
      setPreview((cur) => {
        if (cur?.url) URL.revokeObjectURL(cur.url);
        return { name: e.name, url };
      });
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Preview failed");
    }
  }

  async function open(e: Entry) {
    if (e.isDir) {
      await load(e.path);
      return;
    }
    if (isImage(e.name)) {
      await previewImage(e);
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
    if (!isSafeName(newFolder)) {
      message.error("Enter a folder name without slashes");
      return;
    }
    const path = joinPath(listing.path, newFolder.trim());
    try {
      await api.post("/api/files/mkdir", { user, path });
      setNewFolder("");
      setFolderOpen(false);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function makeFile() {
    if (!isSafeName(newFile)) {
      message.error("Enter a file name without slashes");
      return;
    }
    const path = joinPath(listing.path, newFile.trim());
    try {
      await api.put("/api/files/content", { user, path, content: "" });
      setNewFile("");
      setFileOpen(false);
      await load();
      const data = await api.get<{ content: string; absPath?: string }>(
        `/api/files/content?user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`,
      );
      setEditPath(path);
      setAbsPath(data.absPath || "");
      setContent(data.content);
      setLspOn(false);
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
      if (clip?.items.some((i) => i.path === path)) setClip(null);
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
        await new Promise<void>((resolve, reject) => {
          const xhr = new XMLHttpRequest();
          xhr.open("POST", "/api/files/upload");
          xhr.withCredentials = true;
          xhr.upload.onprogress = (ev) => {
            const filePct = ev.lengthComputable && ev.total > 0 ? ev.loaded / ev.total : 0;
            const pct = Math.round(((i + filePct) / list.length) * 100);
            setUpPct(pct);
            setUpLabel(`${f.name} · ${pct}%`);
          };
          xhr.onload = () => {
            let data: { error?: string } = {};
            try {
              data = JSON.parse(xhr.responseText || "{}");
            } catch {
              data = {};
            }
            if (xhr.status >= 200 && xhr.status < 300) resolve();
            else reject(new Error(data.error || `Upload failed: ${f.name}`));
          };
          xhr.onerror = () => reject(new Error(`Upload failed: ${f.name}`));
          xhr.send(fd);
        });
        ok++;
        setUpPct(Math.round(((i + 1) / list.length) * 100));
        setUpLabel(`${f.name} · ${Math.round(((i + 1) / list.length) * 100)}%`);
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

  async function archiveEntry(e: Entry) {
    setBusy(true);
    try {
      const out = await api.post<{ dest?: string }>("/api/files/archive", { user, path: e.path });
      message.success("Archived");
      await load();
      if (out.dest) {
        const parent = out.dest.replace(/\/[^/]+$/, "") || "/";
        if (parent !== listing.path) await load(parent);
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Archive failed");
    } finally {
      setBusy(false);
    }
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

  function setClipboard(items: Entry[], op: "copy" | "move") {
    if (!items.length) return;
    setClip({ items: items.map((e) => ({ path: e.path, name: e.name, isDir: e.isDir })), op });
    const label = items.length === 1 ? items[0].name : `${items.length} items`;
    message.success(op === "copy" ? `Copied ${label}` : `Cut ${label}`);
  }

  async function pasteInto(dir: string) {
    if (!clip?.items.length) return;
    setBusy(true);
    try {
      for (const item of clip.items) {
        const dest = joinPath(dir, item.name);
        if (clip.op === "move") {
          await api.post("/api/files/rename", { user, path: item.path, dest });
        } else {
          await api.post("/api/files/copy", { user, path: item.path, dest });
        }
      }
      const n = clip.items.length;
      message.success(clip.op === "move" ? (n === 1 ? "Moved" : `Moved ${n} items`) : n === 1 ? "Copied" : `Copied ${n} items`);
      if (clip.op === "move") setClip(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Paste failed");
    } finally {
      setBusy(false);
    }
  }

  function openRename(e: Entry) {
    setRenameItem(e);
    setRenameName(e.name);
  }

  async function applyRename() {
    if (!renameItem) return;
    const name = renameName.trim();
    if (!isSafeName(name)) {
      message.error("Use a file name without / or ..");
      return;
    }
    if (name === renameItem.name) {
      setRenameItem(null);
      return;
    }
    const parent = renameItem.path.replace(/\/[^/]+$/, "") || "/";
    setBusy(true);
    try {
      await api.post("/api/files/rename", { user, path: renameItem.path, dest: joinPath(parent, name) });
      message.success("Renamed");
      setRenameItem(null);
      setSelected([]);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Rename failed");
    } finally {
      setBusy(false);
    }
  }

  function openMove(items: Entry[]) {
    if (!items.length) return;
    setMoveItems(items);
    setMoveDest(items.length === 1 ? joinPath(listing.path, items[0].name) : listing.path);
  }

  async function applyMove() {
    const jobs = moveDestinations(moveItems, moveDest);
    if (!jobs.length) return;
    setBusy(true);
    try {
      for (const job of jobs) {
        await api.post("/api/files/rename", { user, path: job.path, dest: job.dest });
      }
      message.success(jobs.length === 1 ? "Moved" : `Moved ${jobs.length} items`);
      setMoveItems([]);
      setSelected([]);
      if (clip && jobs.some((j) => clip.items.some((i) => i.path === j.path))) setClip(null);
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
  useEffect(() => {
    setFilePage(1);
  }, [listing.path, filter]);

  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return [...listing.entries]
      .filter((e) => !q || e.name.toLowerCase().includes(q))
      .sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
      });
  }, [listing.entries, filter]);
  const picked = useMemo(() => rows.filter((e) => selected.includes(e.path)), [rows, selected]);
  const treeData = useMemo<DataNode[]>(() => {
    const walk = (path: string): DataNode[] =>
      (treeKids[path] || [])
        .filter((e) => e.isDir)
        .map((e) => ({
          title: e.name,
          key: e.path,
          children: treeKids[e.path] ? walk(e.path) : undefined,
        }));
    return [
      {
        title: "Home directory",
        key: "/",
        icon: <HomeOutlined />,
        children: walk("/"),
      },
    ];
  }, [treeKids]);

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

  function needPicked(action: string) {
    if (picked.length) return true;
    message.info(`Select a file or folder to ${action}`);
    return false;
  }

  function toolbarCopy() {
    if (!needPicked("copy")) return;
    setClipboard(picked, "copy");
  }

  function toolbarMove() {
    if (!needPicked("move")) return;
    openMove(picked);
  }

  function toolbarRename() {
    if (picked.length !== 1) {
      message.info("Select one file or folder to rename");
      return;
    }
    openRename(picked[0]);
  }

  function toolbarExtract() {
    const e = picked.find((x) => !x.isDir && isArchive(x.name));
    if (!e) {
      message.info("Select a zip, tar, or 7z file to extract");
      return;
    }
    confirmExtract(e);
  }

  function toolbarArchive() {
    if (!needPicked("archive")) return;
    void archiveEntry(picked[0]);
  }

  function toolbarDelete() {
    if (!needPicked("delete")) return;
    const names = picked.map((e) => e.name).join(", ");
    modal.confirm({
      title: picked.length === 1 ? `Delete ${picked[0].name}?` : `Delete ${picked.length} items?`,
      content: names,
      okText: "Delete",
      okButtonProps: { danger: true },
      onOk: async () => {
        for (const e of picked) await remove(e.path);
      },
    });
  }

  function actionTargets(e: Entry) {
    return picked.some((x) => x.path === e.path) && picked.length > 1 ? picked : [e];
  }

  function fileActions(items: Entry[]): MenuProps["items"] {
    const one = items.length === 1 ? items[0] : null;
    const out: MenuProps["items"] = [];
    if (one) {
      out.push({ key: "open", icon: one.isDir ? <FolderOutlined /> : <FileOutlined />, label: "Open" });
      if (!one.isDir && isImage(one.name)) out.push({ key: "preview", icon: <EyeOutlined />, label: "Preview" });
      out.push({ key: "rename", icon: <EditOutlined />, label: "Rename" });
      out.push({ key: "download", icon: <DownloadOutlined />, label: "Download" });
      if (one.isDir) out.push({ key: "terminal", icon: <LaptopOutlined />, label: "Terminal" });
      if (!one.isDir && isArchive(one.name)) {
        out.push({ key: "extract", icon: <FileZipOutlined />, label: "Extract", disabled: busy });
      } else {
        out.push({ key: "archive", icon: <FileZipOutlined />, label: "Archive", disabled: busy });
      }
      out.push({ key: "perm", icon: <LockOutlined />, label: "Permissions" });
    } else if (items.length > 1) {
      out.push({ key: "download", icon: <DownloadOutlined />, label: "Download" });
    }
    const pasteDir = one?.isDir ? one.path : listing.path;
    out.push(
      { key: "copy", icon: <CopyOutlined />, label: items.length > 1 ? `Copy ${items.length} items` : "Copy" },
      { key: "move", icon: <ScissorOutlined />, label: items.length > 1 ? `Move ${items.length} items` : "Move" },
      {
        key: "paste",
        icon: <SnippetsOutlined />,
        label: "Paste",
        disabled: !clip || busy || clip.items.some((i) => i.path === pasteDir),
      },
      { type: "divider" },
      { key: "delete", icon: <DeleteOutlined />, label: items.length > 1 ? `Delete ${items.length} items` : "Delete", danger: true },
    );
    return out;
  }

  function folderActions(): MenuProps["items"] {
    return [
      { key: "new-file", icon: <FileAddOutlined />, label: "New file" },
      { key: "new-folder", icon: <FolderAddOutlined />, label: "New folder" },
      { key: "upload", icon: <UploadOutlined />, label: "Upload" },
      { key: "paste", icon: <SnippetsOutlined />, label: "Paste", disabled: !clip || busy },
      { type: "divider" },
      { key: "refresh", icon: <ReloadOutlined />, label: "Refresh" },
    ];
  }

  function onFileAction(items: Entry[], key: string) {
    const one = items.length === 1 ? items[0] : null;
    if (key === "open" && one) void open(one);
    if (key === "preview" && one) void previewImage(one);
    if (key === "rename" && one) openRename(one);
    if (key === "download") {
      for (const e of items.filter((x) => !x.isDir)) void download(e);
    }
    if (key === "terminal" && one) openTerminal(one.path);
    if (key === "extract" && one) confirmExtract(one);
    if (key === "archive" && one) void archiveEntry(one);
    if (key === "perm" && one) openPerm(one);
    if (key === "copy") setClipboard(items, "copy");
    if (key === "move") openMove(items);
    if (key === "paste") void pasteInto(one?.isDir ? one.path : listing.path);
    if (key === "delete") {
      modal.confirm({
        title: items.length === 1 ? `Delete ${items[0].name}?` : `Delete ${items.length} items?`,
        content: items.map((e) => e.name).join(", "),
        okText: "Delete",
        okButtonProps: { danger: true },
        onOk: async () => {
          for (const e of items) await remove(e.path);
        },
      });
    }
  }

  function onFolderAction(key: string) {
    if (key === "new-file") setFileOpen(true);
    if (key === "new-folder") setFolderOpen(true);
    if (key === "upload") setUpOpen(true);
    if (key === "paste") void pasteInto(listing.path);
    if (key === "refresh") void load();
  }

  function openContext(ev: MouseEvent, items: Entry[]) {
    ev.preventDefault();
    ev.stopPropagation();
    setCtxItems(items);
    setCtxPoint({ x: ev.clientX, y: ev.clientY });
    setCtxOpen(true);
  }

  if (!ready) return <PageSkeleton rows={10} />;

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          File manager
        </Typography.Title>
        <Select style={{ minWidth: 160, maxWidth: "100%", width: 220 }} value={user || undefined} placeholder="Select account" onChange={setUser} options={options} />
      </div>
      {!user ? <Typography.Text type="secondary">Create a hosting account first.</Typography.Text> : null}
      <div className="fm-shell file-card">
        <aside className="fm-tree">
          <div className="fm-tree-title">Home directory</div>
          {root ? (
            <div className="fm-tree-shortcuts">
              {rootShortcuts.map((p) => (
                <Button key={p} type="link" size="small" onClick={() => void load(p)}>
                  {p}
                </Button>
              ))}
            </div>
          ) : null}
          <Tree.DirectoryTree
            blockNode
            showIcon
            treeData={treeData}
            selectedKeys={[listing.path]}
            expandedKeys={expanded}
            onExpand={(keys) => setExpanded(keys.map(String))}
            onSelect={(keys) => {
              const key = String(keys[0] || "/");
              void load(key);
            }}
            loadData={async (node) => loadTreeNode(String(node.key))}
          />
        </aside>
        <section className="fm-main">
          <div className="fm-toolbar">
            <Button icon={<FileAddOutlined />} disabled={!user} onClick={() => setFileOpen(true)}>
              New file
            </Button>
            <Button icon={<FolderAddOutlined />} disabled={!user} onClick={() => setFolderOpen(true)}>
              New folder
            </Button>
            <Button type="primary" icon={<UploadOutlined />} onClick={() => setUpOpen(true)} disabled={!user}>
              Upload
            </Button>
            <Button icon={<FileZipOutlined />} disabled={!user || busy} onClick={toolbarExtract}>
              Extract
            </Button>
            <Button icon={<FileZipOutlined />} disabled={!user || busy} onClick={toolbarArchive}>
              Archive
            </Button>
            <Button icon={<CopyOutlined />} disabled={!user} onClick={toolbarCopy}>
              Copy
            </Button>
            <Button icon={<ScissorOutlined />} disabled={!user} onClick={toolbarMove}>
              Move
            </Button>
            <Button icon={<EditOutlined />} disabled={!user || picked.length !== 1} onClick={toolbarRename}>
              Rename
            </Button>
            <Tooltip title="Paste here">
              <Button icon={<SnippetsOutlined />} disabled={!clip || busy} onClick={() => void pasteInto(listing.path)}>
                Paste
              </Button>
            </Tooltip>
            <Button danger icon={<DeleteOutlined />} disabled={!user || busy} onClick={toolbarDelete}>
              Remove
            </Button>
            <Dropdown
              menu={{
                items: [
                  { key: "fetch", icon: <CloudDownloadOutlined />, label: "Remote download" },
                  { key: "search", icon: <SearchOutlined />, label: "Search contents" },
                  { key: "term", icon: <LaptopOutlined />, label: "Terminal" },
                  { key: "up", label: "Go up", disabled: listing.path === "/" },
                  { key: "refresh", icon: <ReloadOutlined />, label: "Refresh" },
                ],
                onClick: ({ key }) => {
                  if (key === "fetch") setFetchOpen(true);
                  if (key === "search") setSearchOpen(true);
                  if (key === "term") openTerminal();
                  if (key === "up") void load(parent());
                  if (key === "refresh") void load();
                },
              }}
            >
              <Button icon={<MoreOutlined />}>More</Button>
            </Dropdown>
            <Input.Search
              allowClear
              placeholder="Search in this folder"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              style={{ marginLeft: "auto", maxWidth: 240 }}
            />
          </div>
          <div className="fm-crumb">
            <Input
              className="fm-path"
              value={pathDraft}
              spellCheck={false}
              disabled={!user}
              placeholder={root ? "/etc/ssh" : "/domains/example/public_html"}
              onChange={(e) => setPathDraft(e.target.value)}
              onPressEnter={() => void goPath()}
            />
            <Button disabled={!user} onClick={() => void goPath()}>
              Open
            </Button>
            <Breadcrumb
              items={crumbParts(listing.path).map((c) => ({
                title: (
                  <button type="button" className="fm-crumb-btn" onClick={() => void load(c.path)}>
                    {c.path === "/" ? <HomeOutlined /> : null} {c.title}
                  </button>
                ),
              }))}
            />
            {root ? <Tag color="red">root</Tag> : null}
            {listing.absPath && listing.absPath !== listing.path ? (
              <Typography.Text type="secondary" className="file-path">
                {listing.absPath}
              </Typography.Text>
            ) : null}
          </div>
          {clip ? (
            <Typography.Paragraph type="secondary" style={{ margin: "8px 12px 0" }}>
              {clip.op === "move" ? "Moving" : "Copied"}{" "}
              <Typography.Text code>{clip.items.length === 1 ? clip.items[0].name : `${clip.items.length} items`}</Typography.Text>{" "}
              — open a folder, then Paste.
              <Button type="link" size="small" onClick={() => setClip(null)}>
                Cancel
              </Button>
            </Typography.Paragraph>
          ) : null}
          <div
            className="cp-table-wrap file-table fm-table"
            onContextMenu={(ev) => {
              const row = (ev.target as HTMLElement).closest("tr.ant-table-row");
              if (row) return;
              openContext(ev, []);
            }}
          >
            <Table
              size="small"
              rowKey="path"
              pagination={{
                current: filePage,
                pageSize: 50,
                showSizeChanger: false,
                hideOnSinglePage: true,
                showTotal: (total) => `${total} items`,
                onChange: (page) => setFilePage(page),
              }}
              dataSource={rows}
              tableLayout="auto"
              rowSelection={{
                selectedRowKeys: selected,
                onChange: (keys) => setSelected(keys.map(String)),
              }}
              onRow={(record) => ({
                onContextMenu: (ev) => {
                  const items = selected.includes(record.path) && selected.length > 1 ? picked : [record];
                  if (!selected.includes(record.path)) setSelected([record.path]);
                  openContext(ev, items);
                },
              })}
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
                  title: "Modified",
                  width: 180,
                  responsive: ["sm"],
                  render: (_, e: Entry) => fmtTime(e.modTime),
                  sorter: (a: Entry, b: Entry) => (a.modTime || "").localeCompare(b.modTime || ""),
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
                { title: "Permissions", dataIndex: "mode", width: 110, responsive: ["md"] },
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
                        items: fileActions(actionTargets(e)),
                        onClick: ({ key, domEvent }) => {
                          domEvent.stopPropagation();
                          onFileAction(actionTargets(e), key);
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
        </section>
      </div>

      {ctxOpen ? (
        <Dropdown
          open
          trigger={["click"]}
          onOpenChange={(v) => {
            if (!v) setCtxOpen(false);
          }}
          getPopupContainer={() => document.body}
          menu={{
            items: ctxItems.length ? fileActions(ctxItems) : folderActions(),
            onClick: ({ key }) => {
              setCtxOpen(false);
              if (ctxItems.length) onFileAction(ctxItems, key);
              else onFolderAction(key);
            },
          }}
        >
          <span className="fm-ctx-anchor" style={{ position: "fixed", left: ctxPoint.x, top: ctxPoint.y, width: 1, height: 1 }} />
        </Dropdown>
      ) : null}

      <Modal
        title="New file"
        open={fileOpen}
        onCancel={() => setFileOpen(false)}
        onOk={() => void makeFile()}
        okText="Create"
        destroyOnHidden
      >
        <Input
          autoFocus
          placeholder="index.php"
          value={newFile}
          onChange={(e) => setNewFile(e.target.value)}
          onPressEnter={() => void makeFile()}
        />
      </Modal>
      <Modal
        title="New folder"
        open={folderOpen}
        onCancel={() => setFolderOpen(false)}
        onOk={() => void makeDir()}
        okText="Create"
        destroyOnHidden
      >
        <Input
          autoFocus
          placeholder="httpdocs"
          value={newFolder}
          onChange={(e) => setNewFolder(e.target.value)}
          onPressEnter={() => void makeDir()}
        />
      </Modal>
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
        title={renameItem ? `Rename ${renameItem.name}` : "Rename"}
        open={!!renameItem}
        onCancel={() => setRenameItem(null)}
        onOk={() => void applyRename()}
        confirmLoading={busy}
        okText="Rename"
        destroyOnHidden
      >
        <Input value={renameName} autoFocus onChange={(e) => setRenameName(e.target.value)} onPressEnter={() => void applyRename()} />
      </Modal>

      <Modal title={preview?.name || "Preview"} open={!!preview} onCancel={closePreview} footer={null} width={880} destroyOnHidden>
        {preview ? <img className="fm-preview" src={preview.url} alt={preview.name} /> : null}
      </Modal>

      <Modal
        title={moveItems.length === 1 ? `Move ${moveItems[0].name}` : `Move ${moveItems.length} items`}
        open={moveItems.length > 0}
        onCancel={() => setMoveItems([])}
        onOk={() => void applyMove()}
        confirmLoading={busy}
        okText="Move"
      >
        {moveItems.length > 1 ? (
          <Typography.Paragraph type="secondary">
            {moveItems.map((e) => e.name).join(", ")}
          </Typography.Paragraph>
        ) : null}
        <Typography.Paragraph type="secondary">
          {moveItems.length === 1 ? "Destination path (folder + name)" : "Destination folder. Each item keeps its name."}
        </Typography.Paragraph>
        <Input
          value={moveDest}
          onChange={(e) => setMoveDest(e.target.value)}
          placeholder={moveItems.length === 1 ? "/path/name" : "/path/folder"}
        />
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
