#!/usr/bin/env bash
# RPanel installer for Ubuntu / Debian. Run as root from the extracted release directory:
#
#   sudo ./deploy/install.sh
#
# Optional environment variables:
#   PANEL_PORT=<port>       public HTTPS port of the panel (default: random 20000-59999)
#   ADMIN_EMAIL=<email>     initial administrator (default: admin@rpanel.local)
#   PUBLIC_HOST=<host/ip>   host shown in the printed URL (default: first IP of this machine)
#   PANEL_ALLOWED_IPS=<ip,cidr,...>   restrict the panel to these addresses
#   INSTALL_DIR=/opt/rpanel
#
# Re-running on an installed system upgrades the binary and web files and keeps all secrets.
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/rpanel}"
BACKEND_PORT=8800
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }
rand() { openssl rand -hex "$1"; }

[ "$(id -u)" -eq 0 ] || die "run as root (sudo ./deploy/install.sh)"
[ -f /etc/os-release ] || die "cannot detect the operating system"
# shellcheck disable=SC1091
. /etc/os-release
case "${ID:-}${ID_LIKE:-}" in
  *ubuntu*|*debian*) ;;
  *) die "only Ubuntu and Debian are supported (found: ${PRETTY_NAME:-unknown})" ;;
esac
[ -x "$SRC/rpanel" ] || die "binary $SRC/rpanel not found; run this script from an extracted release (see deploy/build-release.sh)"
[ -f "$SRC/web/index.html" ] || die "frontend $SRC/web not found"
case "$INSTALL_DIR" in
  /*[!A-Za-z0-9._/-]*|"") die "INSTALL_DIR may only contain letters, digits and . _ / -" ;;
  /*) ;;
  *) die "INSTALL_DIR must be an absolute path" ;;
esac

render() { # render <template> <dest>  (placeholders @NAME@)
  sed -e "s|@INSTALL_DIR@|$INSTALL_DIR|g" \
      -e "s|@PANEL_PORT@|${PANEL_PORT:-}|g" \
      -e "s|@BACKEND_PORT@|$BACKEND_PORT|g" \
      -e "s|@ENTRANCE@|${ENTRANCE:-}|g" \
      -e "s|@TOKEN@|${COOKIE_TOKEN:-}|g" "$1" > "$2"
}

install_files() {
  log "Installing files into $INSTALL_DIR"
  install -d -m 755 "$INSTALL_DIR" "$INSTALL_DIR/storage"
  install -m 755 "$SRC/rpanel" "$INSTALL_DIR/rpanel.new"
  mv -f "$INSTALL_DIR/rpanel.new" "$INSTALL_DIR/rpanel"   # atomic swap, safe while the old binary runs
  rm -rf "$INSTALL_DIR/web.new"
  cp -R "$SRC/web" "$INSTALL_DIR/web.new"
  rm -rf "$INSTALL_DIR/web"
  mv "$INSTALL_DIR/web.new" "$INSTALL_DIR/web"
  install -d -m 755 /www/wwwroot /var/www/letsencrypt
  install -d -m 700 /www/backup/database /www/cron
  install -d -m 755 /www/docker
  render "$SRC/deploy/rpanel.service" /etc/systemd/system/rpanel.service
  systemctl daemon-reload
  [ -f "$SRC/deploy/install-online.sh" ] && install -m 755 "$SRC/deploy/install-online.sh" /usr/local/bin/rpanel-update
  return 0
}

wait_healthy() {
  for _ in $(seq 1 40); do
    if curl -fsS "http://127.0.0.1:$BACKEND_PORT/api/v1/version" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  journalctl -u rpanel -n 30 --no-pager >&2 || true
  die "the panel did not become healthy; see the log above"
}

# ----------------------------------------------------------------- upgrade
if [ -f "$INSTALL_DIR/.env" ]; then
  log "Existing installation found: upgrading (secrets are kept)"
  systemctl stop rpanel 2>/dev/null || true
  # Keep the previous build so a failed upgrade can be undone.
  rm -rf "$INSTALL_DIR/web.prev"
  cp -p "$INSTALL_DIR/rpanel" "$INSTALL_DIR/rpanel.prev"
  cp -R "$INSTALL_DIR/web" "$INSTALL_DIR/web.prev"
  # Explicit && chain: set -e is ignored inside an "if" condition.
  if ! ( install_files && cd "$INSTALL_DIR" && ./rpanel migrate && ./rpanel seed && systemctl enable --now rpanel && wait_healthy ); then
    warn "upgrade failed: restoring the previous version"
    systemctl stop rpanel 2>/dev/null || true
    mv -f "$INSTALL_DIR/rpanel.prev" "$INSTALL_DIR/rpanel"
    rm -rf "$INSTALL_DIR/web"; mv "$INSTALL_DIR/web.prev" "$INSTALL_DIR/web"
    systemctl start rpanel || true
    die "upgrade failed and was rolled back (database migrations that already ran are additive and stay)"
  fi
  rm -f "$INSTALL_DIR/rpanel.prev"; rm -rf "$INSTALL_DIR/web.prev"
  log "Upgrade finished"
  exit 0
fi

# ----------------------------------------------------------------- fresh install
log "Installing system packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y nginx postgresql redis-server certbot openssl curl ca-certificates
systemctl enable --now postgresql redis-server nginx

PANEL_PORT="${PANEL_PORT:-$(shuf -i 20000-59999 -n 1)}"
case "$PANEL_PORT" in ''|*[!0-9]*) die "PANEL_PORT must be a number" ;; esac
[ "$PANEL_PORT" -ge 1024 ] && [ "$PANEL_PORT" -le 65535 ] || die "PANEL_PORT must be 1024-65535"
if ss -ltn "( sport = :$PANEL_PORT )" | grep -q LISTEN; then die "port $PANEL_PORT is already in use"; fi
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@rpanel.local}"
PUBLIC_HOST="${PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
PUBLIC_HOST="${PUBLIC_HOST:-localhost}"
ALLOWED_IPS="${PANEL_ALLOWED_IPS:-}"

DB_PASS="$(rand 16)"
REDIS_PASS="$(rand 16)"
SECRET_KEY="$(rand 32)"
ADMIN_PASS="Rp-$(rand 8)-9!"
ENTRANCE="/$(rand 4)"
COOKIE_TOKEN="$(rand 16)"

log "Creating the PostgreSQL role and database"
PSQL=(runuser -u postgres -- psql -v ON_ERROR_STOP=1 -tA)
if [ "$("${PSQL[@]}" -c "SELECT 1 FROM pg_roles WHERE rolname='rpanel'")" = "1" ]; then
  "${PSQL[@]}" -c "ALTER ROLE rpanel LOGIN PASSWORD '$DB_PASS'" >/dev/null
else
  "${PSQL[@]}" -c "CREATE ROLE rpanel LOGIN PASSWORD '$DB_PASS'" >/dev/null
fi
if [ "$("${PSQL[@]}" -c "SELECT 1 FROM pg_database WHERE datname='rpanel'")" != "1" ]; then
  "${PSQL[@]}" -c "CREATE DATABASE rpanel OWNER rpanel ENCODING 'UTF8' TEMPLATE template0" >/dev/null
fi

log "Securing Redis with a password"
if redis-cli ping 2>/dev/null | grep -q PONG; then
  echo "requirepass $REDIS_PASS" >> /etc/redis/redis.conf
  systemctl restart redis-server
elif [ -n "${EXISTING_REDIS_PASS:-}" ]; then
  REDIS_PASS="$EXISTING_REDIS_PASS"
else
  die "Redis already requires a password; re-run with EXISTING_REDIS_PASS=<password>"
fi

install_files

log "Writing $INSTALL_DIR/.env"
umask 077
cat > "$INSTALL_DIR/.env" <<EOF
APP_ENV=production
APP_PORT=$BACKEND_PORT
APP_HOST=127.0.0.1
FRONTEND_URL=https://$PUBLIC_HOST:$PANEL_PORT

DB_DRIVER=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=rpanel
DB_PASS=$DB_PASS
DB_NAME=rpanel

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASS=$REDIS_PASS
REDIS_DB=0

SESSION_MAX_AGE=86400
SESSION_MAX_CONCURRENT=3

SECRET_KEY=$SECRET_KEY
TRUSTED_PROXIES=127.0.0.1,::1
PANEL_ALLOWED_IPS=$ALLOWED_IPS
EOF
chmod 600 "$INSTALL_DIR/.env"
umask 022

log "Migrating the database and creating the administrator"
(
  cd "$INSTALL_DIR"
  ./rpanel migrate
  ./rpanel seed
  RPANEL_ADMIN_EMAIL="$ADMIN_EMAIL" RPANEL_ADMIN_PASSWORD="$ADMIN_PASS" ./rpanel set-admin
)

log "Starting the panel service"
systemctl enable --now rpanel
wait_healthy

log "Configuring nginx (TLS + secret entrance)"
install -d -m 700 "$INSTALL_DIR/ssl"
if [ ! -f "$INSTALL_DIR/ssl/panel.crt" ]; then
  openssl req -x509 -nodes -newkey rsa:2048 -days 3650 -subj "/CN=rpanel" \
    -keyout "$INSTALL_DIR/ssl/panel.key" -out "$INSTALL_DIR/ssl/panel.crt" >/dev/null 2>&1
  chmod 600 "$INSTALL_DIR/ssl/panel.key"
fi
render "$SRC/deploy/nginx-panel.conf.tmpl" /etc/nginx/sites-available/rpanel-panel.conf
ln -sfn /etc/nginx/sites-available/rpanel-panel.conf /etc/nginx/sites-enabled/rpanel-panel.conf
if ! nginx -t; then
  rm -f /etc/nginx/sites-enabled/rpanel-panel.conf
  die "nginx configuration test failed; the panel site was not enabled"
fi
systemctl reload nginx

# Let's Encrypt renewals must reload nginx so renewed certificates are served.
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/rpanel-nginx-reload.sh <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF
chmod 755 /etc/letsencrypt/renewal-hooks/deploy/rpanel-nginx-reload.sh

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  ufw allow "$PANEL_PORT/tcp" >/dev/null
  log "ufw is active: opened $PANEL_PORT/tcp"
else
  warn "ufw is not active. Consider enabling it from the panel (Firewall); SSH and the panel port are protected automatically."
fi

URL="https://$PUBLIC_HOST:$PANEL_PORT$ENTRANCE"
umask 077
cat > "$INSTALL_DIR/install-info.txt" <<EOF
RPanel access
  URL (open once; it sets the entrance cookie): $URL
  Administrator: $ADMIN_EMAIL
  Password:      $ADMIN_PASS

Enable two-factor authentication right after the first login (user menu > Security).
Lost access? On the server run:  cd $INSTALL_DIR && ./rpanel disable-2fa <email>
This file contains secrets; delete it once you have stored them safely.
EOF
umask 022

cat <<EOF

$(printf '\033[1;32m')RPanel is installed.$(printf '\033[0m')

  URL:       $URL
  Email:     $ADMIN_EMAIL
  Password:  $ADMIN_PASS

  The certificate is self-signed (browser warning is expected). The URL contains a
  secret entrance: without visiting it first, every page returns 404.
  These details are also saved in $INSTALL_DIR/install-info.txt (root only).
  Next step: log in and enable two-factor authentication (user menu > Security).
EOF
