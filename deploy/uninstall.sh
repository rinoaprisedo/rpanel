#!/usr/bin/env bash
# Removes the RPanel service and its nginx site. Websites, databases, containers
# and files managed by the panel are NOT touched.
#   sudo ./uninstall.sh            keep /opt/rpanel (config, uploads) and the rpanel database
#   sudo ./uninstall.sh --purge    also delete /opt/rpanel and drop the rpanel database/role
set -euo pipefail
INSTALL_DIR="${INSTALL_DIR:-/opt/rpanel}"
[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }

systemctl disable --now rpanel 2>/dev/null || true
rm -f /etc/systemd/system/rpanel.service
systemctl daemon-reload
rm -f /etc/nginx/sites-enabled/rpanel-panel.conf /etc/nginx/sites-available/rpanel-panel.conf
nginx -t && systemctl reload nginx || echo "nginx config test failed; check it manually" >&2
rm -f /etc/cron.d/rpanel-* /etc/letsencrypt/renewal-hooks/deploy/rpanel-nginx-reload.sh

if [ "${1:-}" = "--purge" ]; then
  runuser -u postgres -- psql -c "DROP DATABASE IF EXISTS rpanel" -c "DROP ROLE IF EXISTS rpanel" || true
  rm -rf "$INSTALL_DIR"
  echo "Purged $INSTALL_DIR and the rpanel database."
else
  echo "Kept $INSTALL_DIR and the database. Use --purge to delete them."
fi
echo "RPanel removed."
