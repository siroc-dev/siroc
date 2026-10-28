# Siroc

Open-source Linux hosting control panel. Each hosting account is a Linux user. Nginx reverse-proxies to Apache; PHP-FPM, Node.js, Python, Go, Rust, and Docker sites run per account.

Public repo: [github.com/siroc-dev/siroc](https://github.com/siroc-dev/siroc) · License: [Apache License 2.0](LICENSE)

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

Open that link to create the first panel admin and a Let's Encrypt account email. Visiting the panel without the token does not show the setup form. The link is also saved at `/var/lib/siroc/setup.url` until setup completes. If Certbot is already installed, the ACME account is registered during setup; otherwise it is created when you install Certbot or click **Create Let's Encrypt account**.

If binaries are already built in this tree (`bin/siroc-agent`, `bin/siroc-panel`, `web/dist`), you can run `sudo ./scripts/install.sh` directly.

### Reset the admin password

On the server, as root:

```bash
sudo siroc passwd-admin
# or: sudo siroc-panel passwd-admin [username] [password]
```

A random password is printed if you omit one. Existing admin sessions are signed out.

### Update the panel

On the server:

```bash
sudo siroc update              # check https://get.siroc.dev
sudo siroc upgrade             # download, install, restart
```

Or open **System tools → Updates**. The current version is shown in the sidebar.

- **Check for updates** reads `latest.json` from an HTTPS channel (`SIROC_UPDATE_URL` or the channel field). When automatic updates are on, the agent checks every hour and installs a newer version. Turn that off under **System tools → Updates**. Check and Apply still work either way.
- **Apply update** installs a `siroc-linux-amd64.tar.gz` from that URL, or from a local path such as `/var/lib/siroc/updates/…`.
- Sites, accounts, and `/var/lib/siroc` are kept. The agent replaces binaries and the web UI, then restarts the services.

Example `latest.json` (published to Cloudflare R2 by CI):

```json
{
  "version": "0.3.21",
  "url": "https://get.siroc.dev/0.3.21/siroc-linux-amd64.tar.gz",
  "sha256": "optional"
}
```

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
