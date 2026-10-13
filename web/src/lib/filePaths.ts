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

export function parentFilePath(path: string, jail?: string) {
  const cur = (path || "/").replace(/\/+$/, "") || "/";
  const parts = cur.split("/").filter(Boolean);
  parts.pop();
  const next = "/" + parts.join("/");
  const root = (jail || "").replace(/\/+$/, "");
  if (root && (cur === root || (next !== root && !next.startsWith(root + "/")))) return "/";
  return next || "/";
}

export function fileCrumbs(path: string, jail?: string) {
  const cur = (path || "/").replace(/\/+$/, "") || "/";
  const root = (jail || "").replace(/\/+$/, "");
  const items: { title: string; path: string }[] = [{ title: "Home directory", path: "/" }];
  if (root && (cur === root || cur.startsWith(root + "/"))) {
    items.push({ title: root, path: root });
    let at = root;
    for (const part of cur.slice(root.length).split("/").filter(Boolean)) {
      at += "/" + part;
      items.push({ title: part, path: at });
    }
    return items;
  }
  let at = "";
  for (const part of cur.split("/").filter(Boolean)) {
    at += "/" + part;
    items.push({ title: part, path: at });
  }
  return items;
}

export function accountAbs(home: string, path: string, jail?: string) {
  const base = home.replace(/\/+$/, "") || "/";
  const p = (path || "/").replace(/\/+$/, "") || "/";
  if (p === "/") return base;
  const root = (jail || "").replace(/\/+$/, "");
  if (root && (p === root || p.startsWith(root + "/"))) return p;
  return p.startsWith("/") ? `${base}${p}` : `${base}/${p}`;
}

export function moveDestinations(items: { path: string; name: string }[], dest: string) {
  dest = dest.trim().replace(/\/+$/, "") || "/";
  if (!items.length) return [];
  if (items.length === 1) {
    return [{ path: items[0].path, dest }];
  }
  return items.map((e) => ({ path: e.path, dest: joinPath(dest, e.name) }));
}
