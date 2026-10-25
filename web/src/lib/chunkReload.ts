const reloadKey = "siroc-chunk-reload";

export function shouldReloadMissingChunk(lastReloadAt: number, now: number) {
  if (!lastReloadAt) return true;
  return now - lastReloadAt >= 10_000;
}

export function installChunkReload() {
  window.addEventListener("vite:preloadError", (event) => {
    event.preventDefault();
    const last = Number(sessionStorage.getItem(reloadKey) || 0);
    const now = Date.now();
    if (!shouldReloadMissingChunk(last, now)) return;
    sessionStorage.setItem(reloadKey, String(now));
    window.location.reload();
  });
}
