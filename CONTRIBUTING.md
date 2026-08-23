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

```bash
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
npm --prefix extension test
docker compose config
```

When Docker is available:

```bash
docker build -t media-cookie-broker:preview .
COOKIE_BROKER_TEST_IMAGE=media-cookie-broker:preview tests/container-smoke.sh
```

Do not suppress, weaken, or delete a failing test to make a contribution green.
