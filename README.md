# Siroc

Open-source Linux hosting control panel. Each hosting account is a Linux user. Nginx reverse-proxies to Apache; PHP-FPM, Node.js, Python, Go, Rust, and Docker sites run per account.

Public repo: [github.com/siroc-dev/siroc](https://github.com/siroc-dev/siroc) · License: [MIT](LICENSE)

## Install on Ubuntu / Debian

From the website:

```bash
curl -fsSL https://siroc.dev/install.sh | sudo bash
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
  "url": "https://get.siroc.dev/siroc-linux-amd64.tar.gz",
  "sha256": "optional"
}
```

## Website and auto-deploy

The public site lives in `site/` and deploys with **Cloudflare Workers static assets** (`npx wrangler deploy`) to **siroc.dev**. Release tarballs go to R2 bucket **siroc-cp** at **https://get.siroc.dev**.

### One-time Cloudflare setup

In the Cloudflare “Set up your application” form:

| Field | Value |
| --- | --- |
| GitHub | `siroc-dev/siroc` |
| Project name | `siroc` |
| Build command | `bash scripts/stamp-site.sh` |
| Deploy command | `npx wrangler deploy` |

Then:

1. Attach custom domain **siroc.dev** to the Worker (wrangler already routes `siroc.dev` and `www.siroc.dev`).
2. On R2 bucket **siroc-cp**, add custom domain **get.siroc.dev** and allow public reads.
3. GitHub Actions secrets for the release workflow (R2 uses the S3 API, not Wrangler):
   - `CLOUDFLARE_ACCOUNT_ID`
   - `R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY` from Cloudflare → R2 → **Manage R2 API Tokens** → Object Read & Write on `siroc-cp`
   - `CLOUDFLARE_API_TOKEN` is only for Worker deploys, not for uploading packages

### What CI does

| Workflow | Trigger | Result |
| --- | --- | --- |
| Cloudflare Git deploy | push to `main` | publishes `site/` to siroc.dev |
| `.github/workflows/release.yml` | tag `v*` (or manual) | uploads `siroc-linux-amd64.tar.gz` and `latest.json` to `siroc-cp` |

Tag a release after the token can write R2:

```bash
git tag v0.2.0
git push origin v0.2.0
```

In the panel, set the update channel to `https://get.siroc.dev`.

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
- `site` — public website (siroc.dev)
- `docker` — Ubuntu 24.04 + systemd test VPS
- `scripts/install.sh` — production installer
- `scripts/package.sh` — build the Linux tarball
