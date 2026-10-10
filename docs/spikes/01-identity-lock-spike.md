# Phase 1 spike: TCC identity, grant persistence and lock delivery

This record holds signing requirements, hashes and dates only. Never paste
window titles, URLs or key material here.

## Baseline

- Date: 2026-10-10
- macOS: 26.6 (25G5065a)
- Signing: ad-hoc (`make bundle SIGN_MODE=adhoc`)
- DR: `designated => identifier "com.tock.trackd"`
- CDHash: `7e7d5809f59f0f45747d1596c0a8552ccb39cfd9`
- Identity `tock-local-codesign` imported into the login keychain (private
  key non-extractable, untrusted until `make cert-trust`); public cert at
  `~/.local/share/tock/codesign/tock-local-codesign.pem`.

## Identity decision

- Choice: **trusted** (2026-10-10). The user ran `make cert-trust` and approved
  the macOS authorization dialog.
- Makefile pinned to `SIGN_MODE ?= cert`.
- DR: `designated => identifier "com.tock.trackd" and certificate leaf = H"6a0b051872072fc35c1dd1892d44cb7ccbd25e09"`
- CDHash: `f1d10816007f4f13452d661619725677c955e2d6`
- Security note: certificate-anchored; grants survive rebuilds if A1 holds.

### Key access

key_access: always-allow

The user clicked Always Allow on the first `make bundle SIGN_MODE=cert`; the
partition-list route may also have been applied. A 30 s alarmed codesign of a
scratch binary and an unattended `make bundle` under a 300 s alarm both
succeeded with no dialog.

If a future `make bundle` reports "codesign timed out":

- In your own terminal run
  `security set-key-partition-list -S apple-tool:,apple:,codesign: -s -l tock-local-codesign ~/Library/Keychains/login.keychain-db`
  and type the login keychain password only at security's own prompt.
- Or run `make bundle SIGN_MODE=cert` interactively and click Always Allow.

## Persistence across rebuild

_Filled by plan 01-10._

## Lock delivery

_Filled by plan 01-10._

## Assumptions resolved

| ID | Assumption | Status |
|----|------------|--------|
| A1 | A self-signed cert trusted for codeSign gives a cert-anchored DR, and TCC keeps Accessibility/Automation grants across rebuilds on macOS 26.6 | pending plan 01-10 (cert-anchored DR confirmed: certificate leaf) |
| A2 | The Automation prompt from the launchd-run bundle is attributed to com.tock.trackd | pending |
| A3 | com.apple.screenIsLocked/Unlocked reach a faceless run-loop process, and the CGSSessionScreenIsLocked key name is correct | pending |
| A5 | AXIsProcessTrusted flips to true after a later grant without a restart | pending |
| A6 | `open --stdout /dev/ttysNNN` works for `make dump` | pending |
| A8 | com.apple.loginwindow is frontmost while locked | confirmed at planning time: com.apple.loginwindow is frontmost while locked |
