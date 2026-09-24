#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
plugin_id=$(node -p 'require("./plugin.json").id')
version=$(node -p 'require("./plugin.json").version')
mkdir -p server/dist dist
npm --prefix webapp run build
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "server/dist/plugin-linux-$arch" ./server
done
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/$plugin_id/server/dist" "$stage/$plugin_id/webapp/dist" "$stage/$plugin_id/assets"
cp plugin.json README.md "$stage/$plugin_id/"
cp server/dist/plugin-linux-* "$stage/$plugin_id/server/dist/"
cp webapp/dist/main.js "$stage/$plugin_id/webapp/dist/"
cp assets/icon.svg "$stage/$plugin_id/assets/"
COPYFILE_DISABLE=1 tar -czf "dist/$plugin_id-$version.tar.gz" -C "$stage" "$plugin_id"
node scripts/check-package.mjs "dist/$plugin_id-$version.tar.gz"
if command -v sha256sum >/dev/null; then
  (cd dist && sha256sum "$plugin_id-$version.tar.gz" > SHA256SUMS)
else
  (cd dist && shasum -a 256 "$plugin_id-$version.tar.gz" > SHA256SUMS)
fi
