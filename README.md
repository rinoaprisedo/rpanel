# RPanel

A lightweight web panel for Ubuntu/Debian servers: dashboard, websites (Nginx + Let's Encrypt),
file manager, web terminal, databases (MariaDB/PostgreSQL), Docker, cron and firewall (UFW).

## Install

Build a release on your development machine (needs Go, Node/npm and `swag`):

```bash
./deploy/build-release.sh amd64        # or arm64
scp dist/rpanel-*-linux-amd64.tar.gz root@your-server:/root/
```

On the server (Ubuntu 22.04+/Debian 12+, as root):

```bash
tar xzf rpanel-*.tar.gz && cd rpanel-*/
./deploy/install.sh
```

The installer installs nginx, PostgreSQL, Redis and certbot, creates the database, generates every
secret, writes `/opt/rpanel/.env`, starts the `rpanel` systemd service and puts nginx in front of it.
It prints the access URL and the generated administrator password (also saved in
`/opt/rpanel/install-info.txt`, readable by root only — delete it after storing the password).

Options (environment variables): `PANEL_PORT`, `ADMIN_EMAIL`, `PUBLIC_HOST`, `PANEL_ALLOWED_IPS`, `INSTALL_DIR`.
Running the installer again on an installed system upgrades the binary and web files and keeps all secrets.
`deploy/uninstall.sh [--purge]` removes the service (and with `--purge` the panel database and `/opt/rpanel`).

## Security model

The panel runs as **root** because it manages nginx, ufw, docker, cron, databases and the filesystem.
Treat panel access as root access to the server.

| Layer | What it does |
|-------|--------------|
| Secret entrance | nginx answers 404 to everything until you open `https://host:port/<random>` once, which sets a cookie. |
| TLS | nginx terminates TLS on the panel port (self-signed at first — install your own certificate for a domain). |
| IP allow-list | `PANEL_ALLOWED_IPS` in `/opt/rpanel/.env` (IPs/CIDRs, comma separated) rejects every other address in the backend. |
| 2FA (TOTP) | Enable it in *user menu → Security*. Recovery codes are shown once. Codes cannot be replayed. |
| Login protection | Per-IP rate limit and lockout after repeated failures; sessions in Redis; CSRF tokens; `HttpOnly`/`Strict` cookies. |
| Trusted proxies | The backend only believes `X-Forwarded-For` from `TRUSTED_PROXIES` (the local nginx), so client IPs cannot be spoofed. |
| Least exposure | The backend listens on `127.0.0.1:8800` only; Redis requires a password; the database role is panel-only. |
| Guardrails | The firewall refuses changes that would block SSH, the panel port or your own IP unless you confirm; nginx/UFW changes are validated and rolled back on failure. |
| Audit | File changes, terminal sessions, cron edits, firewall/database/docker/website actions are recorded in *Activity Logs*. |

Recommended first steps: log in → enable 2FA → enable the firewall (SSH and the panel port are opened automatically)
→ restrict `PANEL_ALLOWED_IPS` if you have a fixed IP (then `systemctl restart rpanel`).

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

Give other users access through *Roles → permissions*. Grant **Terminal**, **File Manager**, **Docker** and
**Cron** only to people you would trust with root.

## Operations

```bash
systemctl status rpanel         # service
journalctl -u rpanel -f         # logs
sudo -u postgres pg_dump rpanel > rpanel-db.sql   # back up the panel's own data (plus /opt/rpanel/.env)
```

## Development

See `CLAUDE.md`. In short: Postgres + Redis via `docker compose -p rpanel up -d postgres redis`,
`cd backend && swag init -g main.go -o docs && go run main.go migrate && go run main.go seed && go run main.go`,
`cd webadmin && npm run dev`. Tests: `cd backend && go test ./...`.
Features that touch the host (nginx, ufw, docker, apt, cron) are unit-tested with fakes; verify them on a real
Ubuntu VM before production use.
