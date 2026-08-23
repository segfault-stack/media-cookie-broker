#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

if [[ -x $SCRIPT_DIR/install-server.sh ]]; then
    exec "$SCRIPT_DIR/install-server.sh" "$@"
fi

printf 'install.sh is a compatibility wrapper.\n' >&2
printf 'Download and run install-server.sh from the same Media Cookie Broker release.\n' >&2
exit 1
