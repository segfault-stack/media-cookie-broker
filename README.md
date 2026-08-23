# Media Cookie Broker

> 🍪 **Keep unattended media software logged in without babysitting a browser on every server.**

[![Release](https://img.shields.io/github/v/release/segfault-stack/media-cookie-broker?include_prereleases&sort=semver&label=release)](https://github.com/segfault-stack/media-cookie-broker/releases)
[![License](https://img.shields.io/github/license/segfault-stack/media-cookie-broker)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Chrome MV3](https://img.shields.io/badge/Chromium-MV3-4285F4?logo=googlechrome&logoColor=white)

**Media Cookie Broker** is a human-in-the-loop authentication control plane for remote downloaders, bots, NAS boxes, and media services.

Your software keeps using an ordinary Netscape `cookies.txt`. When that browser session eventually needs a human again — login, CAPTCHA, 2FA, account confirmation, whatever — the broker tells your everyday Chromium browser:

> **“This session needs attention.”**

You click **Refresh session**, complete the provider's normal browser flow, and a fresh cookie revision is pushed back to consumers.

**No permanent VNC session. No manual cookie shuffling. No pretending browser sessions are immortal.**

> ⚠️ **Early public preview.** Cookies are bearer credentials. Read [SECURITY.md](SECURITY.md) before exposing a broker outside a private development environment.

---

## 🚀 Quick start

Requirements:

- Docker + Docker Compose
- Chromium / Chrome / another Chromium-based browser
- Go 1.24+ only for host builds and development

### 1. Build + bootstrap

```bash
git clone https://github.com/segfault-stack/media-cookie-broker.git
cd media-cookie-broker

docker build -t media-cookie-broker:preview .
./scripts/bootstrap-compose.sh
```

Create a publisher for the browser extension and a reader for the consumer:

```bash
COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose run --rm \
  --entrypoint brokerctl broker \
  user add browser-publisher --role publisher --provider youtube

COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose run --rm \
  --entrypoint brokerctl broker \
  user add downloader --role reader --provider youtube
```

Each command prints a **one-time random password**. Save both.

For host-side `cookie-sync`, put the reader password in a protected file:

```bash
install -m 600 /dev/null secrets/reader-password
${EDITOR:-vi} secrets/reader-password
chmod 600 secrets/reader-password
```

### 2. Start the broker

```bash
COOKIE_BROKER_IMAGE=media-cookie-broker:preview docker compose up -d

curl http://127.0.0.1:8787/healthz
```

Expected:

```json
{"status":"ok"}
```

### 3. Load the extension

1. Open `chrome://extensions`.
2. Enable **Developer mode**.
3. Click **Load unpacked** → select `extension/`.
4. Open the extension's **Details** → enable **Allow in incognito**.
5. Open **Settings and guide**.
6. Enter:
   - broker URL: `http://127.0.0.1:8787`
   - username: `browser-publisher`
   - publisher password

Remote broker URLs must use HTTPS. Loopback development may use plain HTTP.

### 4. Publish your first session

Open the extension popup:

> **YouTube / default → Refresh session**

Complete the normal Google / YouTube login flow.

The extension captures the YouTube-scoped browser state, publishes it to the broker, and the popup should end up showing something like:

```text
Healthy · revision 1
```

### 5. Sync an ordinary `cookies.txt`

```bash
go build -o bin/cookie-sync ./cmd/cookie-sync
mkdir -p /tmp/media-cookies

BROKER_URL=http://127.0.0.1:8787 \
BROKER_USERNAME=downloader \
BROKER_PASSWORD_FILE=secrets/reader-password \
COOKIE_SYNC_TARGETS='youtube/default=/tmp/media-cookies/youtube.txt' \
./bin/cookie-sync
```

You now have:

```text
/tmp/media-cookies/youtube.txt
/tmp/media-cookies/youtube.txt.meta.json
```

Point any compatible software at `youtube.txt`.

That's the whole basic loop.

---

## 🧠 The idea in one picture

```text
unattended consumer
        │
        │ "auth no longer works"
        ▼
┌─────────────────┐
│     broker      │
│ health/revision │
└────────┬────────┘
         │ extension polls
         ▼
┌──────────────────────┐
│ your everyday browser│
│ 🔔 needs attention   │
│ 👤 Refresh session   │
└──────────┬───────────┘
           │ interactive login
           ▼
┌──────────────────────┐
│ encrypted snapshot   │
└──────────┬───────────┘
           │
           ▼
      cookie-sync
           │
           ▼
       cookies.txt
           │
           ▼
   existing software
```

The useful trick is that **the consumer does not have to understand Media Cookie Broker**.

If it already accepts Netscape `cookies.txt`, the compatibility layer can keep that file updated. Broker-aware consumers can additionally report authentication health so the extension knows when a human is actually needed.

---

## 🤔 Why not just copy cookies?

Because a cookie jar is a **snapshot**, not a permanent credential.

Eventually providers rotate or invalidate state. Some flows require:

- interactive login;
- CAPTCHA;
- 2FA;
- account confirmation;
- another browser-only step.

The usual alternatives are annoying:

- run a browser beside every downloader;
- keep VNC/RDP access to every remote machine;
- manually export and SCP cookie files;
- discover broken authentication only after jobs have been failing for hours.

Media Cookie Broker moves the rare human step back to the browser where the human already lives.

---

## 🕹️ Browser = human control plane

The Manifest V3 extension:

- polls broker status every few minutes;
- distinguishes broker connectivity from provider authentication health;
- shows provider/profile state;
- sends system notifications on meaningful health transitions;
- starts recovery **only after you click `Refresh session`**;
- keeps bounded redacted local diagnostics;
- can optionally upload scoped encrypted diagnostics.

Clicking a notification opens the status/action UI first.

**It never starts an interactive login automatically.**

---

## 🥷 YouTube recovery

YouTube is the primary preview workflow.

Default recovery is isolated:

```text
consumer reports authentication_required
→ broker marks the current revision refresh_required
→ extension notifies the operator
→ operator clicks Refresh session
→ broker-owned incognito window opens
→ operator completes normal login
→ extension captures YouTube-scoped cookies
→ a fresh revision is published
→ broker-owned incognito window closes
→ old-revision failures no longer affect current health
```

Chromium shares one cookie store across incognito windows, so isolated recovery refuses to start while another incognito window is open.

Normal-browser recovery is also available. It uses the ordinary browser session and **never auto-closes the normal window**.

> Broker profiles are logical broker sessions. They are not extra Chromium cookie stores.

Media Cookie Broker does **not** automate Google login, CAPTCHA, 2FA, or upstream anti-bot controls.

---

## 📦 Two ways to integrate

### 🗃️ File-only

No application changes required.

`cookie-sync`:

- fetches standard Netscape cookie files;
- uses ETags;
- writes atomically with mode `0600`;
- preserves last-known-good state after bad responses;
- writes an optional `.meta.json` sidecar;
- tracks provider/profile/revision/timestamps/SHA-256;
- refuses to report health for a locally modified cookie file;
- restores broker-backed content when local files or sidecars diverge from trusted state.

### 🩺 Broker-aware

A wrapper or application can report what happened while using a specific revision:

```bash
cookie-sync report \
  --provider youtube \
  --profile default \
  --file /run/media-cookie-broker/youtube.txt \
  --kind authentication_required
```

Report kinds:

| Kind | Meaning |
| --- | --- |
| `ok` | this consumer successfully used the revision |
| `authentication_required` | human authentication is needed |
| `access_denied` | upstream denied access |
| `rate_limited` | upstream rate-limited the consumer |
| `unknown_failure` | other failure |

In this preview, one current-revision `authentication_required` report is enough to request human attention.

---

## 🧬 Providers + profiles

A session is:

```text
(provider, profile)
```

For example:

```text
youtube/default
youtube/music-bot
youtube/private-account
```

`default` keeps the one-account case simple.

| Provider | Recovery / capture | Status |
| --- | --- | --- |
| YouTube | isolated incognito by default; optional normal mode | ✅ primary workflow |
| TikTok | regular-browser capture | 🧪 experimental |
| Instagram | regular-browser capture | 🧪 experimental |
| X / Twitter | regular-browser capture | 🧪 experimental |

Provider policy lives in:

```text
internal/providers/
extension/providers/
```

There is no runtime plugin system in this preview.

---

## 🔐 Security

Media Cookie Broker handles **bearer credentials**.

The broker stores AES-256-GCM-encrypted snapshots, bounded revision history, users/grants, consumer activity, normalized health reports, and optional encrypted diagnostics. The master key stays outside SQLite.

Rules worth remembering:

- use HTTPS for every non-loopback broker connection;
- use separate publisher/reader credentials;
- grant only the exact provider/profile scopes required;
- protect extension local storage;
- protect the master key, SQLite volume, cookie files, sidecars, and password files;
- keep the broker behind appropriate network controls;
- never log cookie values, passwords, Authorization headers, master keys, or live session material.

Encryption at rest protects a stolen database **without** its master key. It does not save a host compromised together with the key.

Read **[SECURITY.md](SECURITY.md)** before deployment.

Security vulnerabilities should use GitHub's **private vulnerability reporting**, not public issues.

---

<details>
<summary><strong>🧾 Revision + health semantics</strong></summary>

Snapshots are revisioned per provider/profile.

Ordinary publication deduplicates identical canonical cookie material.

A successful explicit recovery is different:

```text
publication_reason = recovery
```

Recovery **always advances the revision**, even if the resulting cookie SHA is unchanged. That makes the authentication lifecycle explicit and naturally retires failures attached to the previous revision.

The latest report wins per consumer/provider/profile/revision.

An `ok` report clears only the same consumer's previous report for that revision. One consumer's success does not erase another consumer's failure.

Only reports from currently existing reader users with a current grant to that exact provider/profile contribute to active health. Revoking a grant retires that consumer's reports from active aggregation without deleting history.

Revision history is currently bounded to five snapshots per provider/profile.

</details>

<details>
<summary><strong>👥 User management</strong></summary>

Users live in SQLite and are managed locally with `brokerctl`.

Roles:

```text
publisher
reader
diagnostics_reader
```

Commands:

```bash
brokerctl user add
brokerctl user list
brokerctl user delete
brokerctl user passwd
brokerctl user grant
brokerctl user revoke
```

Named profile example:

```bash
docker compose run --rm --entrypoint brokerctl broker \
  user grant browser-publisher --provider youtube --profile music-bot

docker compose run --rm --entrypoint brokerctl broker \
  user grant downloader --provider youtube --profile music-bot
```

There is deliberately no HTTP admin API or web admin panel.

</details>

<details>
<summary><strong>🌐 API</strong></summary>

Default-profile shorthand:

```text
PUT  /v1/providers/{provider}/cookies
GET  /v1/providers/{provider}/cookies.txt
GET  /v1/providers/{provider}/status
POST /v1/providers/{provider}/reports
```

Named-profile routes insert:

```text
/profiles/{profile}
```

before the resource.

Publisher polling:

```text
GET /v1/status
```

Cookie responses include ETag, revision, capture/create timestamps, and provider/profile headers.

Profile-aware rollback:

```bash
docker compose exec broker \
  brokerctl rollback \
  --provider youtube \
  --profile default \
  --revision 1
```

</details>

<details>
<summary><strong>🩻 Diagnostics</strong></summary>

The extension keeps bounded local diagnostics across service-worker restarts.

Remote diagnostics:

- are off by default;
- require explicit enablement;
- are provider/profile scoped;
- are validated server-side;
- are encrypted by the broker.

Local history is richer. Generic control-plane/system events stay local-only.

</details>

<details>
<summary><strong>⚙️ More cookie-sync options</strong></summary>

Named targets:

```bash
COOKIE_SYNC_TARGETS='youtube/default=/run/cookies/default.txt,youtube/music-bot=/run/cookies/music.txt'
```

`COOKIE_SYNC_INTERVAL` defaults to `5m` and must be at least `10s`.

A combined jar can be built with:

```bash
COOKIE_SYNC_COMBINED=/absolute/path/cookies.txt
```

Per-profile metadata sidecars can be disabled with:

```bash
COOKIE_SYNC_METADATA=false
```

Combined jars intentionally have no single-revision sidecar.

</details>

---

## 🧪 Development

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

---

## 🚧 Preview boundaries

This is intentionally **not** an everything-platform.

Not in scope right now:

- web admin panel;
- OAuth;
- automatic login;
- CAPTCHA / 2FA automation;
- HA / replication;
- Firefox packaging;
- runtime plugin loading;
- generic non-cookie browser state;
- quorum-based refresh policy.

Profile support exists, but polished profile creation/deletion/account-label UX is still deferred.

More detail:

- [Known limitations](docs/KNOWN_LIMITATIONS.md)
- [Provider notes](docs/PROVIDERS.md)
- [Contributing](CONTRIBUTING.md)

---

## 📜 License

MIT — see [LICENSE](LICENSE).

