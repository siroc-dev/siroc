# Siroc

Open-source Linux hosting control panel. Each hosting account is a Linux user. Nginx reverse-proxies to Apache; PHP-FPM, Node.js, Python, Go, Rust, and Docker sites run per account.

Public repo: [github.com/siroc-dev/siroc](https://github.com/siroc-dev/siroc)

## Install on Ubuntu / Debian

From the website (after Cloudflare Pages + R2 are connected):

```bash
curl -fsSL https://siroc.pages.dev/install.sh | sudo bash
```

Or build a package from this repo:

```bash
./scripts/package.sh
# writes dist/siroc-linux-amd64.tar.gz
```

On the server:

```bash
tar -xzf siroc-linux-amd64.tar.gz
cd siroc
sudo ./install.sh
```

The installer prints a **one-time, token-protected** URL:

`https://<server-ip>:8443/setup?token=<secret>`

Open that link to create the first panel admin. Visiting the panel without the token does not show the setup form. The link is also saved at `/var/lib/siroc/setup.url` until setup completes.

If binaries are already built in this tree (`bin/siroc-agent`, `bin/siroc-panel`, `web/dist`), you can run `sudo ./scripts/install.sh` directly.

### Update the panel

After install, open **System tools → Updates**. The current version is shown in the sidebar.

- **Check for updates** reads `latest.json` from an HTTPS channel (`SIROC_UPDATE_URL` or the channel field).
- **Apply update** installs a `siroc-linux-amd64.tar.gz` from that URL, or from a local path such as `/var/lib/siroc/updates/…`.
- Sites, accounts, and `/var/lib/siroc` are kept. The agent replaces binaries and the web UI, then restarts the services.

Example `latest.json` (published to Cloudflare R2 by CI):

```json
{
  "version": "0.2.1",
  "url": "https://releases.siroc.dev/siroc-linux-amd64.tar.gz",
  "sha256": "optional"
}
```

## Website and auto-deploy

The public site lives in `site/` and deploys with **Cloudflare Workers static assets** (`npx wrangler deploy`). Release tarballs go to **Cloudflare R2**.

### One-time Cloudflare setup

In the Cloudflare “Set up your application” form:

| Field | Value |
| --- | --- |
| GitHub | `siroc-dev/siroc` |
| Project name | `siroc` |
| Build command | `bash scripts/stamp-site.sh` |
| Deploy command | `npx wrangler deploy` |

Then:

1. Create an R2 bucket named `siroc-releases` and allow public reads (R2.dev or `releases.siroc.dev`).
2. Optional GitHub Actions secrets if you also use the workflows:
   - `CLOUDFLARE_API_TOKEN` — **Workers Scripts Edit** and **R2 Admin Read & Write**
   - `CLOUDFLARE_ACCOUNT_ID`
   - Variable `SIROC_RELEASE_PUBLIC_BASE` — public bucket URL
   - Variable `SIROC_SITE_PUBLIC_BASE` — Worker URL, e.g. `https://siroc.<account>.workers.dev`

### What CI does

| Workflow | Trigger | Result |
| --- | --- | --- |
| Cloudflare Git deploy | push to `main` | `npx wrangler deploy` publishes `site/` |
| `.github/workflows/release.yml` | tag `v*` (or manual) | builds `siroc-linux-amd64.tar.gz`, uploads it and `latest.json` to R2 |

Tag a release after the secrets exist:

```bash
git tag v0.2.0
git push origin v0.2.0
```

In the panel, set the update channel to the same public R2 URL.

## Test with Docker

Requires Docker Desktop (WSL2 on Windows).

```bash
docker compose up --build
```

Wait until bootstrap finishes (first start compiles Go + the web UI), then open the setup link printed in the bootstrap log (`docker compose logs vps`). First-run setup is token-protected.

Host ports: panel `8443`, nginx `8080` (maps to container 80), TLS `8444` (maps to 443).

1. Open the first-run setup link and create the panel admin
2. Software → install nginx, Apache, PHP, then MySQL **or** MariaDB
3. Accounts → create a Linux user
4. Websites → add a domain (for local tests add `127.0.0.1 site.test` to your hosts file)
5. Files / Databases as needed

Rebuild the image after code changes (`docker compose up --build`). Source is copied into the image because Docker Desktop on Windows can hang on bind-mounts of this folder.

## Layout

- `cmd/panel` — HTTP UI/API (non-root)
- `cmd/agent` — privileged Unix-socket RPC (root)
- `web` — React + Vite control panel
- `site` — public website (Cloudflare Pages)
- `docker` — Ubuntu 24.04 + systemd test VPS
- `scripts/install.sh` — production installer
- `scripts/package.sh` — build the Linux tarball
