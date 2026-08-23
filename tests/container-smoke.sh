#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
IMAGE=${COOKIE_BROKER_TEST_IMAGE:-media-cookie-broker:preview}
BROKER_NAME=${COOKIE_BROKER_TEST_NAME:-cookie-broker-smoke}
SYNC_NAME=${COOKIE_SYNC_TEST_NAME:-cookie-sync-smoke}
PORT=${COOKIE_BROKER_TEST_PORT:-18787}
TEMP_DIR=$(mktemp -d)

cleanup() {
    docker rm -f "$SYNC_NAME" "$BROKER_NAME" >/dev/null 2>&1 || true
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

mkdir -m 700 "$TEMP_DIR/data" "$TEMP_DIR/cookies"
docker rm -f "$SYNC_NAME" "$BROKER_NAME" >/dev/null 2>&1 || true
for user_role in publisher:publisher reader:reader; do
    username=${user_role%%:*}
    role=${user_role##*:}
    docker run --rm --user "$(id -u):$(id -g)" --entrypoint brokerctl \
        -v "$TEMP_DIR/data:/data" \
        -v "$ROOT/tests/fixtures/master-key:/run/secrets/master-key:ro" \
        -v "$ROOT/tests/fixtures/password:/run/secrets/password:ro" \
        "$IMAGE" user add "$username" --role "$role" --provider youtube --password-file /run/secrets/password
done
docker run -d --rm --name "$BROKER_NAME" \
    --user "$(id -u):$(id -g)" \
    --read-only --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
    --cap-drop ALL --security-opt no-new-privileges:true \
    -p "127.0.0.1:${PORT}:8787" \
    -e BROKER_LISTEN_ADDR=0.0.0.0:8787 \
    -e BROKER_DB_PATH=/data/broker.sqlite3 \
    -e BROKER_MASTER_KEY_FILE=/run/secrets/master-key \
    -v "$TEMP_DIR/data:/data" \
    -v "$ROOT/tests/fixtures/master-key:/run/secrets/master-key:ro" \
    "$IMAGE" >/dev/null

for _ in {1..40}; do
    curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break
    sleep 0.25
done
curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null
curl -fsS -u 'publisher:correct horse battery staple' \
    -H 'Content-Type: application/json' --data-binary "@$ROOT/tests/fixtures/upload.json" \
    -X PUT "http://127.0.0.1:${PORT}/v1/providers/youtube/cookies" >/dev/null

docker run -d --rm --name "$SYNC_NAME" \
    --user "$(id -u):$(id -g)" --network "container:$BROKER_NAME" \
    --entrypoint cookie-sync \
    -e BROKER_URL=http://127.0.0.1:8787 \
    -e BROKER_USERNAME=reader \
    -e BROKER_PASSWORD_FILE=/run/secrets/password \
    -e COOKIE_SYNC_TARGETS=youtube=/cookies/youtube.txt \
    -e COOKIE_SYNC_COMBINED=/cookies/cookies.txt \
    -e COOKIE_SYNC_INTERVAL=10s \
    -v "$TEMP_DIR/cookies:/cookies" \
    -v "$ROOT/tests/fixtures/password:/run/secrets/password:ro" \
    "$IMAGE" >/dev/null

for _ in {1..40}; do
    [[ -s $TEMP_DIR/cookies/youtube.txt && -s $TEMP_DIR/cookies/cookies.txt ]] && break
    sleep 0.25
done
grep -q $'SID\tcontainer-smoke-value' "$TEMP_DIR/cookies/youtube.txt"
[[ $(stat -c '%a' "$TEMP_DIR/cookies/youtube.txt") == 600 ]]
[[ $(stat -c '%a' "$TEMP_DIR/cookies/cookies.txt") == 600 ]]
[[ -s $TEMP_DIR/cookies/youtube.txt.meta.json ]]
[[ $(stat -c '%a' "$TEMP_DIR/cookies/youtube.txt.meta.json") == 600 ]]

docker run --rm --user "$(id -u):$(id -g)" --network "container:$BROKER_NAME" \
    --entrypoint cookie-sync \
    -e BROKER_URL=http://127.0.0.1:8787 \
    -e BROKER_USERNAME=reader \
    -e BROKER_PASSWORD_FILE=/run/secrets/password \
    -v "$TEMP_DIR/cookies:/cookies" \
    -v "$ROOT/tests/fixtures/password:/run/secrets/password:ro" \
    "$IMAGE" report --provider youtube --file /cookies/youtube.txt --kind authentication_required >/dev/null

curl -fsS -u 'publisher:correct horse battery staple' \
    "http://127.0.0.1:${PORT}/v1/providers/youtube/status" | grep -q '"auth_health":"refresh_required"'
[[ $(stat -c '%a' "$TEMP_DIR/data/broker.sqlite3") == 600 ]]

docker stop -t 10 "$SYNC_NAME" "$BROKER_NAME" >/dev/null
printf 'cookie broker container smoke passed\n'
