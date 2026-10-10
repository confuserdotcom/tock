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

_Pending: filled after the user trusts or declines the certificate._

### Key access

_Pending._

## Persistence across rebuild

_Filled by plan 01-10._

## Lock delivery

_Filled by plan 01-10._

## Assumptions resolved

| ID | Assumption | Status |
|----|------------|--------|
| A1 | A self-signed cert trusted for codeSign gives a cert-anchored DR, and TCC keeps Accessibility/Automation grants across rebuilds on macOS 26.6 | pending |
| A2 | The Automation prompt from the launchd-run bundle is attributed to com.tock.trackd | pending |
| A3 | com.apple.screenIsLocked/Unlocked reach a faceless run-loop process, and the CGSSessionScreenIsLocked key name is correct | pending |
| A5 | AXIsProcessTrusted flips to true after a later grant without a restart | pending |
| A6 | `open --stdout /dev/ttysNNN` works for `make dump` | pending |
| A8 | com.apple.loginwindow is frontmost while locked | confirmed at planning time: com.apple.loginwindow is frontmost while locked |
