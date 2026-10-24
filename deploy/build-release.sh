#!/usr/bin/env bash
# Builds a self-contained release tarball on your development machine (needs Go, Node/npm, swag).
#   ./deploy/build-release.sh [amd64|arm64]     default: amd64
# Output: dist/rpanel-<version>-linux-<arch>.tar.gz — copy it to the server, extract, run deploy/install.sh.
set -euo pipefail
ARCH="${1:-amd64}"
case "$ARCH" in amd64|arm64) ;; *) echo "arch must be amd64 or arm64" >&2; exit 1 ;; esac
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(sed -n 's/^const AppVersion = "\(.*\)"/\1/p' "$ROOT/backend/main.go")"
NAME="rpanel-${VERSION:-dev}-linux-$ARCH"
OUT="$ROOT/dist/$NAME"

rm -rf "$OUT"; mkdir -p "$OUT/deploy"
echo "==> frontend"
( cd "$ROOT/webadmin" && npm ci && VITE_API_URL=/api/v1 npm run build )
cp -R "$ROOT/webadmin/dist" "$OUT/web"
echo "==> backend ($ARCH)"
( cd "$ROOT/backend" && swag init -g main.go -o docs >/dev/null && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags "-s -w" -o "$OUT/rpanel" . )
cp "$ROOT"/deploy/{install.sh,install-online.sh,uninstall.sh,rpanel.service,nginx-panel.conf.tmpl} "$OUT/deploy/"
cp "$ROOT/README.md" "$OUT/README.md"
chmod +x "$OUT/deploy/install.sh" "$OUT/deploy/install-online.sh" "$OUT/deploy/uninstall.sh" "$OUT/rpanel"
( cd "$ROOT/dist" && tar czf "$NAME.tar.gz" "$NAME" )
echo "==> $ROOT/dist/$NAME.tar.gz"
