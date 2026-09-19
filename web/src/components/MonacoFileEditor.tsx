import { useEffect, useRef } from "react";
import * as monaco from "monaco-editor";
import { lsp } from "monaco-editor";
import { ensureMonacoWorkers } from "@/lib/monaco";

const lspLangs = new Set(["php", "yaml", "python", "javascript", "typescript", "html", "css", "json", "shell", "dockerfile"]);

export function MonacoFileEditor({
  path,
  absPath,
  workspace,
  user,
  value,
  language,
  onChange,
  onSave,
  onLsp,
}: {
  path: string;
  absPath?: string;
  workspace?: string;
  user: string;
  value: string;
  language: string;
  onChange: (value: string) => void;
  onSave: () => void;
  onLsp?: (connected: boolean) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  const onLspRef = useRef(onLsp);
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;
  onLspRef.current = onLsp;

  useEffect(() => {
    ensureMonacoWorkers();
    if (!host.current) return;
    const uri = absPath ? monaco.Uri.parse("file://" + absPath) : monaco.Uri.parse("inmemory://model/" + encodeURIComponent(path));
    const existing = monaco.editor.getModel(uri);
    existing?.dispose();
    const model = monaco.editor.createModel(value, language, uri);
    const editor = monaco.editor.create(host.current, {
      model,
      automaticLayout: true,
      minimap: { enabled: false },
      fontSize: 13,
      scrollBeyondLastLine: false,
      wordWrap: "on",
      theme: "vs",
      padding: { top: 8 },
    });
    editorRef.current = editor;
    const sub = editor.onDidChangeModelContent(() => onChangeRef.current(editor.getValue()));
    const saveAction = editor.addAction({
      id: "siroc-save-file",
      label: "Save file",
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
      run: () => onSaveRef.current(),
    });

    let transport: { dispose: () => void } | null = null;
    let cancelled = false;
    async function connectLsp() {
      if (!lspLangs.has(language) || !workspace) {
        onLspRef.current?.(false);
        return;
      }
      try {
        const proto = location.protocol === "https:" ? "wss" : "ws";
        const q = new URLSearchParams({ user, lang: language, workspace });
        if (user === "root") q.set("root", "1");
        const t = await lsp.WebSocketTransport.connectTo({ address: `${proto}://${location.host}/api/files/lsp?${q.toString()}` });
        if (cancelled) {
          t.dispose();
          return;
        }
        transport = t;
        new lsp.MonacoLspClient(t);
        onLspRef.current?.(true);
      } catch {
        onLspRef.current?.(false);
      }
    }
    void connectLsp();

    return () => {
      cancelled = true;
      transport?.dispose();
      sub.dispose();
      saveAction.dispose();
      editor.dispose();
      model.dispose();
      editorRef.current = null;
      onLspRef.current?.(false);
    };
  }, [path, absPath, language, user, workspace]);

  return <div ref={host} style={{ height: "100%", minHeight: 480, border: "1px solid #f0f0f0", borderRadius: 8 }} />;
}
