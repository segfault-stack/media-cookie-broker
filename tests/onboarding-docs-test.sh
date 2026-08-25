#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
QUICK_START=$(sed -n '/^## .*Quick start/,/^---$/p' "$ROOT/README.md")

for required in install.sh install-extension.sh install-cookie-sync.sh \
    'v0.3.0-preview' 'ssh -N -L 8787:127.0.0.1:8787' 'http://127.0.0.1:8787'; do
    grep -Fq "$required" <<<"$QUICK_START"
done
if grep -Fq '/main/' <<<"$QUICK_START"; then
    printf 'Quick start executes an installer from the mutable main branch\n' >&2
    exit 1
fi
for internal in XDG_DATA_HOME 'go build' COOKIE_SYNC_TARGETS BROKER_PASSWORD_FILE; do
    if grep -Fq "$internal" <<<"$QUICK_START"; then
        printf 'Quick start contains implementation detail: %s\n' "$internal" >&2
        exit 1
    fi
done
grep -Fq 'docs/INSTALLATION.md' "$ROOT/README.md"
grep -Fq 'docs/COOKIE_SYNC.md' "$ROOT/README.md"
printf 'onboarding documentation checks passed\n'
