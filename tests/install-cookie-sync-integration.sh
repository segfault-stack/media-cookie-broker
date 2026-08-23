#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
BROKER_IMAGE=${COOKIE_BROKER_TEST_IMAGE:-media-cookie-broker:preview}
SYNC_IMAGE=${COOKIE_SYNC_INSTALL_TEST_IMAGE:-media-cookie-broker-cookie-sync:installer-test}
BROKER_NAME=${COOKIE_SYNC_INSTALL_TEST_BROKER_NAME:-cookie-sync-installer-broker}
PORT=${COOKIE_SYNC_INSTALL_TEST_PORT:-28787}
TEMP_DIR=$(mktemp -d)
XDG_ROOT=$TEMP_DIR/data-home
OUTPUT_DIR=$TEMP_DIR/cookies
TARGET=$XDG_ROOT/media-cookie-broker-cookie-sync
COMPOSE_PROJECT_NAME=media-cookie-sync-installer-test-$$

cleanup() {
    if [[ -x $TARGET/cookie-sync ]]; then
        COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME" \
            "$TARGET/cookie-sync" down --remove-orphans >/dev/null 2>&1 || true
    fi
    docker rm -f "$BROKER_NAME" >/dev/null 2>&1 || true
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

[[ $(uname -s) == Linux ]] || {
    printf 'generated installer loopback integration requires Linux host networking\n' >&2
    exit 1
}
for tool in curl docker tar; do
    command -v "$tool" >/dev/null 2>&1 || {
        printf '%s is required\n' "$tool" >&2
        exit 1
    }
done
docker info >/dev/null
docker image inspect "$BROKER_IMAGE" >/dev/null

FIXTURE_ROOT=$TEMP_DIR/media-cookie-broker-main
FAKE_BIN=$TEMP_DIR/bin
mkdir -p "$FIXTURE_ROOT/cmd" "$FIXTURE_ROOT/internal" "$FAKE_BIN" "$XDG_ROOT" "$OUTPUT_DIR" "$TEMP_DIR/broker-data"
cp -- "$ROOT/Dockerfile" "$ROOT/go.mod" "$ROOT/go.sum" "$FIXTURE_ROOT/"
cp -R -- "$ROOT/cmd/." "$FIXTURE_ROOT/cmd/"
cp -R -- "$ROOT/internal/." "$FIXTURE_ROOT/internal/"
tar -czf "$TEMP_DIR/source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main

cat >"$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
while (($#)); do
    if [[ $1 == -o ]]; then
        cp -- "$FIXTURE_ARCHIVE" "$2"
        exit
    fi
    shift
done
exit 1
EOF
chmod 0755 "$FAKE_BIN/curl"
printf 'correct horse battery staple\n' >"$TEMP_DIR/reader-password"
chmod 0600 "$TEMP_DIR/reader-password"

docker rm -f "$BROKER_NAME" >/dev/null 2>&1 || true
for user_role in publisher:publisher reader:reader; do
    username=${user_role%%:*}
    role=${user_role##*:}
    docker run --rm --user "$(id -u):$(id -g)" --entrypoint brokerctl \
        -v "$TEMP_DIR/broker-data:/data" \
        -v "$ROOT/tests/fixtures/master-key:/run/secrets/master-key:ro" \
        -v "$ROOT/tests/fixtures/password:/run/secrets/password:ro" \
        "$BROKER_IMAGE" user add "$username" --role "$role" --provider youtube \
        --password-file /run/secrets/password
done
docker run -d --rm --name "$BROKER_NAME" \
    --user "$(id -u):$(id -g)" --read-only \
    --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
    --cap-drop ALL --security-opt no-new-privileges:true \
    -p "127.0.0.1:${PORT}:8787" \
    -e BROKER_LISTEN_ADDR=0.0.0.0:8787 \
    -e BROKER_DB_PATH=/data/broker.sqlite3 \
    -e BROKER_MASTER_KEY_FILE=/run/secrets/master-key \
    -v "$TEMP_DIR/broker-data:/data" \
    -v "$ROOT/tests/fixtures/master-key:/run/secrets/master-key:ro" \
    "$BROKER_IMAGE" >/dev/null

for _ in {1..40}; do
    curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break
    sleep 0.25
done
curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null
curl -fsS -u 'publisher:correct horse battery staple' \
    -H 'Content-Type: application/json' --data-binary "@$ROOT/tests/fixtures/upload.json" \
    -X PUT "http://127.0.0.1:${PORT}/v1/providers/youtube/cookies" >/dev/null

PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    XDG_DATA_HOME="$XDG_ROOT" \
    COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME" \
    COOKIE_SYNC_IMAGE="$SYNC_IMAGE" \
    MCB_COOKIE_SYNC_BROKER_URL="http://127.0.0.1:${PORT}" \
    MCB_COOKIE_SYNC_USERNAME=reader \
    MCB_COOKIE_SYNC_PROVIDER=youtube \
    MCB_COOKIE_SYNC_PROFILE=default \
    MCB_COOKIE_SYNC_OUTPUT_DIR="$OUTPUT_DIR" \
    MCB_COOKIE_SYNC_READER_PASSWORD_FILE="$TEMP_DIR/reader-password" \
    "$ROOT/install-cookie-sync.sh" >"$TEMP_DIR/install.log"
printf 'COMPOSE_PROJECT_NAME=%s\n' "$COMPOSE_PROJECT_NAME" >>"$TARGET/.env"

grep -q '^    network_mode: host$' "$TARGET/compose.yaml"
for _ in {1..40}; do
    [[ -s $OUTPUT_DIR/youtube.txt ]] && break
    sleep 0.25
done
grep -q $'SID\tcontainer-smoke-value' "$OUTPUT_DIR/youtube.txt"
[[ -s $OUTPUT_DIR/youtube.txt.meta.json ]]
[[ $(stat -c '%a' "$OUTPUT_DIR/youtube.txt") == 600 ]]
printf 'generated cookie-sync installer loopback integration passed\n'
