# RPanel

A lightweight, self-hosted control panel for Ubuntu / Debian servers. One binary and a web UI to manage
websites, databases, files, containers, scheduled jobs and the firewall, with a live health view of the server.

**Contents:** [Features](#features) · [Requirements](#requirements) · [Install](#install) · [First login](#first-login) ·
[Update](#update) · [Uninstall](#uninstall) · [Security](#security-model) · [Configuration](#configuration) ·
[Operations](#operations) · [Development](#development) · [Releasing](#releasing-maintainers)

## Features

| Module | What you get |
|--------|--------------|
| **Dashboard** | Server health speedometer (0–100) from CPU, load per core, memory/swap, disk, 5xx rate and stopped services, with a donut per factor. Live CPU / memory / disk / load, traffic from nginx logs (requests, visitors, errors, top pages), services, a *Needs attention* list and an "update available" notice. |
| **Websites** | Nginx virtual hosts (PHP, static, reverse proxy), Let's Encrypt certificates, force-HTTPS, config validated and rolled back on failure. |
| **File Manager** | Browse, edit (IDE-style editor), upload, download, archive and permissions, restricted per role. |
| **Terminal** | Browser shell over WebSocket with idle timeout and session limits. **Command templates**: save, edit and delete snippets, then *Run* or *Insert* them into the shell. |
| **Databases** | Register MariaDB/MySQL and PostgreSQL servers (passwords stored encrypted). Create/drop databases and users, backups and restore, and a phpMyAdmin-style **data browser**: tables, sort/search/paginate rows, insert/edit/delete rows, table structure, and a **SQL console**. |
| **Docker** | Containers, images and Compose projects. |
| **Cron** | Shell or URL jobs run through system cron, with run history and output, enable/disable and run-now. |
| **Firewall** | UFW rules, default policy, protected ports, lock-out guard. |
| **App Store** | One-click Nginx, PHP (several versions), MariaDB, PostgreSQL, Redis, Node.js and more. |
| **Users & roles** | Per-module view / create / edit / delete permissions, 2FA (TOTP), activity log, branding (name, logo, color), dark mode. |

## Requirements

- Ubuntu or Debian, amd64 or arm64, a fresh server and root access. Tested on **Ubuntu 24.04 LTS**.
- Do **not** install it next to another panel (aaPanel, BT, Plesk, ...): both would manage nginx, the firewall and cron.
- Outbound internet access (apt, GitHub, Let's Encrypt).

## Install

On the server, as root, one command downloads the latest release, verifies its checksum and installs it:

```bash
curl -fsSL https://raw.githubusercontent.com/rinoaprisedo/rpanel/main/deploy/install-online.sh | sudo bash
```

The installer installs nginx, PostgreSQL, Redis and certbot, creates the panel database, generates every secret,
writes `/opt/rpanel/.env`, starts the `rpanel` systemd service and puts nginx (TLS + secret entrance) in front of it.
It prints the access URL and the generated administrator password (also saved in `/opt/rpanel/install-info.txt`,
readable by root only; delete it after storing the password).

Options are environment variables, e.g. `curl ... | sudo PANEL_PORT=8443 ADMIN_EMAIL=me@example.com bash`:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PANEL_PORT` | random 20000–59999 | Public HTTPS port of the panel |
| `ADMIN_EMAIL` | `admin@rpanel.local` | Initial administrator |
| `PUBLIC_HOST` | first IP of the machine | Host shown in the printed URL |
| `PANEL_ALLOWED_IPS` | any | Comma-separated IPs/CIDRs allowed to reach the panel |
| `INSTALL_DIR` | `/opt/rpanel` | Install location |
| `RPANEL_VERSION` | latest | Install a specific version (or pass it as the first argument) |

**Install from a local build** (no GitHub): on your development machine run `./deploy/build-release.sh arm64`
(or `amd64`; needs Go, Node/npm and `swag`), copy `dist/rpanel-*.tar.gz` to the server, then
`tar xzf rpanel-*.tar.gz && cd rpanel-*/ && sudo ./deploy/install.sh`.

## First login

1. Open the URL the installer printed (`https://<ip>:<port>/<entrance>`) **once**. It sets the entrance cookie;
   until then every page answers 404. The certificate is self-signed, so the browser warns.
2. Sign in with the printed e-mail and password.
3. Enable **2FA** (user menu → Security), then the **firewall** (SSH and the panel port are opened automatically).
4. Optionally set `PANEL_ALLOWED_IPS` if you have a fixed IP, and change the app name and color in *Settings*.

## Update

```bash
sudo rpanel-update            # latest release
sudo rpanel-update 1.3.0      # a specific version
```

`rpanel-update` is installed together with the panel. It downloads the release, verifies the checksum and upgrades
in place: secrets, database and websites are kept, and the previous binary and web files are restored automatically
if the upgrade fails (migrations that already ran are additive and stay). The panel is down for a few seconds;
websites keep running because nginx serves them. The dashboard shows *RPanel x.y.z is available* under
**Needs attention** when a newer release exists.

`FORCE=1 sudo rpanel-update` reinstalls the current version.

## Uninstall

```bash
sudo rpanel-uninstall            # remove the service and nginx site, keep data
sudo rpanel-uninstall --purge    # also drop the panel database and /opt/rpanel
```

Websites, databases you registered and packages installed through the App Store are not removed.
(`rpanel-uninstall` is installed by installs and updates from 1.3.0 on; older installs: use `deploy/uninstall.sh` from the release tarball.)

## Security model

The panel runs as **root** because it manages nginx, ufw, docker, cron, databases and the filesystem.
Treat panel access as root access to the server.

| Layer | What it does |
|-------|--------------|
| Secret entrance | nginx answers 404 to everything until you open `https://host:port/<random>` once, which sets a cookie. |
| TLS | nginx terminates TLS on the panel port (self-signed at first; install your own certificate for a domain). |
| IP allow-list | `PANEL_ALLOWED_IPS` in `/opt/rpanel/.env` (IPs/CIDRs, comma separated) rejects every other address in the backend. |
| 2FA (TOTP) | Enable it in *user menu → Security*. Recovery codes are shown once. Codes cannot be replayed. |
| Login protection | Per-IP rate limit and lockout after repeated failures; sessions in Redis; CSRF tokens; `HttpOnly`/`Strict` cookies. |
| Trusted proxies | The backend only believes `X-Forwarded-For` from `TRUSTED_PROXIES` (the local nginx), so client IPs cannot be spoofed. |
| Least exposure | The backend listens on `127.0.0.1:8800` only; Redis requires a password; the database role is panel-only. |
| Guardrails | The firewall refuses changes that would block SSH, the panel port or your own IP unless you confirm; nginx/UFW changes are validated and rolled back on failure. Database browsing blocks system databases and validates table/column names against the catalog. |
| Audit | File changes, terminal sessions, template edits, cron edits, firewall/database/docker/website actions and SQL console use are recorded in *Activity Logs* (the SQL console logs only the statement type, never its text). |
| Releases | Downloads are verified against a published SHA-256 before anything is installed. |

Give other users access through *Roles → permissions*. Grant **Terminal**, **File Manager**, **Docker**, **Cron**
and the data/SQL part of **Databases** (edit permission) only to people you would trust with root.

### Locked out?

```bash
cd /opt/rpanel
./rpanel disable-2fa you@example.com                                   # remove 2FA for a user
RPANEL_ADMIN_EMAIL=you@example.com RPANEL_ADMIN_PASSWORD='New-Pass-1!' ./rpanel set-admin   # reset the admin credentials
cat install-info.txt                                                   # access URL (entrance path), if you kept it
grep -E 'PANEL_ALLOWED_IPS' .env                                       # clear it if your IP changed, then: systemctl restart rpanel
```
The entrance path is the random path in `/etc/nginx/sites-available/rpanel-panel.conf` (`location = /...`).

## Configuration

Everything lives in `/opt/rpanel/.env` (see `backend/.env.example` for all variables). Important ones:

| Variable | Purpose |
|----------|---------|
| `SECRET_KEY` | Encrypts stored database admin passwords and 2FA secrets. **Back it up**; losing it invalidates them. |
| `FM_ROOTS` | Directories non-superadmin users may browse in the file manager (superadmin: everything). |
| `WEB_ROOT`, `BACKUP_DIR`, `COMPOSE_DIR`, `CRON_DATA_DIR` | Where sites, DB backups, compose projects and cron scripts live. |
| `TERMINAL_*` | Shell, idle timeout and session limit of the web terminal. |
| `FIREWALL_PROTECTED_PORTS` | Extra ports the firewall guard must never block. |
| `PANEL_ALLOWED_IPS` | IPs/CIDRs allowed to use the panel. |

After editing `.env`: `systemctl restart rpanel`. The app name and base color are changed in the UI (*Settings*).

## Operations

```bash
systemctl status rpanel         # service
journalctl -u rpanel -f         # logs
sudo -u postgres pg_dump rpanel > rpanel-db.sql   # back up the panel's own data (plus /opt/rpanel/.env)
```

Files: binary and web UI in `/opt/rpanel`, websites in `/www/wwwroot`, database dumps in `/www/backup/database`,
cron scripts in `/www/cron`, Compose projects in `/www/docker`.

## Development

See `CLAUDE.md`. In short: Postgres + Redis via `docker compose -p rpanel up -d postgres redis`,
`cd backend && swag init -g main.go -o docs && go run main.go migrate && go run main.go seed && go run main.go`,
`cd webadmin && npm run dev`. Tests: `cd backend && go test ./...` and `cd webadmin && npm test`.
Features that touch the host (nginx, ufw, docker, apt, cron) are unit-tested with fakes; verify them on a real
Ubuntu VM before production use.

## Releasing (maintainers)

Bump `AppVersion` (and `@version`) in `backend/main.go`, commit and push to `main`, then:

```bash
./deploy/release.sh --notes-file CHANGES.md     # needs gh (logged in), Go, Node, swag
```

It builds amd64 and arm64 tarballs, writes their `.sha256` files and creates the GitHub release `vX.Y.Z` with
`install-online.sh` attached. Forks can set `RPANEL_REPO=owner/name`; a plain web server can serve the release files
through `RPANEL_MIRROR=https://host/path` (with an explicit version).

---

<p align="center">Develop by <a href="https://rinoaprisedo.com">rinoaprisedo.com</a></p>
