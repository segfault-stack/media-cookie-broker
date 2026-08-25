# Project Agent Context

## Required context

- Purpose and audience: keep unattended software authenticated with browser-refreshed cookies without running a browser on the server; see `README.md`.
- Wedge: recurring `yt-dlp` authentication expiry on a server, VPS, or NAS while the interactive browser session lives on the user's computer.
- General contract: publish scoped, revisioned browser cookies and synchronize them to an ordinary Netscape `cookies.txt`, or integrate through the broker API.
- Primary supported behavior: YouTube to `cookies.txt` to `yt-dlp`; the broker-aware report API provides the full authentication-failure feedback loop.
- Important non-goals and limitations: see `docs/KNOWN_LIMITATIONS.md`; plain `yt-dlp` does not report authentication failure, provider behavior can change, and the project does not automate login, CAPTCHA, or 2FA.
- Sensitive data and boundaries: browser cookies, broker credentials, the master key, synchronized cookie files, metadata sidecars, and private endpoints are sensitive. Do not expose the broker over public plain HTTP or publish real credentials. Destructive removal of volumes, keys, snapshots, or consumer files requires explicit authorization.

## Local verification

- Bootstrap: Go 1.25+, Node.js 24 for parity with CI, Bash, Docker with Compose v2, and `zip`/`unzip` for packaging. No production credentials are required.
- Fast checks: `go test ./...`, `npm --prefix extension test`, and `bash -n mcb install*.sh scripts/*.sh tests/*.sh`.
- Full checks: follow the jobs in `.github/workflows/ci.yml` and the commands in `CONTRIBUTING.md`.
- Build/package: build both Dockerfile targets; package the extension with `scripts/package-extension.sh [version]`.
- Integration or smoke tests: `tests/container-smoke.sh` and `tests/install-cookie-sync-integration.sh` require Docker and locally built test images.
- Documentation checks: `tests/onboarding-docs-test.sh`; verify README links and commands when documentation changes.
- Security checks: `go vet ./...`, `go test -race ./...`, GitHub CodeQL, dependency review, secret scanning, and manual inspection of archives and build contexts. Network access is required to query external vulnerability databases.

## Publication and support

- Publication boundary: source, documentation, container build definitions, installers, and versioned extension ZIP/checksums are public. Operator credentials, cookies, databases, keys, diagnostics, and private deployment details are not.
- Supported versions: while pre-1.0, only the latest public-preview release is expected to receive security fixes; see `SECURITY.md`.
- Contribution and support policy: `CONTRIBUTING.md` and `SUPPORT.md`.
- Contribution intake and maintainer-review policy: focused public issues and pull requests are accepted within the boundaries in `CONTRIBUTING.md`; changes merge only after the repository's required checks and maintainer review.
- Agent-assisted public contribution policy: no additional project-specific disclosure rule is documented; agents must not claim human review and may publish or merge only when the maintainer explicitly authorizes that external action.
- Private security reporting route: GitHub private vulnerability reporting, as documented in `SECURITY.md`.
- External actions requiring maintainer authorization: pushes, repository setting changes, releases, package publication, deployments, and messages or submissions outside the repository.

## Repository and release process

- Default branch and merge policy: `main`, squash-only pull requests with required CI and conversation resolution; force-push and deletion are blocked.
- Generated files and sensitive or persistent paths: `dist/` contains release artifacts; runtime secrets live under `secrets/`; broker state lives in the `broker-data` Docker volume; synchronized cookie files are private persistent data.
- Version source: immutable Git tags and GitHub releases; the current documented preview is `v0.3.0-preview`.
- Release workflow: follow `docs/PROJECT_POLISH_ROADMAP.md`, verify the exact tree, package the extension, publish SHA-256 checksums, and keep installer refs immutable.
- Artifact integrity policy: release assets include `SHA256SUMS`; inspect packaged contents independently and ensure assets trace to the tagged source revision.
- Deployment boundary and rollback: operators own TLS, DNS, firewalls, private networking, backups, and restore. Review release notes before manual updates; preserve the master key and SQLite volume, and retain the extension installer's previous directory for rollback.

## Enabled playbook profiles

- Enabled profiles: `none` (release `v0.4.1` publishes no technology profiles).

## Project-specific decisions

- The repository leads with `yt-dlp` as the concrete discovery path while preserving the consumer-neutral Netscape cookie/API contract.
- Image, logo, SVG, screenshot, demo, and social-preview creation is not part of routine agent maintenance unless the maintainer explicitly requests it.

## Pinned playbook

- Upstream: `https://github.com/segfault-stack/oss-agent-playbook`
- Immutable ref: `v0.4.1`
- Resolved commit: `6ac968f8251b15ee78454fdd1d4358a23d2f71da`
- Import method: `git submodule`
- Imported or last reviewed: `2026-08-25`
