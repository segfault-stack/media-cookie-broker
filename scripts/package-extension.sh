#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
EXTENSION_DIR=$ROOT/extension
DIST_DIR=${MCB_DIST_DIR:-$ROOT/dist}
VERSION=${1:-}

if [[ $# -gt 1 ]]; then
    printf 'Usage: scripts/package-extension.sh [version]\n' >&2
    exit 1
fi
if [[ -n $VERSION && ! $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
    printf 'Invalid extension package version: %s\n' "$VERSION" >&2
    exit 1
fi

for tool in zip unzip; do
    command -v "$tool" >/dev/null 2>&1 || {
        printf '%s is required to package the extension.\n' "$tool" >&2
        exit 1
    }
done
[[ -f $EXTENSION_DIR/manifest.json ]] || {
    printf 'Extension manifest is missing: %s/manifest.json\n' "$EXTENSION_DIR" >&2
    exit 1
}

if [[ -n $VERSION ]]; then
    FILENAME=media-cookie-broker-extension-$VERSION.zip
else
    FILENAME=media-cookie-broker-extension.zip
fi

mkdir -p -- "$DIST_DIR"
TEMP_DIR=$(mktemp -d "$DIST_DIR/.package-extension.XXXXXX")
trap 'rm -rf -- "$TEMP_DIR"' EXIT
TEMP_ARCHIVE=$TEMP_DIR/$FILENAME

(
    cd -- "$EXTENSION_DIR"
    find . -type f \
        ! -path './node_modules/*' \
        ! -path './tests/*' \
        ! -path './test-results/*' \
        ! -path './coverage/*' \
        ! -path './.nyc_output/*' \
        ! -name '.DS_Store' \
        ! -name '*.tmp' \
        ! -name '*.swp' \
        ! -name '*~' \
        -print | LC_ALL=C sort | zip -X -q "$TEMP_ARCHIVE" -@
)

[[ $(unzip -Z1 "$TEMP_ARCHIVE" | grep -c '^manifest.json$') -eq 1 ]] || {
    printf 'Packaged extension does not contain manifest.json at the archive root.\n' >&2
    exit 1
}

mv -f -- "$TEMP_ARCHIVE" "$DIST_DIR/$FILENAME"
printf 'Created %s\n' "$DIST_DIR/$FILENAME"
