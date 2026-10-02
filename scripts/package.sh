#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version=$(tr -d '\n\r' < VERSION)
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$ ]] || { echo 'Invalid VERSION' >&2; exit 1; }
commit=$(git rev-parse HEAD)
built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
mkdir -p release
staging=$(mktemp -d)
trap 'rm -rf -- "$staging"' EXIT
for arch in amd64 arm64; do
  folder="$staging/nodesweep-linux-$arch"
  mkdir -p "$folder/deploy" "$folder/docs"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags="-s -w -X main.version=$version -X main.commit=$commit -X main.builtAt=$built_at" \
    -o "$folder/nodesweep" ./cmd/nodesweep
  cp install.sh README*.md LICENSE VERSION SECURITY.md "$folder/"
  cp deploy/nodesweep.service "$folder/deploy/"
  cp docs/*.md "$folder/docs/"
  cp docs/release-notes.json "$folder/docs/"
  python3 scripts/release_notes.py > "$folder/docs/release-notes.md"
  cp -R docs/images "$folder/docs/"
  VERSION_VALUE="$version" COMMIT_VALUE="$commit" BUILD_TIME="$built_at" python3 - "$folder/build-info.json" <<'PY'
import json, os, sys
with open(sys.argv[1], 'w') as f:
    json.dump(dict(version=os.environ['VERSION_VALUE'], commit=os.environ['COMMIT_VALUE'], builtAt=os.environ['BUILD_TIME']), f, indent=2)
    f.write('\n')
PY
  tar -C "$folder" -czf "release/nodesweep-linux-$arch.tar.gz" .
done
(cd release && sha256sum nodesweep-linux-amd64.tar.gz nodesweep-linux-arm64.tar.gz > SHA256SUMS)
