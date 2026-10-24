#!/usr/bin/env bash
# Installs or upgrades RPanel from GitHub Releases. Run as root:
#
#   curl -fsSL https://raw.githubusercontent.com/rinoaprisedo/rpanel/main/deploy/install-online.sh | sudo bash
#   sudo rpanel-update                 # later, from the installed copy: latest release
#   sudo rpanel-update 1.3.0           # a specific version
#
# Environment: RPANEL_VERSION (same as the argument), RPANEL_REPO (owner/name, for forks),
# RPANEL_MIRROR (base URL serving the release files, needs an explicit version),
# FORCE=1 (reinstall even if that version is already running). Everything install.sh
# understands (PANEL_PORT, ADMIN_EMAIL, ...) is passed through.
set -euo pipefail

REPO="${RPANEL_REPO:-rinoaprisedo/rpanel}"
VERSION="${1:-${RPANEL_VERSION:-latest}}"
BACKEND_PORT="${BACKEND_PORT:-8800}"

log() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root (sudo)"
for c in curl tar sha256sum; do command -v "$c" >/dev/null 2>&1 || die "$c is required"; done

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported CPU architecture: $(uname -m)" ;;
esac

if [ "$VERSION" = "latest" ] && [ -n "${RPANEL_MIRROR:-}" ]; then die "give an explicit version when using RPANEL_MIRROR"; fi
if [ "$VERSION" = "latest" ]; then
  # /releases/latest redirects to /releases/tag/vX.Y.Z: no API token or jq needed.
  loc="$(curl -fsSI "https://github.com/$REPO/releases/latest" | tr -d '\r' | awk 'tolower($1)=="location:"{print $2}')"
  [ -n "$loc" ] || die "no release found for $REPO (has one been published?)"
  VERSION="${loc##*/}"
fi
VERSION="${VERSION#v}"
case "$VERSION" in ''|*[!0-9A-Za-z._-]*) die "invalid version: $VERSION" ;; esac

running="$(curl -fsS "http://127.0.0.1:$BACKEND_PORT/api/v1/version" 2>/dev/null | sed -n 's/.*"version" *: *"\([^"]*\)".*/\1/p' || true)"
if [ "$running" = "$VERSION" ] && [ "${FORCE:-0}" != "1" ]; then
  log "RPanel $VERSION is already installed (FORCE=1 to reinstall)"
  exit 0
fi
[ -n "$running" ] && log "Upgrading RPanel $running -> $VERSION" || log "Installing RPanel $VERSION"

NAME="rpanel-$VERSION-linux-$ARCH"
BASE="${RPANEL_MIRROR:-https://github.com/$REPO/releases/download/v$VERSION}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

log "Downloading $NAME"
curl -fL --progress-bar -o "$TMP/$NAME.tar.gz" "$BASE/$NAME.tar.gz" || die "download failed: $BASE/$NAME.tar.gz"
curl -fsSL -o "$TMP/$NAME.tar.gz.sha256" "$BASE/$NAME.tar.gz.sha256" || die "checksum file missing for $NAME"
( cd "$TMP" && sha256sum -c "$NAME.tar.gz.sha256" >/dev/null ) || die "checksum mismatch: the download is corrupt or was tampered with"

tar xzf "$TMP/$NAME.tar.gz" -C "$TMP"
[ -x "$TMP/$NAME/deploy/install.sh" ] || die "unexpected package layout"
cd "$TMP/$NAME"
# not exec: the EXIT trap must still remove the download afterwards
./deploy/install.sh
