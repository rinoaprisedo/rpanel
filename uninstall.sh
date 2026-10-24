#!/usr/bin/env bash
# Removes RPanel (service, nginx site, files). Keeps nginx/postgresql/redis packages.
# Pass --purge-data to also drop the rpanel database and role.
set -Eeuo pipefail
[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }

PORT=""
[ -f /opt/rpanel/.install ] && read -r PORT _ < /opt/rpanel/.install

systemctl disable --now rpanel 2>/dev/null || true
rm -f /etc/systemd/system/rpanel.service /etc/nginx/conf.d/rpanel.conf
systemctl daemon-reload
nginx -t 2>/dev/null && systemctl reload nginx || true
[ -n "$PORT" ] && command -v ufw >/dev/null && ufw delete allow "${PORT}/tcp" >/dev/null 2>&1 || true

if [ "${1:-}" = "--purge-data" ]; then
  sudo -u postgres psql -c 'DROP DATABASE IF EXISTS rpanel' -c 'DROP ROLE IF EXISTS rpanel'
  rm -rf /opt/rpanel
else
  find /opt/rpanel -mindepth 1 -maxdepth 1 ! -name data ! -name .env ! -name .install -exec rm -rf {} + 2>/dev/null || true
  echo "kept /opt/rpanel/data, .env and the database (use --purge-data to remove)"
fi
echo "RPanel removed"
