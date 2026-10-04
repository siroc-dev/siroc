import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { api } from "@/lib/api";

const FONT = 'Consolas, "Liberation Mono", Menlo, Monaco, "Courier New", monospace';

export function InstallLog({ name, live, endpoint }: { name: string; live: boolean; endpoint?: string }) {
  const host = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const offset = useRef(0);

  useEffect(() => {
    if (!host.current || !name) return;
    const term = new Terminal({
      disableStdin: true,
      cursorBlink: live,
      fontSize: 13,
      fontFamily: FONT,
      scrollback: 8000,
      theme: {
        background: "#0f172a",
        foreground: "#e2e8f0",
        cursor: "#a5b4fc",
        selectionBackground: "#4f46e580",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    fit.fit();
    termRef.current = term;
    const onResize = () => fit.fit();
    window.addEventListener("resize", onResize);
    offset.current = 0;
    let stop = false;
    const path = endpoint || `/api/software/logs/${encodeURIComponent(name)}`;
    const join = path.includes("?") ? "&" : "?";
    async function pull() {
      if (stop) return;
      try {
        const data = await api.get<{ text: string; offset: number }>(`${path}${join}offset=${offset.current}`);
        if (stop || !data) return;
        if (data.text) term.write(data.text.replace(/\r?\n/g, "\r\n"));
        if (typeof data.offset === "number") offset.current = data.offset;
      } catch {
        /* the next poll retries */
      }
    }
    const start = window.setTimeout(() => void pull(), 50);
    const timer = window.setInterval(() => void pull(), 1000);
    return () => {
      stop = true;
      window.clearTimeout(start);
      window.clearInterval(timer);
      window.removeEventListener("resize", onResize);
      term.dispose();
      termRef.current = null;
    };
  }, [name, endpoint]);

  if (!name) {
    return (
      <div style={{ height: 280, borderRadius: 8, background: "#0f172a", color: "#94a3b8", padding: 16, fontFamily: FONT }}>
        Queue a package to follow its install log here. Saved logs stay on the server for each package.
      </div>
    );
  }
  return <div ref={host} style={{ height: 320, borderRadius: 8, overflow: "hidden", background: "#0f172a" }} />;
}
