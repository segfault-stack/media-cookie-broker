#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

if [[ -x $SCRIPT_DIR/install.sh ]]; then
    exec "$SCRIPT_DIR/install.sh" "$@"
fi

printf 'install-server.sh is deprecated; running the canonical install.sh.\n' >&2
curl -fsSL https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/main/install.sh | bash -s -- "$@"
