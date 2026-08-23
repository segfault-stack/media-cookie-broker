#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
cleanup() {
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

FIXTURE_ROOT=$TEMP_DIR/media-cookie-broker-main
FAKE_BIN=$TEMP_DIR/bin
XDG_ROOT=$TEMP_DIR/data
mkdir -p "$FIXTURE_ROOT" "$FAKE_BIN" "$XDG_ROOT"
touch "$FIXTURE_ROOT/docker-compose.yml"
cat >"$FIXTURE_ROOT/mcb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ ${1:-} == setup ]]
touch setup-ran
EOF
chmod +x "$FIXTURE_ROOT/mcb"
tar -czf "$TEMP_DIR/source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main

cat >"$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=
while (($#)); do
    if [[ $1 == -o ]]; then
        output=$2
        shift 2
    else
        shift
    fi
done
cp "$FIXTURE_ARCHIVE" "$output"
EOF
cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ $* == 'compose version' ]]
EOF
chmod +x "$FAKE_BIN/curl" "$FAKE_BIN/docker"

PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" \
    "$ROOT/install.sh" >/dev/null

INSTALL_ROOT=$XDG_ROOT/media-cookie-broker
[[ -x $INSTALL_ROOT/mcb ]]
[[ -f $INSTALL_ROOT/setup-ran ]]

if PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" \
    "$ROOT/install.sh" >/dev/null 2>&1; then
    printf 'installer unexpectedly replaced an existing installation\n' >&2
    exit 1
fi

[[ -f $INSTALL_ROOT/setup-ran ]]
printf 'installer shell tests passed\n'
