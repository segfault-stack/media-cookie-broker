#!/usr/bin/env bash
set -euo pipefail

PROJECT=media-cookie-broker
REPOSITORY=segfault-stack/media-cookie-broker
REF=${MCB_INSTALL_REF:-main}
IMAGE=${COOKIE_BROKER_IMAGE:-media-cookie-broker:preview}
DATA_HOME=${XDG_DATA_HOME:-$HOME/.local/share}
TARGET=$DATA_HOME/$PROJECT

fail() {
    printf '✗ %s\n' "$1" >&2
    if [[ $# -gt 1 ]]; then
        printf '  %s\n' "$2" >&2
    fi
    exit 1
}

if [[ -e $TARGET || -L $TARGET ]]; then
    [[ -d $TARGET && ! -L $TARGET && -x $TARGET/mcb ]] || \
        fail "Refusing to use an invalid existing installation at $TARGET."
    printf 'Existing Media Cookie Broker installation found; preserving its data and credentials.\n\n'
    (cd -- "$TARGET" && COOKIE_BROKER_IMAGE="$IMAGE" ./mcb setup)
    exit
fi

for tool in curl tar docker; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but was not found on PATH."
done
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required (docker compose)."
docker info >/dev/null 2>&1 || fail "Docker is installed but the daemon is not reachable."

case "$REF" in
    '' | /* | *..* | *[!A-Za-z0-9._/-]*) fail "Invalid MCB_INSTALL_REF: $REF" ;;
esac

PARENT=$(dirname -- "$TARGET")
mkdir -p -- "$PARENT"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mcb-server-install.XXXXXX")
ARCHIVE=$TEMP_DIR/source.tar.gz
LISTING=$TEMP_DIR/archive.list
SOURCE=$TEMP_DIR/source
STAGING=
cleanup() {
    rm -rf -- "$TEMP_DIR"
    if [[ -n $STAGING ]]; then
        rm -rf -- "$STAGING"
    fi
}
trap cleanup EXIT

URL=${MCB_SOURCE_ARCHIVE_URL:-https://github.com/$REPOSITORY/archive/$REF.tar.gz}
printf 'Downloading Media Cookie Broker...\n'
curl -fsSL --proto '=https' --tlsv1.2 "$URL" -o "$ARCHIVE"
tar -tzf "$ARCHIVE" >"$LISTING"

archive_root=
while IFS= read -r entry; do
    [[ -n $entry ]] || continue
    case "$entry" in
        /* | ../* | */../* | */..) fail "Downloaded archive contains an unsafe path." ;;
    esac
    top=${entry%%/*}
    [[ -n $top && $top != . && $top != .. ]] || fail "Downloaded archive has an invalid root."
    if [[ -z $archive_root ]]; then
        archive_root=$top
    elif [[ $top != "$archive_root" ]]; then
        fail "Downloaded archive does not have one project root."
    fi
done <"$LISTING"
[[ -n $archive_root ]] || fail "Downloaded archive is empty."

mkdir -- "$SOURCE"
tar -xzf "$ARCHIVE" --strip-components=1 -C "$SOURCE" || fail "Could not extract Media Cookie Broker source."
for path in mcb Dockerfile docker-compose.yml scripts/bootstrap-compose.sh; do
    [[ -e $SOURCE/$path ]] || fail "Downloaded source is missing required file: $path"
done

printf 'Building broker image...\n'
docker build --target broker -t "$IMAGE" "$SOURCE" || fail "Broker image build failed."

STAGING=$(mktemp -d "$PARENT/.mcb-server-staging.XXXXXX")
mkdir -p -- "$STAGING/scripts" "$STAGING/secrets"
cp -- "$SOURCE/mcb" "$STAGING/mcb"
cp -- "$SOURCE/docker-compose.yml" "$STAGING/docker-compose.yml"
cp -- "$SOURCE/scripts/bootstrap-compose.sh" "$STAGING/scripts/bootstrap-compose.sh"
chmod 0755 "$STAGING/mcb" "$STAGING/scripts/bootstrap-compose.sh"
chmod 0700 "$STAGING/secrets"

[[ ! -e $TARGET && ! -L $TARGET ]] || fail "Installation target appeared while preparing setup: $TARGET"
mv -- "$STAGING" "$TARGET"
STAGING=

(cd -- "$TARGET" && COOKIE_BROKER_IMAGE="$IMAGE" ./mcb setup)
