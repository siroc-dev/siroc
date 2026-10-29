import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { App, Modal, Radio, Typography } from "antd";
import { api } from "@/lib/api";

export function StackPrompt({ open, onDone }: { open: boolean; onDone: () => void }) {
  const { message } = App.useApp();
  const nav = useNavigate();
  const [db, setDb] = useState<"mariadb" | "mysql">("mariadb");
  const [busy, setBusy] = useState(false);

  async function choose(skip: boolean) {
    setBusy(true);
    try {
      await api.post("/api/me/stack", skip ? { skip: true } : { db });
      onDone();
      if (!skip) {
        message.success("Software queued");
        nav("/software");
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title="Recommended software"
      open={open}
      closable={false}
      maskClosable={false}
      keyboard={false}
      okText="Install"
      cancelText="Not now"
      confirmLoading={busy}
      onOk={() => void choose(false)}
      onCancel={() => void choose(true)}
      cancelButtonProps={{ disabled: busy }}
    >
      <Typography.Paragraph>
        Setup did not install a web stack. Install Nginx and Apache, one database, Redis, Let's Encrypt, and FTP.
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        Nginx 1.30, Apache 2.4, Redis 7.2, Let's Encrypt, FTP
      </Typography.Paragraph>
      <Radio.Group value={db} onChange={(e) => setDb(e.target.value)} disabled={busy}>
        <Radio value="mariadb">MariaDB 11.4</Radio>
        <Radio value="mysql">MySQL 8.4</Radio>
      </Radio.Group>
      <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
        This prompt shows once. You can install or change any of this later from the Software page.
      </Typography.Paragraph>
    </Modal>
  );
}
