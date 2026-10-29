#!/usr/bin/env bash
# RPanel installer for Ubuntu/Debian. Run as root from a checkout of the repo:
#   sudo bash install.sh
# or on a bare server:
#   RPANEL_REPO=<git-url> bash install.sh
# Re-running upgrades in place and keeps the existing .env, database and admin.
set -Eeuo pipefail

PREFIX=/opt/rpanel
DB_NAME=rpanel
DB_USER=rpanel
BACKEND_PORT=8800
ADMIN_EMAIL="${RPANEL_ADMIN_EMAIL:-admin@rpanel.local}"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }
trap 'die "failed at line $LINENO"' ERR

[ "$(id -u)" -eq 0 ] || die "run as root (sudo bash install.sh)"
[ -r /etc/os-release ] && . /etc/os-release
case "${ID:-}" in ubuntu|debian) ;; *) die "only Ubuntu/Debian is supported (found: ${ID:-unknown})" ;; esac
case "$(uname -m)" in x86_64) GOARCH=amd64 ;; aarch64) GOARCH=arm64 ;; *) die "unsupported arch $(uname -m)" ;; esac

rand_hex() { openssl rand -hex "$1"; }
rand_password() {
  local p
  set +o pipefail
  while :; do
    p=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 14)
    [[ $p =~ [A-Z] && $p =~ [a-z] && $p =~ [0-9] ]] && break
  done
  set -o pipefail
  printf '%s@' "$p"
}

# --- source ---------------------------------------------------------------
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
if [ -f "$SELF_DIR/backend/go.mod" ]; then
  SRC="$SELF_DIR"
else
  [ -n "${RPANEL_REPO:-}" ] || die "not inside the repo; set RPANEL_REPO=<git-url>"
  command -v git >/dev/null || { apt-get update -y && apt-get install -y git; }
  SRC="$PREFIX/src"
  if [ -d "$SRC/.git" ]; then git -C "$SRC" pull --ff-only; else git clone "$RPANEL_REPO" "$SRC"; fi
fi

# --- packages ---------------------------------------------------------------
log "Installing system packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y curl ca-certificates openssl tar gnupg nginx postgresql redis-server

NEED_GO=$(awk '/^go /{print $2; exit}' "$SRC/backend/go.mod")
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${NEED_GO} "; then
  log "Installing Go ${NEED_GO}"
  curl -fsSL "https://go.dev/dl/go${NEED_GO}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
fi
export PATH=/usr/local/go/bin:$PATH:/root/go/bin

if ! command -v node >/dev/null || [ "$(node -p 'process.versions.node.split(".")[0]')" -lt 20 ]; then
  log "Installing Node.js 20"
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
  apt-get install -y nodejs
fi

systemctl enable --now postgresql redis-server nginx

# --- config (first install only) ----------------------------------------------
mkdir -p "$PREFIX"/{bin,www,ssl,data/uploads,data/private}
FRESH=0
if [ ! -f "$PREFIX/.env" ]; then
  FRESH=1
  log "Generating configuration"
  DB_PASS=$(rand_hex 16)
  REDIS_PASS=$(rand_hex 16)
  PANEL_PORT="${RPANEL_PORT:-$(shuf -i 10000-60000 -n 1)}"
  PUBLIC_IP="${RPANEL_HOST:-$(curl -fsS -m 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')}"

  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='${DB_USER}') THEN
    CREATE ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASS}';
  ELSE
    ALTER ROLE ${DB_USER} PASSWORD '${DB_PASS}';
  END IF;
END \$\$;
SQL
  sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1 \
    || sudo -u postgres createdb -O "$DB_USER" "$DB_NAME"

  REDIS_CONF=/etc/redis/redis.conf
  grep -q '^# rpanel$' "$REDIS_CONF" || printf '\n# rpanel\nrequirepass %s\nbind 127.0.0.1 -::1\n' "$REDIS_PASS" >> "$REDIS_CONF"
  systemctl restart redis-server

  cat > "$PREFIX/.env" <<ENV
APP_ENV=production
APP_HOST=127.0.0.1
APP_PORT=${BACKEND_PORT}
FRONTEND_URL=https://${PUBLIC_IP}:${PANEL_PORT}

DB_DRIVER=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=${DB_USER}
DB_PASS=${DB_PASS}
DB_NAME=${DB_NAME}

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASS=${REDIS_PASS}
REDIS_DB=0

SESSION_MAX_AGE=86400
SESSION_MAX_CONCURRENT=3

STORAGE_PATH=${PREFIX}/data/uploads
STORAGE_MAX_SIZE=2097152

LOG_CLEANUP_DAYS=90
RATE_LIMIT_LOGIN=5
RATE_LIMIT_LOGIN_LOCKOUT_MINUTES=15
RATE_LIMIT_API=100
ENV
  chmod 600 "$PREFIX/.env"
  printf '%s %s\n' "$PANEL_PORT" "$PUBLIC_IP" > "$PREFIX/.install"
fi
read -r PANEL_PORT PUBLIC_IP < "$PREFIX/.install"

# --- build ------------------------------------------------------------------------
log "Building backend"
cd "$SRC/backend"
CGO_ENABLED=0 go build -o "$PREFIX/bin/rpanel.new" main.go
mv -f "$PREFIX/bin/rpanel.new" "$PREFIX/bin/rpanel"

log "Building frontend"
cd "$SRC/webadmin"
npm install --no-audit --no-fund
VITE_API_URL=/api/v1 npm run build
rm -rf "$PREFIX/www"/* && cp -r dist/. "$PREFIX/www/"

# --- database ------------------------------------------------------------------------
log "Migrating database"
cd "$PREFIX"
./bin/rpanel migrate
./bin/rpanel seed
if [ "$FRESH" -eq 1 ]; then
  ADMIN_PASS=$(rand_password)
  RPANEL_ADMIN_EMAIL="$ADMIN_EMAIL" RPANEL_ADMIN_PASSWORD="$ADMIN_PASS" ./bin/rpanel set-admin
fi

# --- TLS + nginx ------------------------------------------------------------------------
# The session/CSRF cookies are Secure in production, so plain HTTP would
# silently break login; a self-signed cert keeps the panel usable by IP.
if [ ! -f "$PREFIX/ssl/panel.crt" ]; then
  SAN="DNS:localhost"
  [[ $PUBLIC_IP =~ ^[0-9.]+$ ]] && SAN="IP:${PUBLIC_IP},${SAN}" || SAN="DNS:${PUBLIC_IP},${SAN}"
  openssl req -x509 -nodes -newkey rsa:2048 -days 3650 -subj "/CN=rpanel" -addext "subjectAltName=${SAN}" \
    -keyout "$PREFIX/ssl/panel.key" -out "$PREFIX/ssl/panel.crt" 2>/dev/null
  chmod 600 "$PREFIX/ssl/panel.key"
fi

cat > /etc/nginx/conf.d/rpanel.conf <<NGINX
server {
    listen ${PANEL_PORT} ssl;
    listen [::]:${PANEL_PORT} ssl;
    server_name _;
    ssl_certificate     ${PREFIX}/ssl/panel.crt;
    ssl_certificate_key ${PREFIX}/ssl/panel.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    root ${PREFIX}/www;
    index index.html;
    client_max_body_size 12m;

    add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    location / { try_files \$uri \$uri/ /index.html; }
    location ~ ^/(api|uploads)/ {
        proxy_pass http://127.0.0.1:${BACKEND_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_set_header X-Forwarded-Proto https;
    }
}
NGINX
nginx -t
systemctl reload nginx

# --- service ------------------------------------------------------------------------
cat > /etc/systemd/system/rpanel.service <<UNIT
[Unit]
Description=RPanel
After=network.target postgresql.service redis-server.service

[Service]
WorkingDirectory=${PREFIX}
ExecStart=${PREFIX}/bin/rpanel
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable rpanel
systemctl restart rpanel

if command -v ufw >/dev/null && ufw status | grep -q 'Status: active'; then
  ufw allow "${PANEL_PORT}/tcp" >/dev/null
fi

sleep 2
systemctl is-active --quiet rpanel || die "rpanel service failed to start: journalctl -u rpanel -n 50"

echo
echo "=============================================="
if [ "$FRESH" -eq 1 ]; then
  echo " RPanel installed"
  echo " URL      : https://${PUBLIC_IP}:${PANEL_PORT}"
  echo " Email    : ${ADMIN_EMAIL}"
  echo " Password : ${ADMIN_PASS}"
  echo
  echo " Save these now - the password is not stored anywhere."
  echo " The certificate is self-signed: accept the browser warning."
else
  echo " RPanel upgraded: https://${PUBLIC_IP}:${PANEL_PORT}"
fi
echo " Open TCP ${PANEL_PORT} in your cloud firewall if needed."
echo "=============================================="
