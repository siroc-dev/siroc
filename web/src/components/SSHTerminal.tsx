import { useCallback, useEffect, useRef, useState } from "react";
import { App, Button, Dropdown, Space, Tag, Typography } from "antd";
import { CopyOutlined, SnippetsOutlined } from "@ant-design/icons";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

type Props = {
  user: string;
  cwd?: string;
};

export function SSHTerminal({ user, cwd }: Props) {
  const { message } = App.useApp();
  const host = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<"connecting" | "online" | "offline">("connecting");

  const copySelection = useCallback(async () => {
    const term = termRef.current;
    const text = term?.getSelection() || "";
    if (!text) {
      message.info("Select text in the terminal first");
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      message.success("Copied");
    } catch {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      ta.remove();
      message.success("Copied");
    }
  }, [message]);

  const pasteClipboard = useCallback(async () => {
    const term = termRef.current;
    if (!term) return;
    try {
      const text = await navigator.clipboard.readText();
      if (text) term.paste(text);
    } catch {
      message.warning("Allow clipboard access, or use Ctrl+Shift+V");
    }
  }, [message]);

  const copyRef = useRef(copySelection);
  const pasteRef = useRef(pasteClipboard);
  copyRef.current = copySelection;
  pasteRef.current = pasteClipboard;

  useEffect(() => {
    if (!host.current || !user) return;
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
      theme: {
        background: "#0f172a",
        foreground: "#e2e8f0",
        cursor: "#a5b4fc",
        selectionBackground: "#4f46e580",
        black: "#0f172a",
        red: "#f87171",
        green: "#4ade80",
        yellow: "#facc15",
        blue: "#818cf8",
        magenta: "#c084fc",
        cyan: "#22d3ee",
        white: "#e2e8f0",
        brightBlack: "#64748b",
        brightRed: "#fca5a5",
        brightGreen: "#86efac",
        brightYellow: "#fde047",
        brightBlue: "#a5b4fc",
        brightMagenta: "#d8b4fe",
        brightCyan: "#67e8f9",
        brightWhite: "#f8fafc",
      },
      scrollback: 5000,
      allowProposedApi: false,
      macOptionIsMeta: true,
      rightClickSelectsWord: false,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    termRef.current = term;
    try {
      fit.fit();
    } catch {
      /* host may not be laid out yet */
    }

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const q = new URLSearchParams({
      user,
      cols: String(term.cols || 120),
      rows: String(term.rows || 32),
    });
    if (user === "root") q.set("root", "1");
    if (cwd) q.set("cwd", cwd);
    const ws = new WebSocket(`${proto}://${location.host}/api/term?${q.toString()}`);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;
    setStatus("connecting");

    const sendJSON = (payload: object) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
    };

    ws.onopen = () => {
      setStatus("online");
      sendJSON({ type: "resize", cols: term.cols, rows: term.rows });
    };
    ws.onclose = () => {
      setStatus("offline");
      term.write("\r\n\x1b[90m[session closed]\x1b[0m\r\n");
    };
    ws.onerror = () => setStatus("offline");
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") term.write(ev.data);
      else term.write(new Uint8Array(ev.data as ArrayBuffer));
    };

    const dataSub = term.onData((d) => sendJSON({ type: "in", data: d }));
    term.attachCustomKeyEventHandler((ev) => {
      if (ev.type !== "keydown") return true;
      const mod = ev.ctrlKey || ev.metaKey;
      if (mod && ev.shiftKey && ev.key.toLowerCase() === "c") {
        ev.preventDefault();
        void copyRef.current();
        return false;
      }
      if (mod && !ev.shiftKey && ev.key.toLowerCase() === "c" && term.hasSelection()) {
        ev.preventDefault();
        void copyRef.current();
        return false;
      }
      if (mod && ev.key.toLowerCase() === "v") {
        ev.preventDefault();
        void pasteRef.current();
        return false;
      }
      if (ev.shiftKey && ev.key === "Insert") {
        ev.preventDefault();
        void pasteRef.current();
        return false;
      }
      return true;
    });

    const onResize = () => {
      try {
        fit.fit();
      } catch {
        return;
      }
      sendJSON({ type: "resize", cols: term.cols, rows: term.rows });
    };
    const ro = new ResizeObserver(() => onResize());
    ro.observe(host.current);
    window.addEventListener("resize", onResize);
    const ping = window.setInterval(() => sendJSON({ type: "ping" }), 25000);
    const ready = window.setTimeout(onResize, 50);

    return () => {
      window.clearTimeout(ready);
      window.clearInterval(ping);
      window.removeEventListener("resize", onResize);
      ro.disconnect();
      dataSub.dispose();
      ws.close();
      term.dispose();
      termRef.current = null;
      wsRef.current = null;
    };
  }, [user, cwd]);

  return (
    <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, gap: 8 }}>
      <Space wrap>
        <Tag color={status === "online" ? "success" : status === "connecting" ? "processing" : "default"}>
          {status === "online" ? "Connected" : status === "connecting" ? "Connecting" : "Disconnected"}
        </Tag>
        {cwd ? <Tag>{cwd}</Tag> : null}
        <Button size="small" icon={<CopyOutlined />} onClick={() => void copySelection()}>
          Copy
        </Button>
        <Button size="small" icon={<SnippetsOutlined />} onClick={() => void pasteClipboard()}>
          Paste
        </Button>
        <Typography.Text type="secondary">
          Ctrl+Shift+C / Ctrl+Shift+V · select then Ctrl+C copies · right-click Copy/Paste
        </Typography.Text>
      </Space>
      <Dropdown
        trigger={["contextMenu"]}
        menu={{
          items: [
            { key: "copy", label: "Copy", onClick: () => void copySelection() },
            { key: "paste", label: "Paste", onClick: () => void pasteClipboard() },
          ],
        }}
      >
        <div
          ref={host}
          style={{
            flex: 1,
            minHeight: 360,
            background: "#0f172a",
            borderRadius: 8,
            padding: 10,
            overflow: "hidden",
          }}
        />
      </Dropdown>
    </div>
  );
}
