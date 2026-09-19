export function languageFromPath(path: string) {
  const name = path.toLowerCase().split("/").pop() || "";
  if (name === "dockerfile" || name.endsWith(".dockerfile")) return "dockerfile";
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  switch (ext) {
    case "php":
    case "phtml":
      return "php";
    case "js":
    case "mjs":
    case "cjs":
      return "javascript";
    case "ts":
    case "tsx":
    case "jsx":
      return "typescript";
    case "json":
    case "jsonc":
      return "json";
    case "css":
    case "scss":
    case "less":
      return "css";
    case "html":
    case "htm":
      return "html";
    case "md":
    case "markdown":
      return "markdown";
    case "py":
      return "python";
    case "sh":
    case "bash":
      return "shell";
    case "xml":
    case "svg":
      return "xml";
    case "yml":
    case "yaml":
      return "yaml";
    case "sql":
      return "sql";
    case "ini":
    case "conf":
    case "cnf":
      return "ini";
    case "go":
      return "go";
    case "rs":
      return "rust";
    case "java":
      return "java";
    case "rb":
      return "ruby";
    default:
      return "plaintext";
  }
}
