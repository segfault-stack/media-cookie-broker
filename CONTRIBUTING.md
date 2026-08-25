# Contributing

Contributions are welcome, especially focused provider fixes, tests, security improvements, and documentation corrections appropriate to an early public preview.

## Never submit real credentials

Do not include any real browser cookie value, exported `cookies.txt`, Authorization header, password, broker key, account identifier, private endpoint, or sanitized production dump in commits, tests, issues, or pull requests. Use unmistakably fake values such as `not-a-real-cookie` and `example.invalid`.

## Keep changes narrow

Preserve encrypted revisions, provider/profile-scoped authorization, revision-aware health semantics, bounded inputs, redacted diagnostics, ETag behavior, and atomic mode-`0600` consumer writes. Avoid speculative runtime plugin systems or unrelated platform infrastructure.

## Adding or changing a provider

Provider-specific behavior should not enter the broker core.

1. Add or update the server policy in `internal/providers/registry.go`.
2. Add or update one module in `extension/providers/` and register it in `extension/providers/registry.js`.
3. Declare only the browser host permissions needed for its cookie domains.
4. Keep setup, authentication-expiry hints, and recovery-context rules in provider policy.
5. Add fake-cookie tests for accepted and rejected domains.
6. Update the provider table and limitations.
7. Verify every existing provider still works.

See [docs/PROVIDERS.md](docs/PROVIDERS.md) for the current contracts. YouTube's default incognito recovery is intentionally provider-specific and user-disableable. Isolated recovery must reject shared/existing incognito state and close only its broker-owned window; normal recovery contexts must never be auto-closed.

## Development checks

Clone with submodules, or initialize an existing checkout before repository-maintenance work:

```bash
git submodule update --init --recursive
```

The immutable playbook revision and resolved commit are recorded in `PROJECT_AGENT_CONTEXT.md`. To update it, review a newer release, check out that exact tag in `.agent/oss-playbook`, update the single recorded pin, run the playbook and project checks, and commit the gitlink and context together. Roll back a problematic update by reverting that dependency-style commit.

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
```

When Docker is available:

```bash
docker build --target broker -t media-cookie-broker:preview .
docker build --target cookie-sync -t media-cookie-broker-cookie-sync:preview .
COOKIE_BROKER_TEST_IMAGE=media-cookie-broker:preview \
COOKIE_SYNC_TEST_IMAGE=media-cookie-broker-cookie-sync:preview \
tests/container-smoke.sh
```

Do not suppress, weaken, or delete a failing test to make a contribution green.
