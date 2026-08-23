# Provider architecture

Provider support is source-defined for the public preview. There is no runtime plugin protocol.

## Server policy

`internal/providers/registry.go` is the authoritative broker allowlist. Each `Spec` has a conservative provider ID, cookie-domain roots accepted on upload, and optional authentication-cookie names used only for a reliable expiry hint. Broker authentication, upload validation, routes, and diagnostics consult this registry.

The broker core stores generic cookie revisions keyed by `(provider, profile)` and must not contain browser workflow logic. `cookie-sync` validates conservative provider/profile syntax, escapes URL segments, and lets the broker reject unknown providers. Targets use `provider=/path` for the `default` profile or `provider/profile=/path` for a named profile.

## Browser policy

`extension/providers/registry.js` exposes the provider definitions used by capture and UI code. Each module declares:

- `id`, `label`, and `cookieDomains`;
- capture/recovery context policy;
- default enablement and honest help text;
- optional capture validation;
- optional setup/navigation behavior.

The YouTube module owns interactive Google-login navigation, the `youtube.com/robots.txt` handoff, authentication-cookie hints, and its default-isolated recovery policy. Chromium shares incognito state across its incognito windows, so the spanning control plane checks incognito permission and refuses isolation while another incognito window or isolated recovery exists. Recovery records explicitly bind provider/profile and mode to the broker-created window, tab, and the cookie store resolved from that tab. Isolation can be disabled explicitly; normal mode uses the current ordinary browser cookie session and its broker profiles are not separate browser identities. Only the tracked broker-created incognito recovery window may be closed automatically. Google navigation is separate from cookie scope: Google cookies are not collected or uploaded.

Manifest host permissions remain explicit because Chromium requires user-approved website access. Provider cookie origins are declared at build time. The broker origin is an optional permission requested only when the user saves that endpoint.

## Adding a provider

1. Add the server `Spec` and domain-policy tests.
2. Add one extension provider module and registry import.
3. Add exact manifest host permissions and update the registry/manifest test.
4. Implement only provider-specific setup or validation justified by the real workflow.
5. Add fake fixtures and describe capture maturity in the README.

A new provider must not require changes to encryption, profile storage, authentication, health reports, Netscape rendering, or `cookie-sync` metadata parsing.
