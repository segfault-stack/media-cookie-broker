#!/usr/bin/env bash
set -euo pipefail

PROJECT=media-cookie-broker
REPOSITORY=segfault-stack/media-cookie-broker
REF=${MCB_INSTALL_REF:-main}
DATA_HOME=${XDG_DATA_HOME:-$HOME/.local/share}
TARGET=$DATA_HOME/$PROJECT

fail() {
    printf '✗ %s\n' "$1" >&2
    if [[ $# -gt 1 ]]; then
        printf '  %s\n' "$2" >&2
    fi
    exit 1
}

for tool in curl tar docker; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but was not found on PATH."
done
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required (docker compose)."

case "$REF" in
    '' | /* | *..* | *[!A-Za-z0-9._/-]*) fail "Invalid MCB_INSTALL_REF: $REF" ;;
esac

if [[ -e $TARGET || -L $TARGET ]]; then
    printf 'Media Cookie Broker is already present at:\n  %s\n\n' "$TARGET" >&2
    printf 'Continue or repair setup with:\n  %s/mcb setup\n\n' "$TARGET" >&2
    printf 'Check an existing setup with:\n  %s/mcb status\n' "$TARGET" >&2
    exit 1
fi

PARENT=$(dirname -- "$TARGET")
mkdir -p -- "$PARENT"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mcb-install.XXXXXX")
ARCHIVE=$TEMP_DIR/source.tar.gz
LISTING=$TEMP_DIR/archive.list
STAGING=
cleanup() {
    rm -rf -- "$TEMP_DIR"
    if [[ -n $STAGING ]]; then
        rm -rf -- "$STAGING"
    fi
}
trap cleanup EXIT

URL=https://github.com/$REPOSITORY/archive/$REF.tar.gz
printf 'Downloading Media Cookie Broker (%s)...\n' "$REF"
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

STAGING=$(mktemp -d "$PARENT/.mcb-staging.XXXXXX")
if ! tar -xzf "$ARCHIVE" --strip-components=1 -C "$STAGING"; then
    fail "Could not extract Media Cookie Broker into $TARGET."
fi
[[ -x $STAGING/mcb && -f $STAGING/docker-compose.yml ]] || fail "Downloaded source does not contain the expected Media Cookie Broker files."
[[ ! -e $TARGET && ! -L $TARGET ]] || fail "Installation target appeared while downloading: $TARGET"
mv -- "$STAGING" "$TARGET"
STAGING=

printf 'Installed source at:\n  %s\n\n' "$TARGET"
(cd -- "$TARGET" && ./mcb setup)

printf '\nInstallation directory\n  %s\n' "$TARGET"
printf '\nExtension path\n  %s/extension\n' "$TARGET"
printf '\nUseful commands\n'
printf '  %s/mcb status\n' "$TARGET"
printf '  %s/mcb doctor\n' "$TARGET"
printf '\nPreview updates are manual. Review newer source files, preserve %s/secrets/,\n' "$TARGET"
printf 'replace the installed source deliberately, then run %s/mcb setup --rebuild.\n' "$TARGET"
