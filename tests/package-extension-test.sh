#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
cleanup() {
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

MCB_DIST_DIR="$TEMP_DIR/dist" "$ROOT/scripts/package-extension.sh" v0.0.0-test >/dev/null
ARCHIVE=$TEMP_DIR/dist/media-cookie-broker-extension-v0.0.0-test.zip
[[ -f $ARCHIVE ]]
MCB_DIST_DIR="$TEMP_DIR/dist" "$ROOT/scripts/package-extension.sh" >/dev/null
[[ -f $TEMP_DIR/dist/media-cookie-broker-extension.zip ]]

for required in manifest.json service-worker.js popup.html popup.js options.html options.js style.css icon128.png; do
    unzip -Z1 "$ARCHIVE" | grep -qx "$required"
done

if unzip -Z1 "$ARCHIVE" | grep -Eq '(^|/)(node_modules|tests|test-results|coverage|\.nyc_output)(/|$)|\.tmp$|\.swp$|~$'; then
    printf 'extension archive contains excluded development files\n' >&2
    exit 1
fi

mkdir "$TEMP_DIR/extracted"
unzip -q "$ARCHIVE" -d "$TEMP_DIR/extracted"
node -e 'JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))' "$TEMP_DIR/extracted/manifest.json"

printf 'extension packaging tests passed\n'
