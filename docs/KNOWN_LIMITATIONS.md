# Known limitations

- This is an early public preview, not audited production account infrastructure.
- Only Chromium unpacked-extension installation is currently supported.
- YouTube is the primary tested workflow. TikTok, Instagram, and X / Twitter capture is experimental and is not validated against every downloader or account state.
- YouTube snapshots are not renewed automatically after an interactive recovery context closes; a human still completes provider authentication.
- Chromium shares one cookie store across all incognito windows. Isolated YouTube recovery therefore requires all other incognito windows to be closed and refuses concurrent isolated flows; it is not a separate Chrome profile.
- YouTube normal recovery uses the operator's current ordinary browser session. Broker profile labels do not isolate normal-browser accounts from one another.
- Auth-expiry warnings exist only where provider policy identifies a reliable explicit authentication-cookie expiration. They are hints, not guaranteed failure times.
- The extension stores its dedicated broker publisher password in `chrome.storage.local`, not an operating-system keychain.
- Encryption at rest does not protect a broker host when an attacker can read both the database and mounted master key.
- File-only consumers cannot report authentication failure unless an application or wrapper invokes the normalized report API.
- One current-revision `authentication_required` report immediately requests refresh; quorum and post-refresh verification are not implemented.
- Profiles are foundational in storage, ACLs, APIs, sidecars, and extension status, but profile-management UX is intentionally minimal.
- Remote diagnostics are redacted, profile-scoped, off by default, and remain a preview operational feature rather than a telemetry platform. Generic events without provider/profile scope remain in local diagnostics only.
- There is no runtime provider loading, Firefox package, automated login, OAuth client support, replication, or web administration UI.
