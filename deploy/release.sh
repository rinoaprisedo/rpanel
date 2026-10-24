#!/usr/bin/env bash
# Publishes the current version (const AppVersion in backend/main.go) as a GitHub Release:
# both Linux tarballs, their checksums and install-online.sh. Needs gh (logged in), Go, Node, swag.
#   ./deploy/release.sh [--notes "text" | --notes-file FILE]
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
VERSION="$(sed -n 's/^const AppVersion = "\(.*\)"/\1/p' backend/main.go)"
TAG="v$VERSION"
REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"

[ -z "$(git status --porcelain)" ] || { echo "error: commit your changes first" >&2; exit 1; }
git fetch -q origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "error: HEAD must equal origin/main (push first)" >&2; exit 1; }
if gh release view "$TAG" >/dev/null 2>&1; then echo "error: release $TAG already exists; bump AppVersion" >&2; exit 1; fi

for arch in amd64 arm64; do
  ./deploy/build-release.sh "$arch"
  f="dist/rpanel-$VERSION-linux-$arch.tar.gz"
  ( cd dist && shasum -a 256 "$(basename "$f")" > "$(basename "$f").sha256" )
done

NOTES=(--generate-notes)
[ "${1:-}" = "--notes" ] && NOTES=(--notes "$2")
[ "${1:-}" = "--notes-file" ] && NOTES=(--notes-file "$2")

gh release create "$TAG" --title "RPanel $VERSION" "${NOTES[@]}" \
  "dist/rpanel-$VERSION-linux-amd64.tar.gz" "dist/rpanel-$VERSION-linux-amd64.tar.gz.sha256" \
  "dist/rpanel-$VERSION-linux-arm64.tar.gz" "dist/rpanel-$VERSION-linux-arm64.tar.gz.sha256" \
  deploy/install-online.sh
echo
echo "Install on a server:"
echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/deploy/install-online.sh | sudo bash"
