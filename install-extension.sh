#!/usr/bin/env bash
set -euo pipefail

REPOSITORY=segfault-stack/media-cookie-broker
REF=${MCB_INSTALL_REF:-v0.3.0-preview}
DATA_HOME=${XDG_DATA_HOME:-$HOME/.local/share}
BASE=$DATA_HOME/media-cookie-broker-extension
TARGET=$BASE/current

fail() {
    printf '✗ %s\n' "$1" >&2
    exit 1
}

for tool in curl tar; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but was not found on PATH."
done
case "$REF" in
    '' | /* | *..* | *[!A-Za-z0-9._/-]*) fail "Invalid MCB_INSTALL_REF: $REF" ;;
esac
if [[ -e $TARGET || -L $TARGET ]]; then
    [[ -d $TARGET && ! -L $TARGET && -f $TARGET/manifest.json ]] || \
        fail "Refusing to replace an invalid existing extension installation at $TARGET."
fi

mkdir -p -- "$BASE"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mcb-extension-install.XXXXXX")
ARCHIVE=$TEMP_DIR/source.tar.gz
LISTING=$TEMP_DIR/archive.list
SOURCE=$TEMP_DIR/source
STAGING=$(mktemp -d "$BASE/.extension-staging.XXXXXX")
cleanup() {
    rm -rf -- "$TEMP_DIR"
    if [[ -n ${STAGING:-} ]]; then
        rm -rf -- "$STAGING"
    fi
}
trap cleanup EXIT

URL=${MCB_SOURCE_ARCHIVE_URL:-https://github.com/$REPOSITORY/archive/$REF.tar.gz}
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
tar -xzf "$ARCHIVE" --strip-components=1 -C "$SOURCE" || fail "Could not extract the extension source."
[[ -f $SOURCE/extension/manifest.json ]] || fail "Downloaded source is missing extension/manifest.json."

(
    cd -- "$SOURCE/extension"
    tar -cf - \
        --exclude='./node_modules' \
        --exclude='./tests' \
        --exclude='./test-results' \
        --exclude='./coverage' \
        --exclude='./package.json' \
        --exclude='.DS_Store' \
        .
) | tar -xf - -C "$STAGING"

[[ -s $STAGING/manifest.json ]] || fail "Prepared extension has no manifest.json at its root."
grep -q '"manifest_version"' "$STAGING/manifest.json" || fail "Prepared extension manifest is invalid."
for unwanted in node_modules tests test-results coverage package.json; do
    [[ ! -e $STAGING/$unwanted ]] || fail "Prepared extension contains development-only files."
done

BACKUP=
if [[ -d $TARGET ]]; then
    BACKUP=$BASE/previous-$(date +%s)-$$
    mv -- "$TARGET" "$BACKUP"
fi
if ! mv -- "$STAGING" "$TARGET"; then
    if [[ -n $BACKUP ]]; then
        mv -- "$BACKUP" "$TARGET"
    fi
    fail "Could not activate the prepared extension; the previous installation was restored."
fi
STAGING=

# Keep the just-replaced installation for rollback, but discard older successful
# update backups so repeated installs cannot grow this directory without bound.
for previous in "$BASE"/previous-*; do
    [[ -d $previous && ! -L $previous ]] || continue
    [[ -n $BACKUP && $previous == "$BACKUP" ]] || rm -rf -- "$previous"
done

printf 'Media Cookie Broker extension ready 🍪\n\n'
printf 'Load unpacked:\n  %s\n\n' "$TARGET"
printf 'Then:\n'
printf '  1. Open chrome://extensions\n'
printf '  2. Enable Developer mode\n'
printf '  3. Click Load unpacked\n'
printf '  4. Select the directory above\n'
printf '  5. Enable Allow in incognito\n'
