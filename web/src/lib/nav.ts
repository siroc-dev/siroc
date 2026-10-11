export const ADMIN_ONLY_PATHS = new Set([
  "/accounts",
  "/software",
  "/security",
  "/tools",
  "/os",
  "/settings",
  "/monitoring",
  "/apache",
  "/nginx",
  "/php-fpm",
  "/mysql",
  "/mariadb",
  "/redis",
]);

export function canUsePath(admin: boolean | undefined, path: string) {
  return !!admin || !ADMIN_ONLY_PATHS.has(path);
}

export function spaClick(to: string, go: (path: string) => void) {
  return (e: {
    defaultPrevented?: boolean;
    metaKey: boolean;
    ctrlKey: boolean;
    shiftKey: boolean;
    altKey: boolean;
    button?: number;
    preventDefault: () => void;
  }) => {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || (e.button ?? 0) !== 0) return;
    e.preventDefault();
    go(to);
  };
}
