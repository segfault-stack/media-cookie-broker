#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repository_dir=$(cd -- "$script_dir/.." && pwd)
secrets_dir="$repository_dir/secrets"
master_key="$secrets_dir/master-key"
image_name=${COOKIE_BROKER_IMAGE:-media-cookie-broker:preview}

mkdir -p "$secrets_dir"
chmod 0700 "$secrets_dir"

if [[ -e $master_key ]]; then
    if [[ ! -f $master_key || ! -s $master_key ]]; then
        printf 'Refusing to replace invalid existing path: %s\n' "$master_key" >&2
        exit 1
    fi
    chmod 0444 "$master_key"
    printf 'Existing master key preserved; permissions set for local Compose.\n'
else
    temporary_key=$(mktemp "$secrets_dir/.master-key.XXXXXX")
    trap 'rm -f -- "$temporary_key"' EXIT
    docker run --rm --entrypoint brokerctl "$image_name" generate-key >"$temporary_key"
    if [[ ! -s $temporary_key ]]; then
        printf 'Master-key generation produced no output.\n' >&2
        exit 1
    fi
    chmod 0444 "$temporary_key"
    mv -- "$temporary_key" "$master_key"
    trap - EXIT
    printf 'Created secrets/master-key for the local Compose deployment.\n'
fi

printf '\nNext, create the publisher and reader identities:\n'
printf '  docker compose run --rm --entrypoint brokerctl broker user add browser-publisher --role publisher --provider youtube\n'
printf '  docker compose run --rm --entrypoint brokerctl broker user add downloader --role reader --provider youtube\n'
printf 'Store each generated password securely; it is shown only once.\n'
