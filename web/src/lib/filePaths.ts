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
