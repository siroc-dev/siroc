document.querySelectorAll("[data-copy]").forEach((btn) => {
  btn.addEventListener("click", async () => {
    const cmd = btn.closest(".cmd")?.querySelector("pre")?.textContent?.trim();
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
});
