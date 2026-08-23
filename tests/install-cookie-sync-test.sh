#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf -- "$TEMP_DIR"' EXIT

FIXTURE_ROOT=$TEMP_DIR/media-cookie-broker-main
FAKE_BIN=$TEMP_DIR/bin
FAKE_STATE=$TEMP_DIR/docker-state
XDG_ROOT=$TEMP_DIR/data
OUTPUT_DIR=$TEMP_DIR/cookies
mkdir -p "$FIXTURE_ROOT/cmd/cookie-sync" "$FAKE_BIN" "$FAKE_STATE" "$XDG_ROOT" "$OUTPUT_DIR"
touch "$FIXTURE_ROOT/Dockerfile" "$FIXTURE_ROOT/go.mod" "$FIXTURE_ROOT/cmd/cookie-sync/main.go"
tar -czf "$TEMP_DIR/source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main
printf 'reader-secret-value\n' >"$TEMP_DIR/input-password"
printf 'last-known-good\n' >"$OUTPUT_DIR/youtube.txt"

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
cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_DOCKER_STATE/commands"
case " $* " in
    *' compose version '*) printf 'Docker Compose version v2.test\n' ;;
    *' info '*) exit 0 ;;
    *' build --target cookie-sync -t media-cookie-broker-cookie-sync:preview '*)
        [[ ! -f $FAKE_DOCKER_STATE/fail-build ]]
        ;;
    *' compose --env-file '*' -f '*' config -q '*) exit 0 ;;
    *' compose --env-file '*' -f '*' up -d '*) exit 0 ;;
    *' compose --env-file '*' -f '*' ps '*) printf 'cookie-sync running\n' ;;
    *) printf 'unexpected fake docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$FAKE_BIN/curl" "$FAKE_BIN/docker"

run_installer() {
    PATH="$FAKE_BIN:$PATH" \
        FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
        FAKE_DOCKER_STATE="$FAKE_STATE" \
        XDG_DATA_HOME="$XDG_ROOT" \
        MCB_COOKIE_SYNC_BROKER_URL=http://127.0.0.1:8787 \
        MCB_COOKIE_SYNC_USERNAME=downloader \
        MCB_COOKIE_SYNC_PROVIDER=youtube \
        MCB_COOKIE_SYNC_PROFILE=default \
        MCB_COOKIE_SYNC_OUTPUT_DIR="$OUTPUT_DIR" \
        MCB_COOKIE_SYNC_READER_PASSWORD_FILE="$TEMP_DIR/input-password" \
        "$ROOT/install-cookie-sync.sh"
}

run_installer >"$TEMP_DIR/install.log"
TARGET=$XDG_ROOT/media-cookie-broker-cookie-sync
[[ -x $TARGET/cookie-sync && -f $TARGET/compose.yaml && -f $TARGET/.env ]]
[[ $(stat -c '%a' "$TARGET/reader-password") == 600 ]]
[[ $(<"$TARGET/reader-password") == reader-secret-value ]]
[[ $(<"$OUTPUT_DIR/youtube.txt") == last-known-good ]]
! grep -q 'reader-secret-value' "$TARGET/compose.yaml" "$TARGET/.env"
grep -q 'BROKER_PASSWORD_FILE: /run/secrets/reader-password' "$TARGET/compose.yaml"
grep -q 'read_only: true' "$TARGET/compose.yaml"
grep -q 'cap_drop: \[ALL\]' "$TARGET/compose.yaml"
grep -q 'no-new-privileges:true' "$TARGET/compose.yaml"
grep -q '^    network_mode: host$' "$TARGET/compose.yaml"
grep -Fq "$OUTPUT_DIR/youtube.txt" "$TEMP_DIR/install.log"
grep -q '^build --target cookie-sync -t media-cookie-broker-cookie-sync:preview ' "$FAKE_STATE/commands"
[[ -z $(find "$XDG_ROOT" -type f -name '*.go' -print -quit) ]]

password_before=$(cksum "$TARGET/reader-password")
config_before=$(cksum "$TARGET/.env")
run_installer >"$TEMP_DIR/reinstall.log"
grep -q 'Existing Media Cookie Sync installation found' "$TEMP_DIR/reinstall.log"
[[ $(cksum "$TARGET/reader-password") == "$password_before" ]]
[[ $(cksum "$TARGET/.env") == "$config_before" ]]
[[ $(grep -c '^build --target cookie-sync ' "$FAKE_STATE/commands") == 1 ]]
[[ $(<"$OUTPUT_DIR/youtube.txt") == last-known-good ]]

REMOTE_XDG=$TEMP_DIR/remote-data
REMOTE_OUTPUT=$TEMP_DIR/remote-cookies
mkdir "$REMOTE_XDG" "$REMOTE_OUTPUT"
PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    FAKE_DOCKER_STATE="$FAKE_STATE" \
    XDG_DATA_HOME="$REMOTE_XDG" \
    MCB_COOKIE_SYNC_BROKER_URL=https://broker.example.com \
    MCB_COOKIE_SYNC_USERNAME=downloader \
    MCB_COOKIE_SYNC_PROVIDER=youtube \
    MCB_COOKIE_SYNC_PROFILE=default \
    MCB_COOKIE_SYNC_OUTPUT_DIR="$REMOTE_OUTPUT" \
    MCB_COOKIE_SYNC_READER_PASSWORD_FILE="$TEMP_DIR/input-password" \
    "$ROOT/install-cookie-sync.sh" >/dev/null
! grep -q 'network_mode:' "$REMOTE_XDG/media-cookie-broker-cookie-sync/compose.yaml"

FAILED_XDG=$TEMP_DIR/failed-data
mkdir "$FAILED_XDG"
touch "$FAKE_STATE/fail-build"
if PATH="$FAKE_BIN:$PATH" FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" FAKE_DOCKER_STATE="$FAKE_STATE" \
    XDG_DATA_HOME="$FAILED_XDG" MCB_COOKIE_SYNC_BROKER_URL=http://127.0.0.1:8787 \
    MCB_COOKIE_SYNC_USERNAME=downloader MCB_COOKIE_SYNC_PROVIDER=youtube MCB_COOKIE_SYNC_PROFILE=default \
    MCB_COOKIE_SYNC_OUTPUT_DIR="$OUTPUT_DIR" MCB_COOKIE_SYNC_READER_PASSWORD_FILE="$TEMP_DIR/input-password" \
    "$ROOT/install-cookie-sync.sh" >/dev/null 2>&1; then
    printf 'cookie-sync installer unexpectedly survived a failed build\n' >&2
    exit 1
fi
[[ ! -e $FAILED_XDG/media-cookie-broker-cookie-sync ]]
[[ $(<"$OUTPUT_DIR/youtube.txt") == last-known-good ]]
printf 'cookie-sync installer shell tests passed\n'
