# RPanel

A lightweight, self-hosted control panel for Ubuntu / Debian servers. One binary and a web UI to manage
websites, databases, files, containers, scheduled jobs and the firewall, with a live health view of the server.

**Contents:** [Features](#features) · [Requirements](#requirements) · [Install](#install) · [First login](#first-login) ·
[Update](#update) · [Uninstall](#uninstall) · [Security](#security-model) · [Configuration](#configuration) ·
[Operations](#operations) · [Development](#development) · [Releasing](#releasing-maintainers)

## Features

| Module | What you get |
|--------|--------------|
| **Monitor Server** | Server health speedometer (0–100) from CPU, load per core, memory/swap, disk, 5xx rate and stopped services, with a donut per factor. Live CPU (per core), memory breakdown, disk and disk I/O, load, network per interface, TCP connections, system facts, history charts up to 7 days with peaks, storage and inodes, top processes, services, certificates, a *Needs attention* list and recent activity. |
| **Monitor Traffic** | Website visits from the nginx access logs: requests, page views, unique visitors, bandwidth and 5xx rate with comparison to the previous period; requests/visitors/bandwidth over time, busiest hours and weekdays, top pages, referrers and clients, browsers, operating systems, devices, bots vs people, HTTP methods, most requested 404 pages, pages with server errors, per-site table and a live feed of the latest requests. Filter by website and range (1 h – 7 days). |
| **Websites** | Nginx virtual hosts (PHP, static, reverse proxy), Let's Encrypt certificates issued automatically when a site is added (with a live progress dialog; no e-mail needed), force-HTTPS, config validated and rolled back on failure. |
| **File Manager** | Browse, edit (IDE-style editor), upload, download, archive and permissions, restricted per role. |
| **Terminal** | Browser shell over WebSocket with idle timeout and session limits. **Command templates**: save, edit and delete snippets, then *Run* or *Insert* them into the shell. |
| **Databases** | Register MariaDB/MySQL and PostgreSQL servers (passwords stored encrypted). Create/drop databases and users, backups and restore, and a phpMyAdmin-style **data browser**: tables, sort/search/paginate rows, insert/edit/delete rows, table structure, and a **SQL console**. |
| **Docker** | Resource overview with live CPU/RAM per container, containers, images and Compose projects, each with search. |
| **Cron** | Shell or URL jobs run through system cron, with run history and output, enable/disable and run-now. |
| **Logs** | Access and error logs of every website in one place: filter by site, keyword and date, colour-coded status codes, auto refresh, export and clear. |
| **Firewall** | UFW rules, default policy, protected ports, lock-out guard. |
| **Fail2ban** | Appears in the sidebar once Fail2ban is installed from the App Store: live jails and banned IPs (ban / unban), ban time, find time, max retries, IP whitelist, SSH and nginx jails, service control and log viewer. |
| **Supervisor** | Appears in the sidebar once Supervisor is installed from the App Store: live state of every process, start/stop/restart, logs, and add/edit/remove programs (workers, bots) with validated configuration that is rolled back if Supervisor rejects it. |
| **Updates** | OS package updates without a terminal: list of pending updates, one-click install of security or all updates with live progress, automatic security updates, restart-required banner and server restart. |
| **App Store** | One-click Nginx, PHP (several versions), MariaDB, PostgreSQL, Redis, Node.js and more. |
| **Users & roles** | Per-module view / create / edit / delete permissions, 2FA (TOTP), activity log, branding (name, logo, color), dark mode. |

## Requirements

- Ubuntu or Debian, amd64 or arm64, a fresh server and root access. Tested on **Ubuntu 24.04 LTS**.
- Do **not** install it next to another panel (aaPanel, BT, Plesk, ...): both would manage nginx, the firewall and cron.
- Outbound internet access (apt, GitHub, Let's Encrypt).
- **A domain** (e.g. `panel.example.com`) whose A record already points at the server, and port 80 reachable: RPanel is served
  over HTTPS with a Let's Encrypt certificate and refuses access by bare IP.

## Install

On the server, as root, one command downloads the latest release, verifies its checksum and installs it:

```bash
curl -fsSL https://raw.githubusercontent.com/rinoaprisedo/rpanel/main/deploy/install-online.sh | sudo PANEL_DOMAIN=panel.example.com bash
```

(Leave the variable out and the installer asks for the domain. No e-mail address is needed for Let's Encrypt.) Before installing anything it checks that the
domain resolves to the server and that port 80 is free, so a wrong DNS record fails immediately instead of half-way.

The installer installs nginx, PostgreSQL, Redis and certbot, creates the panel database, generates every secret,
writes `/opt/rpanel/.env`, starts the `rpanel` systemd service and obtains the Let's Encrypt certificate and puts nginx (HTTPS + secret entrance) in front of it.
It prints the access URL and the generated administrator password (also saved in `/opt/rpanel/install-info.txt`,
readable by root only; delete it after storing the password).

Options are environment variables, e.g. `curl ... | sudo PANEL_DOMAIN=panel.example.com PANEL_PORT=8443 bash`:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PANEL_DOMAIN` | asked | **Required.** Host name of the panel; its A record must point at the server |
| `CERTBOT_EMAIL` | none | Optional e-mail for the Let's Encrypt account; not required |
| `SKIP_DNS_CHECK` | off | `1` = do not require the domain to resolve to this server (NAT, Cloudflare proxy) |
| `PANEL_PORT` | random 20000–59999 | Public HTTPS port of the panel |
| `ADMIN_EMAIL` | `admin@rpanel.local` | Initial administrator |
| `PANEL_ALLOWED_IPS` | any | Comma-separated IPs/CIDRs allowed to reach the panel |
| `INSTALL_DIR` | `/opt/rpanel` | Install location |
| `RPANEL_VERSION` | latest | Install a specific version (or pass it as the first argument) |

**Install from a local build** (no GitHub): on your development machine run `./deploy/build-release.sh arm64`
(or `amd64`; needs Go, Node/npm and `swag`), copy `dist/rpanel-*.tar.gz` to the server, then
`tar xzf rpanel-*.tar.gz && cd rpanel-*/ && sudo ./deploy/install.sh`.

## First login

1. Open the URL the installer printed (`https://<domain>:<port>/<entrance>`) **once**. It sets the entrance cookie;
   until then every page answers 404. The certificate is valid (Let's Encrypt), so there is no browser warning.
2. Sign in with the printed e-mail and password.
3. Enable **2FA** (user menu → Security), then the **firewall** (SSH and the panel port are opened automatically).
4. Optionally set `PANEL_ALLOWED_IPS` if you have a fixed IP, and change the app name and color in *Settings*.

## Update

When a newer release is published, superadmins see **Update available** in the top bar (left of the dark-mode button). It opens the
release notes from GitHub with **Update now** and **Skip this version**; the panel updates itself, restarts and reloads, and
the previous version is restored automatically if anything fails. (Servers running 1.3.0 or newer; the very first move to a
version with this button still needs the command below.)

You can also open *Settings → RPanel Update*: **Check for updates** looks for a new release right away, and the tab keeps the
update history (release notes of every published version, the installed one marked).

From a terminal (SSH or the panel's own Terminal; the update continues in the background when started from the panel):

```bash
sudo rpanel-update            # latest release
sudo rpanel-update 1.3.0      # a specific version
```

`rpanel-update` is installed together with the panel. It downloads the release, verifies the checksum and upgrades
in place: secrets, database and websites are kept, and the previous binary and web files are restored automatically
if the upgrade fails (migrations that already ran are additive and stay). The panel is down for a few seconds;
websites keep running because nginx serves them. The Monitor Server page shows *RPanel x.y.z is available* under
**Needs attention** when a newer release exists.

`FORCE=1 sudo rpanel-update` reinstalls the current version.

**Panels installed before 1.3.3** keep working on their IP address with the self-signed certificate; a banner asks you to
set a domain. Do it from the panel (*Settings → Panel SSL*: domain → *Issue Let's Encrypt certificate*), or let
the update do it: `sudo PANEL_DOMAIN=panel.example.com rpanel-update`.
The host name and certificate live in `/opt/rpanel/ssl/panel-tls.conf`, which updates never overwrite.

## Panel domain & HTTPS

*Settings → Panel SSL* shows the domain and certificate and can change them without a terminal: issue a Let's Encrypt
certificate for another domain, or install your own certificate (wildcard, Cloudflare origin, commercial). Once a domain is set,
HTTP on port 80 and plain HTTP on the panel port redirect to HTTPS, requests addressed to the bare IP are dropped, and
HSTS is sent. Renewal is automatic (certbot) and reloads nginx. The panel domain cannot also be used for a website.

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
| TLS | nginx terminates TLS on the panel port with a Let's Encrypt (or your own) certificate for the panel domain; HTTP redirects to HTTPS, access by IP is refused, HSTS is on. |
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
