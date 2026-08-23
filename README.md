# Media Cookie Broker

Media Cookie Broker is a human-in-the-loop authentication maintenance system for unattended media software.

Your downloader, bot, NAS, or service can often run for weeks with an ordinary Netscape `cookies.txt` file. Eventually a provider may require interactive login, CAPTCHA, 2FA, or another browser-only step. Keeping a full browser and VNC session beside every consumer is cumbersome. Media Cookie Broker moves that occasional human step to the operator's everyday Chromium browser.

Broker-aware consumers can report that the exact session revision they used now requires authentication. The browser extension polls the broker, notifies the operator, shows which provider/profile needs attention, and starts a provider-specific recovery flow only when the human selects **Refresh session**. A fresh encrypted revision is then redistributed as an ordinary Netscape file.

> **Early public preview:** this software handles sensitive bearer credentials and has not been independently audited. Read the [security model](SECURITY.md) before exposing it outside a private development environment.

## Architecture

```text
                 USER'S BROWSER
          ┌────────────────────────┐
          │ extension              │
          │ status / notifications │
          │ provider/profile       │
          │ interactive refresh    │
          └───────────┬────────────┘
                      │ fresh snapshot
                      ▼
               ┌─────────────┐
               │   broker    │
               │ profiles    │
               │ revisions   │
               │ users / ACL │
               │ health      │
               └──────┬──────┘
                      │ Netscape file + optional metadata
              ┌───────┴────────┐
              ▼                ▼
         cookie-sync      broker-aware app
              │                │
              ▼                │ normalized health report
   file-oriented software ──────┘
```

The broker stores AES-256-GCM-encrypted cookie snapshots, bounded revision history, profile-scoped user grants, consumer activity, normalized health reports, and optional encrypted diagnostics. The master key remains outside SQLite.

Session identity is `(provider, profile)`. A provider may have independent sessions such as:

```text
youtube/default
youtube/music-bot
youtube/private-account
```

`default` preserves the simple one-session workflow. Provider-only API routes, grants, and sync targets always mean `profile=default`; they never grant wildcard access to every profile. Profile persistence and ACLs are implemented, while profile-management UX is intentionally minimal in this preview.

## Why static cookie files are fragile

A copied `cookies.txt` is a snapshot of browser state, not a renewable credential. Providers rotate or invalidate state, account activity can affect it, and manually replacing files on remote hosts risks partial writes and inconsistent versions. Static files also cannot tell an operator which consumer saw an authentication failure or which revision that consumer actually used.

Media Cookie Broker does not replace Netscape files. It makes their creation, revisioning, atomic distribution, and optional health feedback more reliable.

## Two integration levels

### File-only compatibility

No application changes are required. `cookie-sync` periodically downloads a standard Netscape file, uses ETags, and replaces it with a mode-`0600` fsync-and-rename operation. Failed fetches and invalid responses leave the last-known-good file untouched.

By default, each per-profile file also gets a private non-secret sidecar such as `youtube.txt.meta.json` containing provider, profile, revision, timestamps, and the cookie file's SHA-256. Set `COOKIE_SYNC_METADATA=false` to disable sidecar writes. Combined jars intentionally have no single-revision sidecar.

### Broker-aware health reporting

An application or wrapper that recognizes its own authentication failure can submit a normalized report. The broker remains application-agnostic: it does not parse yt-dlp output or accept raw stderr.

The first-party helper verifies that the current cookie file still matches its sidecar before reporting the exact revision:

```bash
cookie-sync report \
  --provider youtube \
  --profile default \
  --file /run/media-cookie-broker/youtube.txt \
  --kind authentication_required
```

Accepted kinds are `ok`, `authentication_required`, `access_denied`, `rate_limited`, and `unknown_failure`. The authenticated reader username is the consumer identity; callers cannot supply another identity in JSON. The latest report wins per consumer/provider/profile/revision. An `ok` report clears only that same consumer's report for that revision. In this preview, one current-revision `authentication_required` report requests a human refresh; other failure kinds remain diagnostic signals.

Only reports from existing reader users with a current grant to that exact provider/profile affect active health. Revoking a grant immediately retires that consumer's reports from active aggregation without deleting report or activity history. Re-granting does not reactivate a stale report; the consumer must submit a fresh report.

## Browser extension: the human control plane

The unpacked Manifest V3 extension stores a dedicated publisher credential, polls publisher-visible profile status every 2–5 minutes, and distinguishes broker connectivity from provider authentication health. It notifies once on meaningful health transitions and once per crossed auth-expiry threshold/revision. Thresholds default to `24, 6, 1` hours and can be edited or disabled in Settings.

Clicking a notification opens the extension's status/action UI. It never starts login automatically. Recovery begins only after the operator selects **Refresh session**.

The broker exposes an auth-expiry hint only when provider policy identifies a relevant authentication cookie with an explicit expiration. The hint is not a guarantee that a session will fail at that time; unrelated short-lived cookies are ignored.

The extension keeps bounded, redacted local diagnostics across service-worker restarts. Remote upload is off by default and requires explicit enablement in Settings. Local history is richer: only events with a valid provider/profile scope are eligible for remote upload, while generic control-plane events remain local-only. Uploaded events are validated and encrypted by the broker.

## YouTube recovery workflow

YouTube is the primary provider and uses an isolated incognito recovery flow by default, following the durable-cookie workflow recommended by yt-dlp:

```text
consumer reports authentication_required for youtube/default revision 17
→ extension observes refresh_required and notifies the operator
→ notification opens status UI focused on YouTube/default
→ operator selects Refresh session
→ extension verifies incognito access and that no incognito window/flow already exists
→ broker-created incognito window → interactive Google/YouTube login
→ youtube.com/robots.txt → capture only youtube.com cookies
→ publish revision 18 → close only the broker-created incognito window
→ old revision-17 reports no longer affect current health
→ cookie-sync atomically distributes revision 18
```

Chrome uses one shared cookie store for all incognito windows. The extension therefore refuses isolated recovery unless **Allow in incognito** is enabled, every existing incognito window is closed, and no other isolated broker recovery is running. It never closes unrelated incognito windows. Avoid opening another incognito window until recovery completes; if one appears, the extension records a warning and does not claim the shared incognito session was destroyed.

The extension uses Chromium's spanning incognito mode so one control-plane process can track the broker-created window and tab. It resolves the cookie store from that tracked tab instead of treating the service worker's ambient context as the recovery identity.

Settings can explicitly disable isolated recovery. Normal mode opens a window backed by the operator's current ordinary browser cookie session, still captures only the provider's declared cookie scope, and never auto-closes the normal window. Broker profiles such as `youtube/personal` and `youtube/music-bot` remain broker labels; they are not separate normal-browser cookie stores. Changing accounts in normal mode can affect everyday browsing and other broker-profile refreshes.

The project does not automate Google login, CAPTCHA, 2FA, or upstream anti-bot controls, and it cannot make a closed browser session permanent.

## Minimal local deployment

Requirements: Docker with Compose and a Chromium-based browser. Go 1.24+ is needed for host builds and development.

### 1. Build and create the master key

```bash
docker build -t media-cookie-broker:preview .
./scripts/bootstrap-compose.sh
```

The bootstrap creates `secrets/` with mode `0700` and, only when absent, generates `secrets/master-key` with mode `0444`. It never prints or overwrites the key and is safe to run again. The apparently broad key mode is intentional for this local file-backed Compose workflow: the non-root `broker` user must be able to read the bind-mounted source file. Other ordinary host users cannot traverse the `0700` parent directory. Compose file-backed secrets are still bind-mounted host files, not a full secret-management system; production deployments may supply `/run/secrets/master-key` through a stronger platform mechanism.

Users and profile-scoped grants live in SQLite and are managed locally with `brokerctl`. Initialize the publisher and reader identities in the Compose data volume:

```bash
COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose run --rm \
  --entrypoint brokerctl broker \
  user add browser-publisher --role publisher --provider youtube

COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose run --rm \
  --entrypoint brokerctl broker \
  user add downloader --role reader --provider youtube
```

Each command generates and prints a one-time random password. Save it immediately in a password manager or another appropriately protected secret store; only its Argon2id hash is stored in SQLite. Enter the publisher password in the extension. If the host-run `cookie-sync` example below will use a password file, create `secrets/reader-password` with mode `0600` and place only the generated downloader password in it, for example with a local editor:

```bash
install -m 600 /dev/null secrets/reader-password
${EDITOR:-vi} secrets/reader-password
chmod 600 secrets/reader-password
```

This `0600` reader-password file is consumed by a process running as your host user; it is not bind-mounted into the non-root container. To add a named profile grant later:

```bash
docker compose run --rm --entrypoint brokerctl broker \
  user grant browser-publisher --provider youtube --profile music-bot
docker compose run --rm --entrypoint brokerctl broker \
  user grant downloader --provider youtube --profile music-bot
```

The other local commands are `user list`, `user delete`, `user passwd`, and `user revoke`. There is deliberately no HTTP admin API or web admin panel.

### 2. Start the broker

```bash
COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose up -d
curl http://127.0.0.1:8787/healthz
```

Compose binds the API to loopback and stores SQLite in a named volume. For a remote browser, put the broker behind HTTPS and appropriate network controls. [`deploy/nginx.conf`](deploy/nginx.conf) is a placeholder whose example hostname and certificate paths must be changed.

### 3. Configure the extension

1. Open `chrome://extensions`, enable Developer mode, and load `extension/` unpacked.
2. Open Details and enable **Allow in incognito** for the default YouTube recovery flow.
3. In Settings, enter the broker URL, `browser-publisher`, and its plaintext publisher password.
4. Chrome requests access only to that broker origin. The extension ships with no configured endpoint.
5. Keep the default `youtube/default` profile or enter a named profile that the publisher was granted.

Remote endpoints must use HTTPS. Loopback development may use `http://localhost`, `http://127.0.0.1`, or `http://[::1]`.

### 4. Publish and synchronize

Use the popup's YouTube/default **Refresh session** action and complete the interactive flow. Then run the file consumer:

```bash
go build -o bin/cookie-sync ./cmd/cookie-sync
BROKER_URL=http://127.0.0.1:8787 \
BROKER_USERNAME=downloader \
BROKER_PASSWORD_FILE=secrets/reader-password \
COOKIE_SYNC_TARGETS=youtube=/tmp/media-cookies/youtube.txt \
./bin/cookie-sync
```

A named target uses `provider/profile=/absolute/path`:

```bash
COOKIE_SYNC_TARGETS='youtube/default=/run/cookies/default.txt,youtube/music-bot=/run/cookies/music.txt'
```

`COOKIE_SYNC_INTERVAL` defaults to `5m` and must be at least `10s`. `COOKIE_SYNC_COMBINED=/absolute/path/cookies.txt` optionally builds one combined jar from all available last-known-good targets.

## API and operations

Default-profile shorthand routes:

- `PUT /v1/providers/{provider}/cookies`;
- `GET /v1/providers/{provider}/cookies.txt`;
- `GET /v1/providers/{provider}/status`;
- `POST /v1/providers/{provider}/reports`.

Canonical named-profile routes insert `/profiles/{profile}` before the resource. `GET /v1/status` returns all scopes visible to an authenticated publisher for extension polling. Cookie responses retain `ETag`, `X-Cookie-Revision`, capture/create timestamps, and now include provider/profile headers.

Cookie uploads carry a small semantic `publication_reason`: `ordinary` publications deduplicate identical canonical cookie material, while a successful explicit `recovery` publication creates a new revision even when the bytes are unchanged. This advances the authentication lifecycle so failures attached to the preceding revision no longer affect current health.

Rollback is profile-aware and defaults to `default`:

```bash
docker compose exec broker brokerctl rollback --provider youtube --profile default --revision 1
```

User and rollback commands operate directly on SQLite and take effect without restarting the broker. Encrypted revision history remains bounded to five snapshots per provider/profile.

## Provider status

| Provider | Recovery/capture mode | Preview status |
| --- | --- | --- |
| YouTube | interactive incognito by default; optional normal window | primary workflow; provider auth-expiry hint supported |
| TikTok | experimental regular-browser capture | storage/distribution works; downloader behavior not broadly validated |
| Instagram | experimental regular-browser capture | storage/distribution works; downloader behavior not broadly validated |
| X / Twitter | experimental regular-browser capture | storage/distribution works; downloader behavior not broadly validated |

Provider support means the broker validates, stores, and distributes the declared cookie scope. It does not promise that every account state or third-party downloader accepts it indefinitely. Provider policy remains source-defined in `internal/providers/` and `extension/providers/`; there is no runtime plugin system.

## Security assumptions

- Cookies are bearer credentials; a valid snapshot may grant the same access as the browser session.
- Use long, separate users with the minimum role and exact provider/profile grants.
- Use HTTPS for every non-loopback connection.
- Protect extension local storage because it contains the dedicated publisher password.
- Protect the master key, SQLite volume, password files, sidecars, and synchronized cookie files.
- AES-GCM protects a stolen database without the master key, not a broker host compromised with both.
- Diagnostics must never contain cookie values, passwords, Authorization headers, account secrets, or master keys.

See [SECURITY.md](SECURITY.md) for the full preview threat model.

## Development checks

```bash
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
npm --prefix extension test
docker compose config
docker build -t media-cookie-broker:preview .
COOKIE_BROKER_TEST_IMAGE=media-cookie-broker:preview tests/container-smoke.sh
```

## Preview boundaries

File-only consumers cannot report failures unless an application or wrapper invokes the report API. One authentication-required report currently requests refresh; quorum policy and post-refresh consumer verification are deferred. Profile support is foundational, but profile creation/deletion and account labels do not yet have a polished UI. Runtime plugins, a web admin panel, OAuth, login automation, HA/replication, Firefox packaging, and generic non-cookie browser state are intentionally out of scope.

See [docs/KNOWN_LIMITATIONS.md](docs/KNOWN_LIMITATIONS.md), [docs/PROVIDERS.md](docs/PROVIDERS.md), and [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
