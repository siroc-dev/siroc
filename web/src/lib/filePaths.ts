const systemRoots = ["/etc", "/usr", "/var", "/opt", "/root", "/bin", "/sbin", "/lib", "/lib64", "/boot", "/tmp", "/run", "/home"];

export function normalizeFileJump(raw: string) {
  let path = raw.trim().replace(/\\/g, "/");
  if (!path || path === "/") return "/";
  if (!path.startsWith("/")) path = "/" + path;
  const parts: string[] = [];
  for (const part of path.split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") {
      parts.pop();
      continue;
    }
    parts.push(part);
  }
  return parts.length ? "/" + parts.join("/") : "/";
}

export function isSystemPath(path: string) {
  return systemRoots.some((root) => path === root || path.startsWith(root + "/"));
}

export function joinPath(dir: string, name: string) {
  const base = dir.replace(/\/+$/, "");
  const leaf = name.replace(/^\/+/, "");
  if (!base || base === "/") return "/" + leaf;
  return `${base}/${leaf}`;
}

export function moveDestinations(items: { path: string; name: string }[], dest: string) {
  dest = dest.trim().replace(/\/+$/, "") || "/";
  if (!items.length) return [];
  if (items.length === 1) {
    return [{ path: items[0].path, dest }];
  }
  return items.map((e) => ({ path: e.path, dest: joinPath(dest, e.name) }));
}
