# Security review

An internal review, so an outside reviewer can check each claim instead of
trusting it. Each line names where to look. Last done 2026-11-13 against
main; report anything that does not hold through
[SECURITY.md](../SECURITY.md).

## What is worth protecting

Which apps, windows and sites you used, and when. It lives in one SQLite
file plus backups, the sync folder if you use one, and short-lived logs.

## Checked

**Nothing leaves the machine unless you turned it on.**
- trackd imports no HTTP client; its only network-ish code is the desktop
  sockets (`internal/collect/hypr.go`, `i3ipc.go`, `x11.go`).
- `tock` talks to Claude (`internal/claude`) and Slack (`internal/slack`)
  only when configured. What goes to Claude passes `suggest.Filter`
  (`internal/suggest/filter.go`): private windows, `[llm]` deny apps,
  hosts and titles, and items under the minimum time are dropped.
- [privacy.md](privacy.md) lists what each feature sends.

**The dashboard on 127.0.0.1** (`internal/web/server.go`)
- Requests with any Host but `127.0.0.1:<port>`, `localhost:<port>` or
  `[::1]:<port>` are refused, which stops DNS rebinding.
- Every API call needs a 256-bit per-run token, compared in constant time.
  The page gets it from the URL fragment, which browsers never send.
- CSP `default-src 'self'`, `frame-ancestors 'none'`, `X-Frame-Options:
  DENY`, same-origin COOP/CORP: no framing, nothing loaded from outside.

**Files** — data, state and invoice folders are made 0700 and chmodded
if they existed; the database, its WAL and SHM, backups (VACUUM INTO then
chmod), sync files, tokens and keys are 0600 (`internal/store/store.go`,
`internal/snapshot/file.go`, `internal/syncdir`, `cmd/tock/cmd_slack.go`).

**Updates** (`internal/update`) — a manifest of sha256 sums signed with
ed25519; the key is trusted once with `tock update trust` and pinned by
fingerprint; each file is checked before it replaces the old one.

**Stopping trackd on Windows** (`internal/stopsignal/stopsignal_windows.go`)
— the event's DACL grants only the current user's SID.

**Bug reports** (`cmd/tock/cmd_bugreport.go`) — config values are
stripped, log lines that could hold a title or URL are dropped, rules
are left out unless asked for; nothing is sent.

**Dependencies** — `govulncheck` runs in CI and before every release
(`.github/workflows/ci.yml`, `release.yml`); `npm audit --omit=dev` on
`web/`: 0 vulnerabilities. Dependabot is on.

**Releases** — tarballs and the .dmg get GitHub build provenance
(`gh attestation verify <file> -R confuserdotcom/tock`) and are listed in
`SHA256SUMS`. The macOS app is ad-hoc signed, not notarized: there is no
Apple Developer ID, so the first open needs Privacy & Security → Open
Anyway.

## Not covered

- Another process running as you: it can read tock.db like any of your
  files. Out of scope by design.
- The sync folder's own sharing and encryption.
- An independent reviewer. This page was written by the project.
