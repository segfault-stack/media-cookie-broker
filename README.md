<div align="center">

# 🍪 Media Cookie Broker

### Browser auth for unattended software — without running the browser on the server.

**Human-in-the-loop browser authentication maintenance for unattended software.**

[![Release](https://img.shields.io/github/v/release/segfault-stack/media-cookie-broker?include_prereleases&sort=semver&label=release)](https://github.com/segfault-stack/media-cookie-broker/releases)
[![CI](https://github.com/segfault-stack/media-cookie-broker/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/segfault-stack/media-cookie-broker/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/segfault-stack/media-cookie-broker)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Chromium MV3](https://img.shields.io/badge/Chromium-MV3-4285F4?logo=googlechrome&logoColor=white)

[Quick start](#-quick-start) ·
[yt-dlp](#-use-it-with-yt-dlp) ·
[How it works](#-what-happens-when-auth-dies) ·
[Integration](#-keep-using-cookiestxt) ·
[Security](#-security) ·
[Limitations](#-public-preview)

</div>

---

Media Cookie Broker was built for a recurring `yt-dlp` problem: the downloader runs unattended on a server, VPS, or NAS, while the authenticated browser session belongs on your own computer.

It is not coupled to `yt-dlp`. Any downloader, bot, media service, or other consumer that reads a Netscape `cookies.txt` file—or integrates with the broker API—can use the same browser-backed session flow.

Then a provider wants a **real browser** again — login, CAPTCHA, 2FA, account confirmation, or another interactive step.

Media Cookie Broker handles that moment across two machines:

> **consumer reports auth failure → Chrome notifies you → you click Refresh session → log in normally → fresh cookie revision goes back to the server**

No permanent browser on the server.
No VNC session beside every downloader.
No manual cookie export + SCP ritual.

> **Existing software does not need to understand Media Cookie Broker. If it reads `cookies.txt`, it can keep reading `cookies.txt`.**

> [!WARNING]
> **Public preview.** Browser cookies are bearer credentials. Use HTTPS for non-loopback deployments and read [SECURITY.md](SECURITY.md) before putting a broker on a real network.

## Two machines by design

```text
YOUR SERVER / VPS / NAS                    YOUR COMPUTER

┌──────────────────────┐                  ┌──────────────────────┐
│ media app            │                  │ Chromium             │
│ cookie-sync          │                  │ MCB extension        │
│                      │                  │                      │
│ Media Cookie Broker  │◄── SSH tunnel ──►│ human                │
└──────────────────────┘    or HTTPS      └──────────────────────┘
          │
          ▼
      cookies.txt
```

> **The browser is intentionally not on the server.** The broker stays loopback-bound by default; the desktop reaches it through an SSH tunnel for the quick start, or through protected HTTPS/private networking for a persistent deployment.

---

## 🚀 Quick start

### 🖥️ 1. On the server

```bash
curl -fsSL https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install.sh | bash
```

It starts the loopback-only broker and prints the publisher credential needed by your browser.

### 💻 2. On your computer

Open an SSH tunnel to the server and leave it running:

```bash
ssh -N -L 8787:127.0.0.1:8787 user@your-server
```

`127.0.0.1` is the desktop end of the tunnel here; the broker itself remains loopback-only on the server.

Install the desktop extension files:

```bash
curl -fsSL https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install-extension.sh | bash
```

Then open `chrome://extensions` → enable **Developer mode** → **Load unpacked** → select the printed path → **Allow in incognito**. Set the broker URL to `http://127.0.0.1:8787`, enter `browser-publisher` and the publisher password, then choose **YouTube / default → Refresh session**.

When the popup says `Healthy · revision 1`, the browser ↔ broker recovery loop works.

For an always-on deployment, expose the broker only through protected HTTPS or an appropriate private network path. Never expose it as public plain HTTP.

### 🍪 3. Connect a consumer

```bash
curl -fsSL https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install-cookie-sync.sh | bash
```

It configures the Dockerized sidecar and prints the resulting Netscape cookie file, for example:

```text
~/media-cookies/youtube.txt
```

Point your existing software at that file. The consumer can run on the broker host or any authorized host that can reach the broker through loopback, protected HTTPS, or appropriate private networking.

### Use it with yt-dlp

Point `yt-dlp` at the synchronized cookie jar:

```bash
yt-dlp --cookies ~/media-cookies/youtube.txt 'https://www.youtube.com/watch?v=...'
```

The file is an ordinary Netscape cookie jar. Replace `yt-dlp` with any software that supports the same format, or use the broker API for a tighter integration.

| Integration | Status |
| --- | --- |
| YouTube → `cookies.txt` → `yt-dlp` | Primary use case |
| Any Netscape `cookies.txt` consumer | Compatible file contract |
| Consumer or wrapper using the report API | Full authentication-failure feedback loop |
| TikTok, Instagram, and X / Twitter capture | Experimental provider support |

> [!NOTE]
> A plain `yt-dlp --cookies ...` process consumes the synchronized file but does not report authentication failures back to the broker. Automatic `authentication_required` notifications require a broker-aware wrapper or another consumer that calls the report API. You can still refresh the browser session manually at any time.

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

The Dockerized `cookie-sync` sidecar turns broker revisions into an ordinary Netscape cookie jar in a host directory shared with your downloader, bot, NAS, or media service.

`cookie-sync` gives you:

- ETag-based sync;
- atomic mode-`0600` writes;
- last-known-good behavior;
- provider/profile/revision metadata;
- SHA-256 integrity checks;
- fail-closed health reporting;
- recovery from locally modified cookie/sidecar files.

> Don't want `cookie-sync`? An authorized consumer can use the broker's HTTP/Netscape response directly.

See [cookie-sync advanced setup](docs/COOKIE_SYNC.md) for multiple targets, named profiles, metadata, health reports, direct binary use, and manual Compose configuration.

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

The installers print the exact status and log commands for each installed instance. See [installation details](docs/INSTALLATION.md) for paths, updates, source setup, and troubleshooting.

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
bash -n mcb install*.sh scripts/*.sh tests/*.sh
tests/mcb-test.sh
tests/install-test.sh
tests/install-extension-test.sh
tests/install-cookie-sync-test.sh
tests/package-extension-test.sh
tests/onboarding-docs-test.sh

gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...

npm --prefix extension test
node --check extension/service-worker.js

docker compose config
docker build --target broker -t media-cookie-broker:preview .
docker build --target cookie-sync -t media-cookie-broker-cookie-sync:preview .
COOKIE_BROKER_TEST_IMAGE=media-cookie-broker:preview \
COOKIE_SYNC_TEST_IMAGE=media-cookie-broker-cookie-sync:preview \
tests/container-smoke.sh
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

- [Installation details](docs/INSTALLATION.md)
- [cookie-sync advanced setup](docs/COOKIE_SYNC.md)
- [Known limitations](docs/KNOWN_LIMITATIONS.md)
- [Provider notes](docs/PROVIDERS.md)
- [Project polish roadmap](docs/PROJECT_POLISH_ROADMAP.md)
- [Support](SUPPORT.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

---

<div align="center">

### 🍪 Browser auth when a human is needed. Plain `cookies.txt` everywhere else.

MIT — see [LICENSE](LICENSE).

</div>
