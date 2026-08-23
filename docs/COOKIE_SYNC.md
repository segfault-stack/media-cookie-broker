# cookie-sync consumer integration

`cookie-sync` is the first-party bridge from authenticated broker responses to ordinary Netscape cookie files. It can run beside the broker or on any authorized consumer host that can reach the broker through loopback, protected HTTPS, or appropriate private networking.

## Installer behavior

The public installer:

```bash
curl -fsSL https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/main/install-cookie-sync.sh | bash
```

asks for one broker route, reader credential, provider/profile, and host output directory. It builds the dedicated `cookie-sync` Docker target from temporary source, then discards the source tree. Host Go is not required.

The permanent user-local runtime contains a Compose file, a small lifecycle helper, non-secret configuration, and a mode-`0600` reader password file. The password is mounted read-only at `/run/secrets/reader-password`; it is not stored in Compose YAML, the image, or generated commands. The cookie directory is the container's only persistent writable mount.

The container runs with the invoking unprivileged user's UID/GID, a read-only root filesystem, all capabilities dropped, and `no-new-privileges`. The installer refuses to run as root.

On Linux, the generated Compose service uses host networking when the broker URL is `http://127.0.0.1:...` or `http://localhost:...`, allowing the container to reach a broker or protected tunnel bound to host loopback. Accepted non-loopback HTTPS URLs keep ordinary container networking. Docker Desktop host networking has separate platform/version requirements, so the loopback installer default is intended for a Linux consumer host; use a protected non-loopback route where host networking is unavailable.

Rerunning the installer preserves the existing configuration, reader password, cookie files, and sidecars, then ensures the existing service is up. Reconfiguration and credential replacement are intentionally explicit rather than silent.

The installer prints exact helper paths for:

```text
cookie-sync up
cookie-sync down
cookie-sync status
cookie-sync logs
cookie-sync logs -f
```

## Connectivity

Use `http://127.0.0.1:8787` only when the sidecar runs on the broker host or has its own protected loopback tunnel. A consumer on another machine needs the broker's protected HTTPS or private-network route. The desktop SSH tunnel is local to the desktop and is not automatically available to a remote consumer.

Transfer only the scoped reader password to the consumer through a secure channel. The consumer never needs the publisher password or broker master key.

## Dedicated Docker target

Build the sidecar image independently:

```bash
docker build --target cookie-sync \
  -t media-cookie-broker-cookie-sync:preview .
```

The default/final `broker` target remains available as:

```bash
docker build --target broker -t media-cookie-broker:preview .
```

For manual Compose deployments, use the same hardening as the generated runtime: a non-root UID/GID, read-only root filesystem, dropped capabilities, `no-new-privileges`, a read-only reader password mount, and a single writable cookie-output mount.

## Runtime configuration

The container uses the existing `cookie-sync` configuration contract:

| Variable | Meaning | Default |
| --- | --- | --- |
| `BROKER_URL` | Reachable broker base URL | required |
| `BROKER_USERNAME` | Scoped reader username | required |
| `BROKER_PASSWORD_FILE` | In-container password-file path | required |
| `COOKIE_SYNC_TARGETS` | Comma-separated `provider[/profile]=/absolute/path` targets | required |
| `COOKIE_SYNC_METADATA` | Write revision/SHA sidecars | `true` |
| `COOKIE_SYNC_INTERVAL` | Poll interval, at least 10 seconds | `5m` |
| `COOKIE_SYNC_COMBINED` | Optional absolute combined-jar path | unset |

Examples:

```text
youtube=/cookies/youtube.txt
youtube/default=/cookies/youtube.txt
youtube/music-bot=/cookies/youtube-music.txt,tiktok/default=/cookies/tiktok.txt
```

Named profiles are logical broker sessions. Output paths must be unique and cannot collide with another target's `.meta.json` sidecar. A combined jar retains available last-known-good provider content when another target temporarily fails.

## Metadata and integrity

With metadata enabled, each cookie file has a `<file>.meta.json` sidecar containing provider, profile, revision, timestamps, and SHA-256. Cookie files, metadata, and combined jars are written atomically with mode `0600`.

Conditional requests use ETags. A `304 Not Modified` is accepted only while the local cookie bytes and trusted sidecar still match the in-memory broker-backed state. Missing or modified local files force one unconditional refetch. Invalid responses and transient broker failures never truncate the last-known-good file, and locally tampered bytes are never promoted to trusted metadata.

## Health reports

A broker-aware wrapper can report the result of using a particular local revision:

```bash
docker run --rm \
  -e BROKER_URL=https://broker.example \
  -e BROKER_USERNAME=downloader \
  -e BROKER_PASSWORD_FILE=/run/secrets/reader-password \
  -v /secure/reader-password:/run/secrets/reader-password:ro \
  -v /path/to/cookies:/cookies:ro \
  media-cookie-broker-cookie-sync:preview \
  report --provider youtube --profile default \
  --file /cookies/youtube.txt --kind authentication_required
```

Supported normalized kinds are `ok`, `authentication_required`, `access_denied`, `rate_limited`, and `unknown_failure`. Reporting fails closed unless the local file matches the SHA and revision in its sidecar.

## Direct binary use for developers

The Docker image is the normal distribution path. Developers can still build the binary directly with Go 1.24+:

```bash
go build -o bin/cookie-sync ./cmd/cookie-sync
```

Then provide the environment variables above and run `./bin/cookie-sync`. Direct-binary mode has the same ETag, SHA, tamper-recovery, and last-known-good semantics as the container.

## Troubleshooting

Start with `cookie-sync status` and `cookie-sync logs` using the exact helper path printed by the installer. Check that:

- the consumer can reach the configured broker route;
- the reader still exists and has a grant for the exact provider/profile;
- the reader password file is non-empty and mode `0600` on the host;
- the invoking UID/GID can write the cookie output directory;
- the media application reads the printed host path, not the container's `/cookies` path.

Stopping or recreating the sidecar does not erase existing cookie files. Preserve those files and their sidecars while investigating transient failures.
