import { useState } from "react";
import type { CSSProperties } from "react";
import { Button, Input, Modal, Space, Table, Typography } from "antd";
import { FileOutlined, FolderOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { parentFilePath } from "@/lib/filePaths";

type Entry = { name: string; path: string; isDir: boolean };
type Listing = { path: string; absPath?: string; entries?: Entry[] };

function stripHome(abs: string, relativeTo?: string) {
  const root = (relativeTo || "").replace(/\/+$/, "");
  const path = (abs || "").replace(/\/+$/, "") || "/";
  if (!root) return path;
  if (path === root) return "";
  if (path.startsWith(root + "/")) return path.slice(root.length + 1);
  return path;
}

export function PathPicker({
  value,
  onChange,
  user = "root",
  placeholder,
  disabled,
  style,
  dirsOnly,
  relativeTo,
  addonBefore,
}: {
  value?: string;
  onChange?: (value: string) => void;
  user?: string;
  placeholder?: string;
  disabled?: boolean;
  style?: CSSProperties;
  dirsOnly?: boolean;
  relativeTo?: string;
  addonBefore?: string;
}) {
  const [open, setOpen] = useState(false);
  const [listing, setListing] = useState<Listing>({ path: "/" });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [filePick, setFilePick] = useState("");

  const here = listing.absPath || listing.path || "/";

  async function load(path: string, fallback = true) {
    setBusy(true);
    setErr("");
    setFilePick("");
    const next = path || "/";
    try {
      const data = await api.get<Listing>(`/api/files?user=${encodeURIComponent(user)}&path=${encodeURIComponent(next)}`);
      setListing({ path: data.path || "/", absPath: data.absPath, entries: data.entries || [] });
    } catch (e) {
      const home = user === "root" ? "/home" : "/";
      if (fallback && next !== home && next !== "/") {
        await load(home, false);
        return;
      }
      setErr(e instanceof Error ? e.message : "Could not open this folder");
    } finally {
      setBusy(false);
    }
  }

  function openBrowser() {
    const start = (value || "").trim();
    setOpen(true);
    void load(start || (user === "root" ? "/home" : "/"));
  }

  function choose(abs: string) {
    onChange?.(stripHome(abs, relativeTo));
    setOpen(false);
  }

  const rows = [...(listing.entries || [])].sort((a, b) => Number(b.isDir) - Number(a.isDir) || a.name.localeCompare(b.name));
  const parent = parentFilePath(listing.path || "/");

  return (
    <>
      <Space.Compact style={{ width: "100%", ...style }}>
        <Input addonBefore={addonBefore} value={value} disabled={disabled} placeholder={placeholder} onChange={(e) => onChange?.(e.target.value)} />
        <Button disabled={disabled} onClick={openBrowser}>
          Browse
        </Button>
      </Space.Compact>
      <Modal
        title="Choose a path"
        open={open}
        onCancel={() => setOpen(false)}
        okText={filePick ? "Use this file" : "Use this folder"}
        onOk={() => choose(filePick || here)}
        width={680}
        destroyOnHidden
      >
        <Space direction="vertical" style={{ width: "100%" }} size={8}>
          <Typography.Text code>{here}</Typography.Text>
          {err ? <Typography.Text type="danger">{err}</Typography.Text> : null}
          <Table
            size="small"
            loading={busy}
            rowKey={(row) => row.path + row.name}
            pagination={rows.length > 12 ? { pageSize: 12 } : false}
            scroll={{ y: 360 }}
            dataSource={listing.path && listing.path !== "/" ? [{ name: "..", path: parent, isDir: true }, ...rows] : rows}
            onRow={(row) => ({
              onClick: () => {
                if (row.isDir) {
                  void load(row.name === ".." ? parent : row.path);
                  return;
                }
                if (!dirsOnly) setFilePick(row.path.startsWith("/") && user === "root" ? row.path : joinAbs(here, row.name));
              },
              style: { cursor: "pointer" },
            })}
            rowClassName={(row) => (!row.isDir && filePick && filePick.endsWith("/" + row.name) ? "ant-table-row-selected" : "")}
            columns={[
              {
                title: "Name",
                render: (_, row) => (
                  <Space>
                    {row.isDir ? <FolderOutlined /> : <FileOutlined />}
                    {row.name}
                  </Space>
                ),
              },
            ]}
          />
        </Space>
      </Modal>
    </>
  );
}

function joinAbs(dir: string, name: string) {
  const base = (dir || "/").replace(/\/+$/, "") || "/";
  if (base === "/") return "/" + name;
  return base + "/" + name;
}
