# Security Policy

## Preview status

Media Cookie Broker is early-stage software that handles browser cookies. Cookies are sensitive bearer credentials, and deployers must review this threat model before use.

Only the latest public-preview release is expected to receive security fixes while the project is pre-1.0.

## Reporting a vulnerability

Do not open a public issue containing cookie values, exported cookie files, passwords, Authorization headers, broker master keys, account identifiers, or other live credentials.

Report security vulnerabilities through GitHub private vulnerability reporting for this repository: open the **Security** tab, go to **Advisories**, and select **Report a vulnerability**. This keeps the report and any sensitive evidence private while the maintainers investigate and coordinate a fix.

If private vulnerability reporting is temporarily unavailable, do not post sensitive details in a public issue, discussion, pull request, or other public repository content.

## Threat model

The project aims to reduce:

- plaintext cookie exposure in the broker database and backups;
- disclosure through application logs or diagnostics;
- access by credentials that are not assigned to an exact provider/profile and role;
- partial or broadly readable cookie files on consumers;
- collection of browser-cookie domains that a provider does not require;
- accidental inclusion of maintainer deployment details in public builds.

It does not protect credentials after full administrative compromise of the browser machine, broker host, or an authorized consumer. In particular, the running broker can access its master key, so encryption at rest does not defend against an attacker who obtains both the database and key.

The project also cannot prevent upstream cookie invalidation, account restrictions, anti-bot measures, or service policy changes.

## Deployment guidance

- Use HTTPS for every non-loopback broker connection.
- Use separate long random credentials for publishers, readers, and diagnostics readers.
- Grant each user only the exact provider/profile scopes it requires. A `youtube/default` grant does not include other YouTube profiles.
- Manage users locally with `brokerctl`; only Argon2id password hashes are stored in SQLite.
- Treat extension local storage as sensitive because it contains the dedicated publisher credential.
- Do not reuse an upstream account password as a broker password.
- Restrict filesystem access to broker secrets, database files, and synchronized cookie files.
- For the documented local Compose workflow, keep `secrets/` at `0700` and its file-backed `master-key` at `0444`. The parent directory prevents traversal by other ordinary host users, while the file mode lets the non-root container process read the bind-mounted secret. Compose file-backed secrets do not provide the stronger isolation of an external secret manager.
- Keep the broker behind a reverse proxy and appropriate network access controls rather than exposing plain HTTP publicly.
- Prefer authenticated cookies only when anonymous downloader access is insufficient.
- Treat metadata sidecars as private operational data even though they contain no cookie values.
- Remote extension diagnostics are off by default and require explicit enablement; local bounded redacted history remains available either way. Only provider/profile-scoped events are remotely eligible, so generic system/control-plane events remain local-only.
- Isolated YouTube recovery requires incognito access and no other open incognito window because Chromium shares one cookie store across incognito windows. The spanning extension records the broker-created window/tab and resolves its exact cookie store; it refuses rather than silently falling back to normal mode.

## Suspected compromise

1. Revoke or rotate the affected upstream browser/account session where possible.
2. Rotate the relevant broker user password.
3. If the master key may have leaked, deliberately recreate or re-encrypt stored snapshots with a new key.
4. Remove compromised consumer files and publish a fresh snapshot.
5. Review redacted diagnostics without posting raw secrets publicly.
