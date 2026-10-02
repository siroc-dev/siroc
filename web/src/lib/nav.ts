export const ADMIN_ONLY_PATHS = new Set([
  "/accounts",
  "/software",
  "/security",
  "/scan-logs",
  "/tools",
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
