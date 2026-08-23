#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf -- "$TEMP_DIR"' EXIT

FIXTURE_ROOT=$TEMP_DIR/media-cookie-broker-main
FAKE_BIN=$TEMP_DIR/bin
XDG_ROOT=$TEMP_DIR/data
mkdir -p "$FIXTURE_ROOT/extension/tests" "$FIXTURE_ROOT/cmd" "$FIXTURE_ROOT/demo" "$FAKE_BIN" "$XDG_ROOT"
printf '{"manifest_version":3,"name":"test","version":"1"}\n' >"$FIXTURE_ROOT/extension/manifest.json"
printf 'runtime\n' >"$FIXTURE_ROOT/extension/service-worker.js"
printf '{}\n' >"$FIXTURE_ROOT/extension/package.json"
printf 'test\n' >"$FIXTURE_ROOT/extension/tests/test.mjs"
printf 'server\n' >"$FIXTURE_ROOT/cmd/main.go"
printf 'video\n' >"$FIXTURE_ROOT/demo/video.ts"
tar -czf "$TEMP_DIR/source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main

cat >"$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
while (($#)); do
    if [[ $1 == -o ]]; then
        cp "$FIXTURE_ARCHIVE" "$2"
        exit
    fi
    shift
done
exit 1
EOF
chmod +x "$FAKE_BIN/curl"

PATH="$FAKE_BIN:$PATH" FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" "$ROOT/install-extension.sh" >"$TEMP_DIR/install.log"

TARGET=$XDG_ROOT/media-cookie-broker-extension/current
[[ -f $TARGET/manifest.json && -f $TARGET/service-worker.js ]]
for absent in package.json tests cmd demo README.md go.mod; do
    [[ ! -e $TARGET/$absent ]]
done
grep -Fq "$TARGET" "$TEMP_DIR/install.log"
grep -q 'Load unpacked' "$TEMP_DIR/install.log"
manifest_before=$(cksum "$TARGET/manifest.json")

printf 'runtime-v2\n' >"$FIXTURE_ROOT/extension/service-worker.js"
tar -czf "$TEMP_DIR/source-v2.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main
PATH="$FAKE_BIN:$PATH" FIXTURE_ARCHIVE="$TEMP_DIR/source-v2.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" "$ROOT/install-extension.sh" >/dev/null
printf 'runtime-v3\n' >"$FIXTURE_ROOT/extension/service-worker.js"
tar -czf "$TEMP_DIR/source-v3.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main
PATH="$FAKE_BIN:$PATH" FIXTURE_ARCHIVE="$TEMP_DIR/source-v3.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" "$ROOT/install-extension.sh" >/dev/null
[[ $(<"$TARGET/service-worker.js") == runtime-v3 ]]
mapfile -t backups < <(find "$XDG_ROOT/media-cookie-broker-extension" -mindepth 1 -maxdepth 1 \
    -type d -name 'previous-*' -print)
[[ ${#backups[@]} == 1 ]]
[[ $(<"${backups[0]}/service-worker.js") == runtime-v2 ]]
manifest_before=$(cksum "$TARGET/manifest.json")
service_worker_before=$(cksum "$TARGET/service-worker.js")

BAD_ROOT=$TEMP_DIR/media-cookie-broker-bad
mkdir -p "$BAD_ROOT/extension"
printf 'missing manifest\n' >"$BAD_ROOT/extension/service-worker.js"
tar -czf "$TEMP_DIR/bad-source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-bad
if PATH="$FAKE_BIN:$PATH" FIXTURE_ARCHIVE="$TEMP_DIR/bad-source.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" "$ROOT/install-extension.sh" >/dev/null 2>&1; then
    printf 'extension installer unexpectedly accepted a bad update\n' >&2
    exit 1
fi
[[ $(cksum "$TARGET/manifest.json") == "$manifest_before" ]]
[[ $(cksum "$TARGET/service-worker.js") == "$service_worker_before" ]]
[[ $(find "$XDG_ROOT/media-cookie-broker-extension" -mindepth 1 -maxdepth 1 \
    -type d -name 'previous-*' | wc -l) == 1 ]]
[[ -z $(find "$XDG_ROOT/media-cookie-broker-extension" -maxdepth 1 -name '.extension-staging.*' -print -quit) ]]
printf 'extension installer shell tests passed\n'
