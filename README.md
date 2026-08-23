<div align="center">

# 🍪 Media Cookie Broker

### Your server shouldn't need a browser just because its cookies expired.

**Human-in-the-loop browser authentication maintenance for unattended media software.**

[![Release](https://img.shields.io/github/v/release/segfault-stack/media-cookie-broker?include_prereleases&sort=semver&label=release)](https://github.com/segfault-stack/media-cookie-broker/releases)
[![License](https://img.shields.io/github/license/segfault-stack/media-cookie-broker)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Chromium MV3](https://img.shields.io/badge/Chromium-MV3-4285F4?logo=googlechrome&logoColor=white)

[Quick start](#-try-it-in-5-minutes) ·
[How it works](#-what-happens-when-auth-dies) ·
[Integration](#-keep-using-cookiestxt) ·
[Security](#-security) ·
[Limitations](#-public-preview)

</div>

---

Your downloader, bot, NAS, or media service can run for weeks with a normal Netscape `cookies.txt`.

Then a provider wants a **real browser** again — login, CAPTCHA, 2FA, account confirmation, or another interactive step.

Media Cookie Broker handles that moment:

> **consumer reports auth failure → Chrome notifies you → you click Refresh session → log in normally → fresh cookie revision goes back to the server**

No permanent browser on the VPS.  
No VNC session beside every downloader.  
No manual cookie export + SCP ritual.

> **Existing software does not need to understand Media Cookie Broker. If it reads `cookies.txt`, it can keep reading `cookies.txt`.**

> [!WARNING]
> **Public preview.** Browser cookies are bearer credentials. Use HTTPS for non-loopback deployments and read [SECURITY.md](SECURITY.md) before putting a broker on a real network.

---

## 🚀 Try it in 5 minutes

### 1. Run setup

```bash
git clone https://github.com/segfault-stack/media-cookie-broker.git
cd media-cookie-broker
./mcb setup
```

`mcb` does the boring part for you:

```text
✓ Docker + Compose
✓ broker master key
✓ publisher + reader
✓ broker image
✓ startup + health check
✓ reader password file
✓ exact extension path
```

At the end it prints the **one-time publisher password** and the exact Chromium extension path.

Running setup again is safe: it preserves the key, database, users, credentials, and snapshots.

### 2. Load the extension

1. Open `chrome://extensions`
2. Enable **Developer mode**
3. **Load unpacked** → choose the path printed by `./mcb setup`
4. Open **Details** → enable **Allow in incognito**
5. Open **Settings and guide**
6. Enter:
   - broker: `http://127.0.0.1:8787`
   - username: `browser-publisher`
   - publisher password printed during setup

### 3. Refresh your first session

In the extension popup:

> **YouTube / default → Refresh session**

Complete the normal Google / YouTube browser flow.

When the popup says:

```text
Healthy · revision 1
```

the browser ↔ broker recovery loop is working.

**That's the first success.** Everything after this is consumer integration.

---

### Prefer a user-local installer?

Inspect first:

```bash
curl -fsSLO \
  https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/main/install.sh

less install.sh
bash install.sh
```

It installs under:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/media-cookie-broker
```

No `sudo`, no systemd service, no shell startup-file edits, no browser-policy hacks.

<details>
<summary><strong>I know what <code>curl | bash</code> means</strong></summary>

```bash
curl -fsSL \
  https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/main/install.sh \
  | bash
```

</details>

---

## 🔔 What happens when auth dies?

```text
remote consumer
      │
      │ authentication_required
      ▼
┌─────────────────────┐
│ Media Cookie Broker │
│ revision 17         │
│ refresh required    │
└──────────┬──────────┘
           │ extension polls
           ▼
┌─────────────────────┐
│ your Chromium       │
│ 🔔 session needs you│
└──────────┬──────────┘
           │ Refresh session
           ▼
┌─────────────────────┐
│ real browser login  │
│ CAPTCHA / 2FA / etc │
└──────────┬──────────┘
           │ fresh scoped cookies
           ▼
┌─────────────────────┐
│ broker revision 18  │
│ encrypted + healthy │
└──────────┬──────────┘
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

The browser handles the rare step that actually needs a human.

The broker handles everything around it: **state, revisions, health, ACLs, distribution, and coordination**.

---

## ✨ Why use it?

| Without a broker | With Media Cookie Broker |
| --- | --- |
| Keep a browser/VNC session on every server | Use the browser you already use |
| Manually export + copy cookies | Publish and sync revisions |
| Notice auth failure after jobs break | Consumer can report `authentication_required` |
| Replace cookie files in place | Atomic `cookies.txt` updates |
| Guess which cookie dump is current | Revision + SHA metadata |
| Build custom integration glue | Keep using Netscape `cookies.txt` |
| Auto-trigger risky login flows | Human explicitly clicks **Refresh session** |

The consumer boundary stays intentionally boring:

```text
broker → cookie-sync → cookies.txt → whatever already reads cookies.txt
```

---

## 📄 Keep using `cookies.txt`

The first-party `cookie-sync` turns broker revisions into a standard Netscape cookie jar.

For the host-side helper you need **Go 1.24+**:

```bash
go build -o bin/cookie-sync ./cmd/cookie-sync
mkdir -p /tmp/media-cookies

BROKER_URL=http://127.0.0.1:8787 \
BROKER_USERNAME=downloader \
BROKER_PASSWORD_FILE=secrets/reader-password \
COOKIE_SYNC_TARGETS='youtube/default=/tmp/media-cookies/youtube.txt' \
./bin/cookie-sync
```

You get:

```text
/tmp/media-cookies/youtube.txt
/tmp/media-cookies/youtube.txt.meta.json
```

Point compatible software at `youtube.txt`.

`cookie-sync` gives you:

- ETag-based sync;
- atomic mode-`0600` writes;
- last-known-good behavior;
- provider/profile/revision metadata;
- SHA-256 integrity checks;
- fail-closed health reporting;
- recovery from locally modified cookie/sidecar files.

> Don't want `cookie-sync`? An authorized consumer can use the broker's HTTP/Netscape response directly.

---

## 🩺 Optional: let consumers report auth health

A broker-aware wrapper can report what happened while using a specific local revision:

```bash
cookie-sync report \
  --provider youtube \
  --profile default \
  --file /run/media-cookie-broker/youtube.txt \
  --kind authentication_required
```

| Kind | Meaning |
| --- | --- |
| `ok` | the revision worked |
| `authentication_required` | human refresh needed |
| `access_denied` | upstream denied access |
| `rate_limited` | upstream rate-limited the consumer |
| `unknown_failure` | other failure |

The helper verifies the local file against its sidecar before reporting.

In this preview, one valid current-revision `authentication_required` report is enough to trigger human attention.

---

## 🕹️ Browser = human control plane

The Manifest V3 extension:

- polls broker health every few minutes;
- shows provider/profile state;
- distinguishes **broker unreachable** from **provider auth broken**;
- sends system notifications on meaningful transitions;
- opens the status/action UI when you click a notification;
- starts login **only after you click `Refresh session`**;
- keeps bounded redacted local diagnostics;
- optionally uploads scoped encrypted diagnostics.

It never starts an interactive login just because a background process complained.

That human boundary is intentional.

---

## 🥷 YouTube recovery is isolated by default

YouTube is the primary preview workflow.

```text
refresh required
→ explicit human click
→ fresh broker-owned incognito window
→ normal Google / YouTube login
→ capture YouTube-scoped cookies
→ publish fresh revision
→ close only the broker-owned window
```

Chromium shares one cookie store across all incognito windows, so isolated recovery refuses to start while another incognito window is open.

Normal-browser recovery is also available. In normal mode:

- your ordinary browser session is used;
- the normal window is never auto-closed;
- changing accounts can affect everyday browser state.

> Broker profiles are logical broker sessions, not extra Chromium cookie stores.

Media Cookie Broker does **not** automate Google login, CAPTCHA, 2FA, or upstream anti-bot controls.

---

## 🧬 Providers + profiles

A session is:

```text
(provider, profile)
```

Examples:

```text
youtube/default
youtube/music-bot
youtube/private-account
```

| Provider | Recovery / capture | Status |
| --- | --- | --- |
| YouTube | isolated incognito by default; optional normal mode | ✅ **primary** |
| TikTok | regular-browser capture | 🧪 experimental |
| Instagram | regular-browser capture | 🧪 experimental |
| X / Twitter | regular-browser capture | 🧪 experimental |

Provider policy is source-defined in `internal/providers/` and `extension/providers/`.

No runtime plugin system yet.

---

## 🔐 Security

Media Cookie Broker handles **bearer credentials**. Treat them like passwords.

The broker encrypts cookie snapshots with **AES-256-GCM**. The master key stays outside SQLite.

Keep these rules:

- HTTPS for every non-loopback broker connection;
- separate publisher and reader credentials;
- exact provider/profile grants;
- protect extension local storage;
- protect the master key, SQLite volume, cookie files, sidecars, and password files;
- never log cookie values, passwords, Authorization headers, master keys, or live session material.

Encryption at rest helps if someone steals the database **without** the key. It does not save a host compromised together with its key.

Read **[SECURITY.md](SECURITY.md)** for the full threat model.

Security bugs → GitHub **private vulnerability reporting**, not public issues.

---

## 🛠️ Operator cheat sheet

```bash
./mcb setup
./mcb up
./mcb down
./mcb status
./mcb logs
./mcb logs -f
./mcb doctor
./mcb extension-path
```

If something feels wrong:

```bash
./mcb doctor
```

Normal `down` preserves persistent data.

---

<details>
<summary><strong>🧾 Revision + health semantics</strong></summary>

Snapshots are revisioned per provider/profile.

Ordinary publication deduplicates identical canonical cookie material.

A successful explicit recovery always advances the revision, even when canonical cookie bytes are unchanged. Recovery is a lifecycle event: failures attached to the old revision naturally stop affecting current health.

An `ok` report supersedes only the same consumer's previous report for that revision. One consumer's success does not erase another consumer's failure.

Only reports from currently existing readers with an active grant to the exact provider/profile contribute to active health.

Revision history is currently bounded to five snapshots per provider/profile.

</details>

<details>
<summary><strong>👥 Users + ACLs</strong></summary>

Users live in SQLite and are managed locally with `brokerctl`.

Roles:

```text
publisher
reader
diagnostics_reader
```

Useful commands:

```bash
brokerctl user add
brokerctl user list
brokerctl user delete
brokerctl user passwd
brokerctl user grant
brokerctl user revoke
```

There is deliberately no HTTP admin API or web admin panel in this preview.

</details>

<details>
<summary><strong>🌐 HTTP API</strong></summary>

Default-profile shorthand:

```text
PUT  /v1/providers/{provider}/cookies
GET  /v1/providers/{provider}/cookies.txt
GET  /v1/providers/{provider}/status
POST /v1/providers/{provider}/reports
```

Named-profile routes insert `/profiles/{profile}` before the resource.

Publisher polling:

```text
GET /v1/status
```

Profile-aware rollback:

```bash
docker compose exec broker \
  brokerctl rollback \
  --provider youtube \
  --profile default \
  --revision 1
```

</details>

---

## 🧪 Development

```bash
bash -n mcb install.sh scripts/bootstrap-compose.sh tests/*.sh
tests/mcb-test.sh
tests/install-test.sh

gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...

npm --prefix extension test
node --check extension/service-worker.js

docker compose config
docker build -t media-cookie-broker:preview .
COOKIE_BROKER_TEST_IMAGE=media-cookie-broker:preview tests/container-smoke.sh
```

---

## 🚧 Public preview

The useful core works, but this is still early software.

Not in scope right now:

- web admin panel;
- OAuth;
- automatic login / CAPTCHA / 2FA;
- HA / replication;
- Firefox packaging;
- runtime plugins;
- generic non-cookie browser state;
- quorum-based refresh policy.

More detail:

- [Known limitations](docs/KNOWN_LIMITATIONS.md)
- [Provider notes](docs/PROVIDERS.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

---

<div align="center">

### 🍪 Browser auth when a human is needed. Plain `cookies.txt` everywhere else.

MIT — see [LICENSE](LICENSE).

</div>

