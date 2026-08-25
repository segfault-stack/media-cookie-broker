# Media Cookie Broker

Keep `yt-dlp` and other unattended software authenticated with browser-refreshed `cookies.txt` files—without running a browser on the server.

[![Release](https://img.shields.io/github/v/release/segfault-stack/media-cookie-broker?include_prereleases&sort=semver&label=release)](https://github.com/segfault-stack/media-cookie-broker/releases)
[![CI](https://github.com/segfault-stack/media-cookie-broker/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/segfault-stack/media-cookie-broker/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/segfault-stack/media-cookie-broker)](LICENSE)

> [!WARNING]
> **Public preview.** Media Cookie Broker handles browser cookies, which are bearer credentials. It has not been audited as production account infrastructure. Keep the broker loopback-only for the quick start, use HTTPS for every non-loopback connection, and read [Security](#security) before deploying it.

Media Cookie Broker was created for a recurring `yt-dlp` problem: the downloader runs on a server, VPS, or NAS, but the authenticated browser session—and the person who can complete login, CAPTCHA, or 2FA—lives on another computer.

The project is not an `yt-dlp` plugin. Its broader contract is an ordinary Netscape `cookies.txt` file for existing consumers, plus an HTTP API for software that wants to participate in authentication-health reporting. YouTube is the primary preview workflow; other providers are experimental.

[Quick start](#quick-start) · [Compatibility](#compatibility) · [Recovery flow](#when-authentication-expires) · [Security](#security) · [Limitations](#limitations) · [Documentation](#documentation)

## How it fits

```text
SERVER / VPS / NAS                         YOUR COMPUTER

┌──────────────────────┐                  ┌──────────────────────┐
│ yt-dlp or other app  │                  │ Chromium             │
│        ▲             │                  │ MCB extension        │
│   cookies.txt        │                  │ human login          │
│        ▲             │                  └──────────┬───────────┘
│   cookie-sync        │                             │
│        ▲             │                    SSH tunnel or HTTPS
│ Media Cookie Broker  │◄────────────────────────────┘
└──────────────────────┘
```

The browser is intentionally not on the server. The broker stores scoped, revisioned cookie snapshots; `cookie-sync` writes them atomically as mode-`0600` Netscape cookie files; existing software keeps reading those files normally.

Interactive authentication remains human-controlled. The extension never starts a login merely because a background process reported a failure.

## Quick start

This route assumes:

- an unprivileged Linux account on the server with `curl`, `tar`, Docker, and Docker Compose v2;
- a desktop with SSH, `curl`, `tar`, and a Chromium-based browser that can load an unpacked extension;
- network access to GitHub and container registries while the installers download and build the immutable `v0.3.0-preview` source;
- `yt-dlp`, or another Netscape-cookie consumer, installed separately.

The example runs the broker and `cookie-sync` on the same server. Keep the two terminal contexts distinct.

### 1. Start the broker on the server

Download and inspect the pinned installer before running it:

```bash
curl -fsSLo mcb-install.sh \
  https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install.sh
less mcb-install.sh
bash mcb-install.sh
```

The installer builds the broker image, creates a loopback-only service, and prints:

- the exact `mcb status`, `mcb logs`, and `mcb doctor` command paths;
- a one-time publisher password for the browser extension;
- the location of the reader password used by `cookie-sync`.

Run the printed `mcb status` command. The broker should be running and healthy. Preserve the generated credentials; the publisher password is not recoverable later without resetting it.

### 2. Connect Chromium from your computer

Open a tunnel in a desktop terminal and leave it running:

```bash
ssh -N -L 8787:127.0.0.1:8787 user@your-server
```

Here, `127.0.0.1:8787` on the desktop is one end of the SSH tunnel. The broker remains bound to server loopback.

In another desktop terminal, download, inspect, and run the extension installer:

```bash
curl -fsSLo mcb-extension-install.sh \
  https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install-extension.sh
less mcb-extension-install.sh
bash mcb-extension-install.sh
```

Open `chrome://extensions`, enable **Developer mode**, choose **Load unpacked**, select the directory printed by the installer, and enable **Allow in incognito**.

In the extension, set:

- broker URL: `http://127.0.0.1:8787`;
- username: `browser-publisher`;
- password: the publisher password printed on the server.

Choose **YouTube / default → Refresh session** and complete the provider login yourself. Success is visible as `Healthy · revision 1` in the extension popup.

### 3. Create `cookies.txt` on the server

Download and inspect the pinned consumer installer:

```bash
curl -fsSLo mcb-cookie-sync-install.sh \
  https://raw.githubusercontent.com/segfault-stack/media-cookie-broker/v0.3.0-preview/install-cookie-sync.sh
less mcb-cookie-sync-install.sh
bash mcb-cookie-sync-install.sh
```

Accept the loopback defaults when the sidecar runs beside the broker, enter the reader password printed during server setup, and choose the output directory. The installer prints the exact status/log commands and resulting file path, normally:

```text
~/media-cookies/youtube.txt
```

Run the printed `cookie-sync status` command and confirm that the cookie file exists after the first successful sync.

### 4. Use it with yt-dlp

Point `yt-dlp` at the synchronized file:

```bash
yt-dlp --cookies ~/media-cookies/youtube.txt \
  'https://www.youtube.com/watch?v=VIDEO_ID'
```

Replace `VIDEO_ID` with a real target that requires the authenticated account. A successful download of that target demonstrates the primary consumer path. Software that accepts a Netscape cookie file can use the same output path without knowing about Media Cookie Broker.

For persistent remote access, updates, rollback, filesystem layout, and troubleshooting, continue with [installation and operations](docs/INSTALLATION.md). For multiple cookie targets or a consumer on another machine, read [cookie-sync integration](docs/COOKIE_SYNC.md).

---

## When authentication expires

The complete feedback loop has an explicit human step:

```text
broker-aware consumer reports authentication_required
        ↓
extension observes the broker state and notifies the user
        ↓
user clicks Refresh session and completes browser authentication
        ↓
extension publishes a new scoped cookie revision
        ↓
cookie-sync atomically updates cookies.txt
        ↓
consumer can retry with the new revision
```

A plain `yt-dlp --cookies ...` process reads the synchronized file but does not report authentication failure to the broker. You can refresh manually at any time. Automatic failure notifications require a wrapper or consumer that calls the report API.

For YouTube, isolated recovery opens a broker-owned incognito window by default and closes only that window after capture. Chromium shares one cookie store across all incognito windows, so recovery refuses to start while another incognito window is open. Optional normal-window recovery uses the current ordinary browser session and never closes that window automatically.

Media Cookie Broker does not automate provider login, CAPTCHA, 2FA, account confirmation, or anti-bot controls.

## Compatibility

These levels describe different kinds of evidence; they are not interchangeable:

| Path | Level |
| --- | --- |
| YouTube browser capture → broker → `cookies.txt` | Primary preview workflow |
| `cookies.txt` → `yt-dlp` | Primary consumer use case |
| Other Netscape `cookies.txt` consumers | Compatible by contract |
| Consumer or wrapper using the report API | Supported interface |
| TikTok, Instagram, and X / Twitter capture | Experimental |
| Firefox extension, runtime providers, non-cookie browser state | Out of scope today |

The maintained first-party YouTube path has automated coverage for broker, extension-policy, and synchronization behavior; real provider login remains an external interactive step. The `yt-dlp` integration uses its existing Netscape-cookie interface rather than an MCB-specific plugin. Contract-compatible applications may work without being tested by this project, while the report API adds the full failure-feedback loop for an exact provider, profile, and revision.

Experimental providers are implemented but are not validated across every downloader, account state, or upstream change.

Provider and consumer compatibility are separate. A consumer that reads Netscape cookies does not make every provider capture workflow supported.

## What the broker and sync sidecar do

- store encrypted cookie snapshots by provider and logical profile;
- enforce separate publisher, reader, and diagnostics-reader roles with exact grants;
- track revisions, normalized consumer health reports, and bounded redacted diagnostics;
- expose ETag-aware Netscape-cookie responses;
- write cookie files and metadata atomically with mode `0600`;
- preserve last-known-good files during invalid responses or temporary broker failures;
- detect local file tampering and require a broker-backed refetch;
- keep browser authentication behind an explicit user action.

Broker profiles are logical sessions, not independent Chromium cookie stores. Provider policy is compiled into the broker and extension; there is no runtime plugin system in this preview.

## Security

Browser cookies are bearer credentials. Anyone who obtains a valid cookie may be able to act as the corresponding account without knowing its password.

The broker encrypts cookie snapshots with AES-256-GCM, with the master key stored outside SQLite. This protects a copied database only when the attacker does not also obtain the key; it does not protect a fully compromised browser, broker host, or authorized consumer.

Operators must:

- use HTTPS for every non-loopback broker connection;
- keep separate publisher and reader credentials with exact provider/profile grants;
- restrict access to extension storage, the master key, database, password files, cookie files, and metadata sidecars;
- avoid publishing cookies, passwords, authorization headers, account identifiers, private endpoints, or unredacted logs;
- back up the master key and broker data together and treat removal of either as potentially destructive.

Read [SECURITY.md](SECURITY.md) for the threat model and compromise response. Report vulnerabilities through GitHub private vulnerability reporting, never through a public issue containing sensitive evidence.

## Limitations

This is an early public preview with a best-effort support boundary. Only the latest preview release is expected to receive security fixes.

Important limitations:

- only unpacked Chromium-extension installation is currently supported;
- a human must complete interactive authentication and explicitly start recovery;
- plain file consumers cannot report authentication failure without a wrapper;
- isolated YouTube recovery requires other incognito windows to be closed;
- normal-browser profiles do not isolate accounts from one another;
- the extension stores its dedicated publisher credential in `chrome.storage.local`, not an operating-system keychain;
- one current-revision authentication failure immediately requests refresh; quorum and post-refresh verification are not implemented;
- there is no web administration, OAuth, high availability, replication, Firefox package, or runtime provider loading.

See [known limitations](docs/KNOWN_LIMITATIONS.md) for the complete current list.

## Operations and recovery

The installers print exact command paths for status and logs. Start troubleshooting with those commands:

- broker: `status`, `logs`, and `doctor`;
- cookie-sync: `status` and `logs`;
- extension: popup state and bounded redacted local diagnostics.

Stopping containers does not remove the broker volume or synchronized cookie files. Removing the master key, broker volume, credentials, or cookie files is a separate destructive action; back up required state first. Detailed update, rollback, and recovery behavior is documented in [installation and operations](docs/INSTALLATION.md).

## Documentation

- [Installation and server operations](docs/INSTALLATION.md)
- [cookie-sync consumer integration](docs/COOKIE_SYNC.md)
- [Provider architecture and policy](docs/PROVIDERS.md)
- [Known limitations](docs/KNOWN_LIMITATIONS.md)
- [Security policy and threat model](SECURITY.md)
- [Support boundary](SUPPORT.md)
- [Contributing](CONTRIBUTING.md)
- [Project polish roadmap](docs/PROJECT_POLISH_ROADMAP.md)

## Development

Clone with the pinned agent playbook and run the documented verification suite:

```bash
git clone --recurse-submodules \
  https://github.com/segfault-stack/media-cookie-broker.git
cd media-cookie-broker
go test ./...
npm --prefix extension test
tests/onboarding-docs-test.sh
```

The complete local and container checks are in [CONTRIBUTING.md](CONTRIBUTING.md). Contributions are welcome within the project's preview scope; never use real cookies or credentials in tests, issues, or pull requests.

## License

[MIT](LICENSE)
