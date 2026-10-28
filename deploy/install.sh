#!/usr/bin/env bash
# RPanel installer for Ubuntu / Debian. Run as root from the extracted release directory:
#
#   sudo ./deploy/install.sh
#
# Optional environment variables:
#   PANEL_DOMAIN=<domain>   REQUIRED on a new install: host name of the panel (e.g. panel.example.com). Its
#                           A record must already point at this server; a Let's Encrypt certificate is
#                           obtained automatically and access by IP is refused. When not set the installer
#                           asks for it. On an upgrade it migrates an old IP-based install.
#   CERTBOT_EMAIL=<email>   optional e-mail for the Let's Encrypt account (not needed; never asked)
#   SKIP_DNS_CHECK=1        do not require the domain to resolve to this server (NAT, Cloudflare proxy, ...)
#   PANEL_PORT=<port>       public HTTPS port of the panel (default: random 20000-59999)
#   ADMIN_EMAIL=<email>     initial administrator (default: admin@rpanel.local)
#   PUBLIC_HOST=<host/ip>   host shown in the printed URL when no domain is used (default: first IP)
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

# ---- Never update from inside the panel's own process tree ---------------------------------------
# A shell in the panel's web terminal (or anything else started by rpanel.service) lives in the service's
# cgroup: stopping the panel for the upgrade would kill this script half-way and leave the panel stopped.
# Detect that and carry on as a separate systemd unit, with the output in $INSTALL_DIR/update.log.
if [ "${RPANEL_DETACHED:-0}" != "1" ] && [ "$(id -u)" -eq 0 ] && [ -f "$INSTALL_DIR/.env" ] \
   && grep -q 'rpanel\.service' /proc/self/cgroup 2>/dev/null && command -v systemd-run >/dev/null 2>&1; then
  STAGE="$(mktemp -d /var/tmp/rpanel-update.XXXXXX)"
  cp -a "$SRC/." "$STAGE/"           # the caller removes its temp dir as soon as we return
  UPDATE_LOG="$INSTALL_DIR/update.log"; : > "$UPDATE_LOG"
  UNIT="rpanel-install-$$"
  ENV_ARGS=()
  for v in PANEL_PORT ADMIN_EMAIL PUBLIC_HOST PANEL_ALLOWED_IPS INSTALL_DIR PANEL_DOMAIN CERTBOT_EMAIL SKIP_DNS_CHECK; do
    [ -n "${!v:-}" ] && ENV_ARGS+=(-E "$v=${!v}")
  done
  printf '\033[1;32m==>\033[0m You are updating from the panel itself: continuing in the background (log: %s)\n' "$UPDATE_LOG"
  printf '    The panel restarts and this terminal will disconnect; that is expected. Reconnect in a minute.\n'
  systemd-run --unit="$UNIT" --collect --quiet \
    -p "StandardOutput=file:$UPDATE_LOG" -p "StandardError=file:$UPDATE_LOG" \
    -E RPANEL_DETACHED=1 -E "RPANEL_STAGE=$STAGE" "${ENV_ARGS[@]}" "$STAGE/deploy/install.sh" \
    || { rm -rf "$STAGE"; printf '\033[1;31merror:\033[0m could not start the detached update\n' >&2; exit 1; }
  tail -n +1 -f "$UPDATE_LOG" 2>/dev/null &
  TAILPID=$!
  while systemctl is-active --quiet "$UNIT" 2>/dev/null; do sleep 1; done
  kill "$TAILPID" 2>/dev/null || true
  grep -q "Upgrade finished" "$UPDATE_LOG" 2>/dev/null
  exit $?
fi
if [ "${RPANEL_DETACHED:-0}" = "1" ] && [ -n "${RPANEL_STAGE:-}" ]; then
  trap 'rm -rf "$RPANEL_STAGE"' EXIT
fi

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

# ---- Panel SSL helpers -------------------------------------------------------------
DOMAIN_RE='^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$'

can_prompt() { [ -r /dev/tty ] && [ -w /dev/tty ] && ( : </dev/tty ) 2>/dev/null; }

ask() { # ask <prompt> <varname>  (reads from the terminal even when the script itself is piped)
  local reply=""
  if can_prompt; then
    printf '%s' "$1" > /dev/tty
    IFS= read -r reply < /dev/tty || true
  fi
  printf -v "$2" '%s' "$reply"
}

write_tls_defaults() { # panel reachable by IP with the self-signed certificate (legacy mode)
  install -d -m 700 "$INSTALL_DIR/ssl"
  cat > "$INSTALL_DIR/ssl/panel-tls.conf" <<TLSEOF
# Managed by RPanel (Settings > Panel SSL). Do not edit: changes are overwritten.
# rpanel-panel-ssl mode=legacy domain=
server_name _;
ssl_certificate $INSTALL_DIR/ssl/panel.crt;
ssl_certificate_key $INSTALL_DIR/ssl/panel.key;
TLSEOF
  printf '# Managed by RPanel: becomes active once a panel domain is set.\n' > "$INSTALL_DIR/ssl/panel-guard.conf"
  chmod 644 "$INSTALL_DIR/ssl/panel-tls.conf" "$INSTALL_DIR/ssl/panel-guard.conf"
}

ensure_panel_env() { # ensure_panel_env <port> : Panel SSL settings in .env (idempotent)
  grep -q '^PANEL_TLS_CONF=' "$INSTALL_DIR/.env" && return 0
  cat >> "$INSTALL_DIR/.env" <<ENVEOF

# Panel SSL (Settings > Panel SSL)
PANEL_TLS_CONF=$INSTALL_DIR/ssl/panel-tls.conf
PANEL_GUARD_CONF=$INSTALL_DIR/ssl/panel-guard.conf
PANEL_SSL_DIR=$INSTALL_DIR/ssl
PANEL_PORT=$1
ENVEOF
}

panel_domain_in_use() { # prints the domain recorded in panel-tls.conf (empty in legacy mode)
  sed -n 's/^# rpanel-panel-ssl mode=[^ ]* domain=\(.*\)$/\1/p' "$INSTALL_DIR/ssl/panel-tls.conf" 2>/dev/null | head -1
}

# Fresh installs need a domain and an e-mail; verify DNS and port 80 BEFORE installing anything.
require_domain() {
  PANEL_DOMAIN="$(printf '%s' "${PANEL_DOMAIN:-}" | tr 'A-Z' 'a-z' | tr -d '[:space:]')"
  if [ -z "$PANEL_DOMAIN" ]; then
    ask "Panel domain (e.g. panel.example.com, its A record must point at this server): " PANEL_DOMAIN
    PANEL_DOMAIN="$(printf '%s' "$PANEL_DOMAIN" | tr 'A-Z' 'a-z' | tr -d '[:space:]')"
  fi
  [ -n "$PANEL_DOMAIN" ] || die "RPanel requires a domain. Point an A record (e.g. panel.example.com) at this server, then run again with PANEL_DOMAIN=panel.example.com"
  printf '%s' "$PANEL_DOMAIN" | grep -Eq "$DOMAIN_RE" || die "invalid domain: $PANEL_DOMAIN (use a host name such as panel.example.com, not an IP address)"
  CERTBOT_EMAIL="${CERTBOT_EMAIL:-}"
  if [ -n "$CERTBOT_EMAIL" ]; then
    printf '%s' "$CERTBOT_EMAIL" | grep -Eq '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$' || die "CERTBOT_EMAIL is not a valid e-mail address (leave it out; Let's Encrypt does not need one)"
  fi

  if [ "${SKIP_DNS_CHECK:-0}" != "1" ]; then
    resolved="$(getent ahostsv4 "$PANEL_DOMAIN" 2>/dev/null | awk '{print $1}' | sort -u | tr '\n' ' ')"
    [ -n "$resolved" ] || die "$PANEL_DOMAIN does not resolve yet. Create an A record pointing at this server's public IP, wait for DNS to update, then run again (SKIP_DNS_CHECK=1 to bypass)"
    mine="$(hostname -I 2>/dev/null) $(curl -fsS4 --max-time 8 https://api.ipify.org 2>/dev/null || true)"
    ok=0; for ip in $resolved; do case " $mine " in *" $ip "*) ok=1 ;; esac; done
    [ "$ok" = 1 ] || die "$PANEL_DOMAIN resolves to ${resolved}which is not an address of this server (${mine}). Fix the DNS A record, or set SKIP_DNS_CHECK=1 if you are behind NAT or a proxy"
  fi
  if ss -ltn "( sport = :80 )" 2>/dev/null | grep -q LISTEN && ! ss -ltnp "( sport = :80 )" 2>/dev/null | grep -q nginx; then
    die "port 80 is used by another program; Let's Encrypt needs it. Stop that service (and do not run another web panel next to RPanel)"
  fi
}

issue_panel_cert() { # issue_panel_cert <domain> [email]
  log "Requesting a Let's Encrypt certificate for $1"
  ( cd "$INSTALL_DIR" && ./rpanel panel-ssl issue "$1" "$2" )
}

# Installs before 1.3.3 had the host name and certificate hard-coded in rpanel-panel.conf.
migrate_panel_conf() {
  local conf=/etc/nginx/sites-available/rpanel-panel.conf
  [ -f "$conf" ] || return 0
  grep -q 'panel-tls.conf' "$conf" && return 0
  log "Migrating the panel nginx site to Panel SSL (certificate in $INSTALL_DIR/ssl/panel-tls.conf)"
  local crt key port
  crt="$(awk '$1=="ssl_certificate"{gsub(";","",$2);print $2;exit}' "$conf")"
  key="$(awk '$1=="ssl_certificate_key"{gsub(";","",$2);print $2;exit}' "$conf")"
  port="$(awk '$1=="listen" && $3=="ssl;" && $2 !~ /\[/ {print $2;exit}' "$conf")"
  if [ -z "$crt" ] || [ -z "$key" ] || [ -z "$port" ]; then warn "unrecognised $conf; left unchanged"; return 0; fi
  cp -p "$conf" "$conf.bak"
  install -d -m 700 "$INSTALL_DIR/ssl"
  cat > "$INSTALL_DIR/ssl/panel-tls.conf" <<TLSEOF
# Managed by RPanel (Settings > Panel SSL). Do not edit: changes are overwritten.
# rpanel-panel-ssl mode=legacy domain=
server_name _;
ssl_certificate $crt;
ssl_certificate_key $key;
TLSEOF
  printf '# Managed by RPanel: becomes active once a panel domain is set.\n' > "$INSTALL_DIR/ssl/panel-guard.conf"
  chmod 644 "$INSTALL_DIR/ssl/panel-tls.conf" "$INSTALL_DIR/ssl/panel-guard.conf"
  sed -i -e '/^[[:space:]]*server_name[[:space:]]/d' \
         -e '/^[[:space:]]*ssl_certificate_key[[:space:]]/d' \
         -e "s|^[[:space:]]*ssl_certificate[[:space:]].*|    include $INSTALL_DIR/ssl/panel-tls.conf;|" "$conf"
  grep -q 'error_page 497' "$conf" || sed -i '/server_tokens off;/a\    error_page 497 =301 https://$host:$server_port$request_uri;' "$conf"
  printf '\ninclude %s/ssl/panel-guard.conf;\n' "$INSTALL_DIR" >> "$conf"
  if nginx -t >/dev/null 2>&1; then
    systemctl reload nginx; rm -f "$conf.bak"
  else
    mv -f "$conf.bak" "$conf"; rm -f "$INSTALL_DIR/ssl/panel-tls.conf" "$INSTALL_DIR/ssl/panel-guard.conf"
    warn "could not migrate $conf to Panel SSL; it was left unchanged"
    return 0
  fi
  ensure_panel_env "$port"
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
  [ -f "$SRC/deploy/uninstall.sh" ] && install -m 755 "$SRC/deploy/uninstall.sh" /usr/local/bin/rpanel-uninstall
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
  # Panels installed before 1.3.1 serve the web UI without a Content-Security-Policy.
  conf=/etc/nginx/sites-available/rpanel-panel.conf
  if [ -f "$conf" ] && ! grep -q "Content-Security-Policy" "$conf"; then
    cp -p "$conf" "$conf.bak"
    sed -i "/add_header Referrer-Policy/a\\    add_header Content-Security-Policy \"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data:; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'\" always;" "$conf"
    if nginx -t >/dev/null 2>&1; then systemctl reload nginx; rm -f "$conf.bak"; else mv -f "$conf.bak" "$conf"; warn "could not add the CSP header to $conf"; fi
  fi
  migrate_panel_conf
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
  PANEL_DOMAIN="$(printf '%s' "${PANEL_DOMAIN:-}" | tr 'A-Z' 'a-z' | tr -d '[:space:]')"
  if [ -n "$PANEL_DOMAIN" ] && [ "$(panel_domain_in_use)" != "$PANEL_DOMAIN" ] && [ -f "$INSTALL_DIR/ssl/panel-tls.conf" ]; then
    if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then ufw allow 80/tcp >/dev/null; fi
    issue_panel_cert "$PANEL_DOMAIN" "${CERTBOT_EMAIL:-}" || warn "could not set the panel domain; finish it in the panel: Settings > Panel SSL"
  elif [ -z "$(panel_domain_in_use)" ]; then
    warn "the panel has no domain yet (self-signed certificate). Open the panel > Settings > Panel SSL to set one, or re-run with PANEL_DOMAIN=<domain>."
  fi
  log "Upgrade finished"
  exit 0
fi

# ----------------------------------------------------------------- fresh install
require_domain
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
FRONT_HOST="${PANEL_DOMAIN:-$PUBLIC_HOST}"
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
FRONTEND_URL=https://$FRONT_HOST:$PANEL_PORT

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
CERTBOT_EMAIL=${CERTBOT_EMAIL:-}

# Panel SSL (Settings > Panel SSL)
PANEL_TLS_CONF=$INSTALL_DIR/ssl/panel-tls.conf
PANEL_GUARD_CONF=$INSTALL_DIR/ssl/panel-guard.conf
PANEL_SSL_DIR=$INSTALL_DIR/ssl
PANEL_PORT=$PANEL_PORT
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
write_tls_defaults
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
  ufw allow 80/tcp >/dev/null
  log "ufw is active: opened $PANEL_PORT/tcp and 80/tcp"
else
  warn "ufw is not active. Consider enabling it from the panel (Firewall); SSH and the panel port are protected automatically."
fi

CERT_NOTE="The certificate is issued by Let's Encrypt and renews automatically."
if issue_panel_cert "$PANEL_DOMAIN" "$CERTBOT_EMAIL"; then
  URL="https://$PANEL_DOMAIN:$PANEL_PORT$ENTRANCE"
else
  URL="https://$PUBLIC_HOST:$PANEL_PORT$ENTRANCE"
  CERT_NOTE="The Let's Encrypt certificate could NOT be issued (see the message above; check that $PANEL_DOMAIN points at this server and port 80 is reachable). The panel works for now at the IP URL with a self-signed certificate: log in, open Settings > Panel SSL and press Issue to finish."
  warn "HTTPS for $PANEL_DOMAIN is not active yet"
fi
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

  $CERT_NOTE
  The URL contains a secret entrance: without visiting it first, every page returns 404.
  These details are also saved in $INSTALL_DIR/install-info.txt (root only).
  Next step: log in and enable two-factor authentication (user menu > Security).
EOF
