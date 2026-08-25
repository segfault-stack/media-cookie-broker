# Installation and server operations

The main README covers the shortest safe first run. This page explains what the installers keep, how reruns behave, and how to operate a source checkout.

## Inspect before running

To review the canonical server installer first:

```bash
curl -fsSLO https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install.sh
less install.sh
bash install.sh
```

The public installers default to the same `v0.3.0-preview` source tag. Set `MCB_INSTALL_REF` only when you deliberately want to test another reviewed tag or commit.

`install-server.sh` is only a compatibility wrapper. All server installation logic lives in `install.sh`.

## Requirements and layout

The server installer requires an unprivileged account with access to a running Docker daemon, Docker Compose v2, `curl`, and `tar`. It does not use `sudo`, install systemd units, edit shell startup files, or install browser files.

It downloads source into a temporary directory, builds the local `media-cookie-broker:preview` image, and permanently keeps only:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/media-cookie-broker/
  mcb
  docker-compose.yml
  scripts/bootstrap-compose.sh
  secrets/
```

Docker keeps the SQLite database in the `broker-data` volume. The Compose port mapping remains `127.0.0.1:8787:8787` unless an operator explicitly changes it.

## Idempotency and credentials

On the first setup, `mcb`:

- creates a master key only when none exists;
- creates `browser-publisher` and prints its generated password once;
- creates `downloader` and stores its password in a mode-`0600` file;
- starts the broker and verifies `/healthz`.

On a rerun, the installer detects the existing runtime and runs its existing `mcb setup`. It does not replace the runtime, rebuild an already available image, overwrite the master key, recreate users, rotate credentials, or delete snapshots. The existing database volume and secret files remain in place.

Generated publisher plaintext cannot be recovered later. If it was lost, reset it explicitly from the installed runtime:

```bash
docker compose run --rm --no-deps --entrypoint brokerctl broker \
  user passwd browser-publisher
```

If the reader password file was lost, reset the `downloader` credential explicitly before configuring a new consumer. Never substitute the master key for a reader credential.

## Source-tree setup

Developers can run the same server helper from a checkout:

```bash
git clone https://github.com/segfault-stack/media-cookie-broker.git
cd media-cookie-broker
./mcb setup
```

Source mode can build the image and supports `./mcb setup --rebuild`. Developers may load `./extension` directly in Chromium.

## Desktop extension layout and updates

`install-extension.sh` downloads source temporarily and copies only extension runtime files into:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/media-cookie-broker-extension/current
```

It stages and validates `manifest.json` before replacing `current`. A failed download, extraction, or validation leaves the previous good directory untouched. After a successful replacement, the former directory is retained under a timestamped `previous-*` name for manual rollback and any older backup is removed. The installer never edits Chromium configuration or policies.

## Persistent remote access

The preview defaults to a loopback broker plus an SSH tunnel. For unattended remote consumers or a persistent desktop connection, use a protected HTTPS reverse proxy or suitable private networking. Do not expose the broker as public plain HTTP: Basic-auth credentials and cookie material are bearer secrets.

The repository includes `deploy/nginx.conf` as a starting point. TLS termination, DNS, firewall policy, and private-network configuration remain operator responsibilities.

## Troubleshooting and removal

Use the exact command path printed by setup:

```text
mcb status
mcb logs
mcb doctor
```

`mcb down` stops containers without deleting the database volume or secrets. Before uninstalling, back up the master key, SQLite volume, and any credentials you still need. Removing only the user-local runtime does not remove the Docker volume; removing that volume is a separate destructive action.

Preview updates are manual. Review a new installer and release notes before replacing runtime files or rebuilding images.
