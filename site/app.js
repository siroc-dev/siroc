const RELEASE_BASE = "https://get.siroc.dev";
const GITHUB = "https://github.com/siroc-dev/siroc";

const versionEl = document.getElementById("version");
const downloadEl = document.getElementById("download");
const channelEl = document.getElementById("channel");

if (channelEl && RELEASE_BASE && !RELEASE_BASE.includes("__SIROC")) {
  channelEl.textContent = RELEASE_BASE;
}

if (downloadEl && RELEASE_BASE && !RELEASE_BASE.includes("__SIROC")) {
  downloadEl.href = `${RELEASE_BASE.replace(/\/$/, "")}/siroc-linux-amd64.tar.gz`;
}

async function loadLatest() {
  if (!RELEASE_BASE || RELEASE_BASE.includes("__SIROC")) return;
  try {
    const res = await fetch(`${RELEASE_BASE.replace(/\/$/, "")}/latest.json`, { cache: "no-store" });
    if (!res.ok) return;
    const meta = await res.json();
    if (meta.version && versionEl) versionEl.textContent = meta.version;
    if (meta.url && downloadEl) downloadEl.href = meta.url;
  } catch {
    /* keep stamped version */
  }
}

document.getElementById("copy-install")?.addEventListener("click", async (e) => {
  const btn = e.currentTarget;
  const cmd = document.getElementById("install-cmd")?.textContent?.trim();
  if (!cmd) return;
  try {
    await navigator.clipboard.writeText(cmd);
    btn.textContent = "Copied";
    setTimeout(() => {
      btn.textContent = "Copy";
    }, 1400);
  } catch {
    btn.textContent = "Copy failed";
  }
});

void loadLatest();
void GITHUB;
