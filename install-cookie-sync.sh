#!/usr/bin/env bash
set -euo pipefail
umask 077

REPOSITORY=segfault-stack/media-cookie-broker
REF=${MCB_INSTALL_REF:-v0.3.0-preview}
IMAGE=${COOKIE_SYNC_IMAGE:-media-cookie-broker-cookie-sync:preview}
DATA_HOME=${XDG_DATA_HOME:-$HOME/.local/share}
TARGET=$DATA_HOME/media-cookie-broker-cookie-sync

fail() {
    printf '✗ %s\n' "$1" >&2
    if [[ $# -gt 1 ]]; then
        printf '  %s\n' "$2" >&2
    fi
    exit 1
}

print_summary() {
    local cookie_file=$1
    printf '\nMedia Cookie Sync is ready 🍪\n\n'
    printf 'Cookie file:\n  %s\n\n' "$cookie_file"
    printf 'Point your downloader at that file.\n\n'
    printf 'Useful commands:\n'
    printf '  %s status\n' "$TARGET/cookie-sync"
    printf '  %s logs\n' "$TARGET/cookie-sync"
}

if [[ -e $TARGET || -L $TARGET ]]; then
    [[ -d $TARGET && ! -L $TARGET && -x $TARGET/cookie-sync && -f $TARGET/compose.yaml && \
        -f $TARGET/.env && -s $TARGET/reader-password ]] || \
        fail "Refusing to use an invalid existing cookie-sync installation at $TARGET."
    COOKIE_FILE=$(sed -n 's/^COOKIE_FILE_HOST=//p' "$TARGET/.env")
    [[ -n $COOKIE_FILE ]] || fail "Existing cookie-sync configuration has no output path."
    printf 'Existing Media Cookie Sync installation found; preserving its configuration, credential, and cookies.\n'
    "$TARGET/cookie-sync" up
    print_summary "$COOKIE_FILE"
    exit
fi

for tool in curl tar docker; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but was not found on PATH."
done
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required (docker compose)."
docker info >/dev/null 2>&1 || fail "Docker is installed but the daemon is not reachable."
[[ $(id -u) -ne 0 ]] || fail "Run this installer as an unprivileged user." "The cookie-sync container intentionally runs without root privileges."
[[ $IMAGE =~ ^[A-Za-z0-9][A-Za-z0-9._/:@-]*$ ]] || fail "COOKIE_SYNC_IMAGE is invalid."
case "$REF" in
    '' | /* | *..* | *[!A-Za-z0-9._/-]*) fail "Invalid MCB_INSTALL_REF: $REF" ;;
esac

TTY_OPEN=false
open_tty() {
    if [[ $TTY_OPEN == false ]]; then
        [[ -r /dev/tty ]] || fail "Interactive input is unavailable." "Set MCB_COOKIE_SYNC_READER_PASSWORD_FILE and the MCB_COOKIE_SYNC_* configuration variables."
        exec 3</dev/tty
        TTY_OPEN=true
    fi
}
prompt() {
    local variable=$1 label=$2 default=$3 value
    value=${!variable:-}
    if [[ -z $value ]]; then
        open_tty
        printf '%s [%s]: ' "$label" "$default" >/dev/tty
        IFS= read -r value <&3 || fail "Could not read $label."
        value=${value:-$default}
        printf -v "$variable" '%s' "$value"
    fi
}

printf 'Media Cookie Sync setup 🍪\n\n'
prompt MCB_COOKIE_SYNC_BROKER_URL 'Broker URL' 'http://127.0.0.1:8787'
prompt MCB_COOKIE_SYNC_USERNAME 'Reader username' 'downloader'
prompt MCB_COOKIE_SYNC_PROVIDER 'Provider' 'youtube'
prompt MCB_COOKIE_SYNC_PROFILE 'Profile' 'default'
prompt MCB_COOKIE_SYNC_OUTPUT_DIR 'Cookie output directory' "$HOME/media-cookies"

BROKER_URL=$MCB_COOKIE_SYNC_BROKER_URL
USERNAME=$MCB_COOKIE_SYNC_USERNAME
PROVIDER=$MCB_COOKIE_SYNC_PROVIDER
PROFILE=$MCB_COOKIE_SYNC_PROFILE
OUTPUT_DIR=$MCB_COOKIE_SYNC_OUTPUT_DIR
if [[ $OUTPUT_DIR == '~/'* ]]; then
    OUTPUT_DIR=$HOME/${OUTPUT_DIR#\~/}
fi
[[ $OUTPUT_DIR == /* && $OUTPUT_DIR != / && $OUTPUT_DIR != *$'\n'* && $OUTPUT_DIR != *$'\r'* && \
    $OUTPUT_DIR != *'$'* && $OUTPUT_DIR != *'#'* ]] || \
    fail "Cookie output directory must be an absolute non-root path without shell/config metacharacters."
[[ $TARGET != *$'\n'* && $TARGET != *$'\r'* && $TARGET != *'$'* && $TARGET != *'#'* ]] || \
    fail "Install path contains unsupported shell/config metacharacters."
[[ $USERNAME =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || fail "Reader username contains unsupported characters."
[[ $PROVIDER =~ ^[a-z0-9][a-z0-9_-]{0,31}$ ]] || fail "Provider is invalid."
[[ $PROFILE =~ ^[a-z0-9][a-z0-9._-]{0,63}$ ]] || fail "Profile is invalid."
case "$BROKER_URL" in
    https://*) LOOPBACK_BROKER=false ;;
    http://127.0.0.1 | http://127.0.0.1:* | http://localhost | http://localhost:*) LOOPBACK_BROKER=true ;;
    http://*) fail "Plain HTTP is allowed only for a loopback broker URL." "Use protected HTTPS or private networking for a remote consumer." ;;
    *) fail "Broker URL must use https://, or loopback http:// for a local broker." ;;
esac
[[ $BROKER_URL != *$'\n'* && $BROKER_URL != *$'\r'* && $BROKER_URL != *' '* && \
    $BROKER_URL != *'?'* && $BROKER_URL != *'#'* && $BROKER_URL != *'$'* ]] || \
    fail "Broker URL contains unsupported query, fragment, whitespace, or config metacharacters."
BROKER_AUTHORITY=${BROKER_URL#*://}
BROKER_AUTHORITY=${BROKER_AUTHORITY%%/*}
[[ -n $BROKER_AUTHORITY && $BROKER_AUTHORITY != *'@'* ]] || fail "Broker URL must not contain user information."

PASSWORD_SOURCE=${MCB_COOKIE_SYNC_READER_PASSWORD_FILE:-}
PASSWORD_TEMP=
trap 'if [[ -n ${PASSWORD_TEMP:-} ]]; then rm -f -- "$PASSWORD_TEMP"; fi' EXIT
if [[ -n $PASSWORD_SOURCE ]]; then
    [[ -f $PASSWORD_SOURCE && ! -L $PASSWORD_SOURCE && -s $PASSWORD_SOURCE ]] || fail "Reader password file is missing, empty, or unsafe."
else
    open_tty
    PASSWORD_TEMP=$(mktemp "${TMPDIR:-/tmp}/mcb-reader-password.XXXXXX")
    chmod 0600 "$PASSWORD_TEMP"
    printf 'Reader password: ' >/dev/tty
    IFS= read -r -s READER_PASSWORD <&3 || fail "Could not read the reader password."
    printf '\n' >/dev/tty
    [[ -n $READER_PASSWORD ]] || fail "Reader password cannot be empty."
    printf '%s\n' "$READER_PASSWORD" >"$PASSWORD_TEMP"
    unset READER_PASSWORD
    PASSWORD_SOURCE=$PASSWORD_TEMP
fi

mkdir -p -- "$OUTPUT_DIR"
[[ -d $OUTPUT_DIR && ! -L $OUTPUT_DIR ]] || fail "Cookie output path is not a safe directory: $OUTPUT_DIR"
if [[ $PROFILE == default ]]; then
    COOKIE_NAME=$PROVIDER.txt
else
    COOKIE_NAME=$PROVIDER-$PROFILE.txt
fi
COOKIE_FILE=$OUTPUT_DIR/$COOKIE_NAME

PARENT=$(dirname -- "$TARGET")
mkdir -p -- "$PARENT"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mcb-cookie-sync-install.XXXXXX")
ARCHIVE=$TEMP_DIR/source.tar.gz
LISTING=$TEMP_DIR/archive.list
SOURCE=$TEMP_DIR/source
STAGING=$(mktemp -d "$PARENT/.mcb-cookie-sync-staging.XXXXXX")
cleanup() {
    rm -rf -- "$TEMP_DIR"
    if [[ -n ${PASSWORD_TEMP:-} ]]; then
        rm -f -- "$PASSWORD_TEMP"
    fi
    if [[ -n ${STAGING:-} ]]; then
        rm -rf -- "$STAGING"
    fi
}
trap cleanup EXIT

URL=${MCB_SOURCE_ARCHIVE_URL:-https://github.com/$REPOSITORY/archive/$REF.tar.gz}
printf 'Preparing cookie-sync image...\n'
curl -fsSL --proto '=https' --tlsv1.2 "$URL" -o "$ARCHIVE"
tar -tzf "$ARCHIVE" >"$LISTING"
archive_root=
while IFS= read -r entry; do
    [[ -n $entry ]] || continue
    case "$entry" in
        /* | ../* | */../* | */..) fail "Downloaded archive contains an unsafe path." ;;
    esac
    top=${entry%%/*}
    if [[ -z $archive_root ]]; then
        archive_root=$top
    elif [[ $top != "$archive_root" ]]; then
        fail "Downloaded archive does not have one project root."
    fi
done <"$LISTING"
[[ -n $archive_root ]] || fail "Downloaded archive is empty."
mkdir -- "$SOURCE"
tar -xzf "$ARCHIVE" --strip-components=1 -C "$SOURCE"
[[ -f $SOURCE/Dockerfile && -f $SOURCE/go.mod && -d $SOURCE/cmd/cookie-sync ]] || \
    fail "Downloaded source cannot build cookie-sync."
docker build --target cookie-sync -t "$IMAGE" "$SOURCE" || fail "cookie-sync image build failed."

cp -- "$PASSWORD_SOURCE" "$STAGING/reader-password"
chmod 0600 "$STAGING/reader-password"
cat >"$STAGING/compose.yaml" <<'EOF'
name: media-cookie-sync

services:
  cookie-sync:
    image: ${COOKIE_SYNC_IMAGE:?}
    user: "${COOKIE_SYNC_UID:?}:${COOKIE_SYNC_GID:?}"
    environment:
      BROKER_URL: ${BROKER_URL:?}
      BROKER_USERNAME: ${BROKER_USERNAME:?}
      BROKER_PASSWORD_FILE: /run/secrets/reader-password
      COOKIE_SYNC_TARGETS: ${COOKIE_SYNC_TARGETS:?}
      COOKIE_SYNC_METADATA: "true"
      COOKIE_SYNC_INTERVAL: ${COOKIE_SYNC_INTERVAL:-5m}
    volumes:
      - type: bind
        source: ${COOKIE_OUTPUT_DIR:?}
        target: /cookies
      - type: bind
        source: ${PASSWORD_FILE:?}
        target: /run/secrets/reader-password
        read_only: true
    read_only: true
    tmpfs:
      - /tmp:rw,nosuid,nodev,noexec,size=8m
    restart: unless-stopped
    pids_limit: 32
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
EOF
if [[ $LOOPBACK_BROKER == true ]]; then
    printf '    network_mode: host\n' >>"$STAGING/compose.yaml"
fi
cat >"$STAGING/.env" <<EOF
COOKIE_SYNC_IMAGE=$IMAGE
COOKIE_SYNC_UID=$(id -u)
COOKIE_SYNC_GID=$(id -g)
BROKER_URL=$BROKER_URL
BROKER_USERNAME=$USERNAME
COOKIE_SYNC_TARGETS=$PROVIDER/$PROFILE=/cookies/$COOKIE_NAME
COOKIE_SYNC_INTERVAL=5m
COOKIE_OUTPUT_DIR=$OUTPUT_DIR
COOKIE_FILE_HOST=$COOKIE_FILE
PASSWORD_FILE=$TARGET/reader-password
EOF
chmod 0600 "$STAGING/.env"
cat >"$STAGING/cookie-sync" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
compose() {
    docker compose --env-file "$ROOT/.env" -f "$ROOT/compose.yaml" "$@"
}
case "${1:-status}" in
    up) shift; compose up -d "$@" ;;
    down) shift; compose down "$@" ;;
    status) shift; compose ps "$@" ;;
    logs) shift; compose logs --tail=200 "$@" cookie-sync ;;
    *) printf 'Usage: %s {up|down|status|logs [-f]}\n' "$0" >&2; exit 2 ;;
esac
EOF
chmod 0755 "$STAGING/cookie-sync"

docker compose --env-file "$STAGING/.env" -f "$STAGING/compose.yaml" config -q || \
    fail "Generated cookie-sync Compose configuration is invalid."
[[ ! -e $TARGET && ! -L $TARGET ]] || fail "Installation target appeared while preparing setup: $TARGET"
mv -- "$STAGING" "$TARGET"
STAGING=
"$TARGET/cookie-sync" up
print_summary "$COOKIE_FILE"
