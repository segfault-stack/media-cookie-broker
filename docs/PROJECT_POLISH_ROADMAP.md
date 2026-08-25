# Project polish roadmap

This is the working path for presenting, hardening, releasing, and introducing Media Cookie Broker without narrowing it to one consumer.

## Product position

`yt-dlp` is the original problem and the clearest discovery path, not the product boundary.

The short version:

> Keep `yt-dlp` and other unattended software authenticated with browser-refreshed `cookies.txt`—without running a browser on the server.

The broader contract:

- a human maintains authentication in their normal desktop browser;
- the broker stores and distributes scoped, revisioned browser cookies;
- `cookie-sync` produces an ordinary Netscape cookie jar;
- any compatible file consumer can use that jar;
- API-aware consumers can also report failures and participate in the full recovery loop.

Do not present the project as an `yt-dlp` plugin, a Chromium extension, or a generic login-automation tool. Lead with the concrete `yt-dlp` pain, then explain the consumer-neutral contract.

## Phase 1: clear discovery path

- [x] Give the GitHub repository a concrete English description.
- [x] Add search topics centered on `yt-dlp`, `cookies.txt`, self-hosting, and compatible consumers.
- [x] Name `yt-dlp` on the first screen of the README.
- [x] Add a minimal `yt-dlp --cookies ...` example.
- [x] Explain the difference between file consumption and the full report-driven recovery loop.
- [x] Keep the README explicit that any Netscape-cookie or API consumer can integrate.
- [ ] Upload a user-supplied or designer-provided 1280×640 social preview showing desktop browser → broker → `cookies.txt` → remote consumer, with `yt-dlp` as the concrete example. Do not generate or draw this asset as part of repository maintenance.
- [ ] Add one lightweight visual proof: two screenshots or a short optimized demo showing `authentication required` → human refresh → healthy revision.
- [ ] Review repository topics after real traffic arrives; do not add implementation-detail topics merely to fill the limit.

## Phase 2: contributor and support hygiene

- [x] Add issue forms for bugs, configuration help, and provider/integration requests.
- [x] Put a prominent warning in issue forms: never attach real cookies, passwords, keys, Authorization headers, or private endpoints.
- [x] Add a pull request template with tests, security-boundary, and documentation checkboxes.
- [x] Add a support policy that separates public usage questions from private vulnerability reports.
- [ ] Add a concise Code of Conduct if outside contributions begin.
- [ ] Replace generic labels with a small useful set for broker, extension, cookie-sync, providers, security, and documentation.
- [ ] Create a few real roadmap or good-first issues before advertising contributor labels.
- [ ] Keep Discussions disabled until recurring questions justify a public Q&A area.
- [ ] Fill the GitHub Project with a public roadmap or disable the empty tab.

## Phase 3: verification and repository protection

- [x] Replace the open Dependabot update with a manually verified `golang.org/x/crypto` update; do not dismiss security alerts as presentation cleanup.
- [x] Add CI for Go tests, race detection, vet, extension tests, shell/onboarding tests, Compose validation, and container smoke tests.
- [x] Enable CodeQL default setup for Go and JavaScript.
- [x] Add a CI badge only after the workflow is reliably green.
- [x] Protect `main` after the required check names exist.
- [x] Require pull requests and passing checks; block force-pushes and branch deletion.
- [x] Avoid mandatory external approval while the project has a single maintainer.
- [x] Prefer squash merges, delete merged branches, and allow branches to be updated before merge.
- [ ] Review GitHub Actions permissions and restrict third-party actions once the actual workflow set is known.

## Phase 4: reproducible preview release

- [x] Verify the current tree locally before tagging.
- [ ] Publish an honest `v0.3.0-preview`; do not create a fake stable release for the `latest` badge.
- [ ] Use release notes centered on the outcome: keeping remote consumers authenticated from a desktop browser.
- [ ] Include a tested/compatible/experimental matrix and upgrade notes.
- [ ] Attach a versioned extension ZIP and SHA-256 checksums.
- [ ] Consider Linux `amd64`/`arm64` binaries, a Compose bundle, SBOM, and provenance when the build pipeline is ready.
- [ ] Sign the release tag when a maintainable signing workflow exists.
- [x] Pin installation documentation to a release or tag instead of treating `raw/main` as a reproducible distribution channel.
- [x] Keep an inspect-before-run installation path alongside convenience commands.

## Phase 5: project identity

- [ ] Add an organization display name, one-line purpose, avatar, and profile README for `segfault-stack`.
- [ ] Pin Media Cookie Broker on the organization profile.
- [ ] Set a repository homepage only when a real landing page, documentation site, or demo exists; do not link the repository to itself.
- [ ] Keep the project name and generic broker model primary; `yt-dlp` remains the lead use case.

## Phase 6: external introduction

Start this only after the README example, visual proof, and a verified packaged release exist.

- [ ] Prepare a short reproducible story: remote `yt-dlp` cookies expire; desktop Chromium refreshes them; the NAS receives a fresh `cookies.txt`.
- [ ] State immediately that the same file/API contract works with other unattended consumers.
- [ ] Share selectively with self-hosted, data-hoarding, NAS, and `yt-dlp` communities.
- [ ] Submit to self-hosted newsletters or catalogs when installation and release artifacts are stable.
- [ ] Propose integration documentation to real consumers only where it provides concrete value; avoid promotional drive-by pull requests.
- [ ] Consider `awesome-selfhosted` only after its project-age requirement is met. Based on the first release date, the earliest useful review point is 2026-12-23.

## Things not to optimize for

- Topic count for its own sake.
- Implementation-detail discovery such as Manifest V3 or Go unless a contributor is specifically searching for those internals.
- An empty Discussions forum, Wiki, Project, or homepage.
- A premature stable release.
- Claims that plain `yt-dlp` automatically reports authentication failure.
- Claims that the broker automates login, CAPTCHA, 2FA, or upstream anti-bot controls.

## Next checkpoint

The next practical batch is:

1. create and upload the social preview;
2. add issue and pull request templates;
3. verify and merge the dependency update;
4. add CI and then protect `main`;
5. package `v0.3.0-preview` with checksums and clear release notes.
