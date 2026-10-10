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

build1_dr: designated => identifier "com.tock.trackd" and certificate leaf = H"6a0b051872072fc35c1dd1892d44cb7ccbd25e09"
build1_cdhash: 87673b86069f6644771984e11557431180145442
ax_before_grant: 1

Build 1 (version 1ba5208) was launched with `make dump-bg`; its banner ended
`ppid=1` with no terminal notice. It already reported ax=1 before the Task 2
grant step, so an Accessibility grant made to an earlier cert-signed build
(CDHash f1d10816…) was honoured by this one.

Build 2 (`make bundle VERSION=spike-rebuild`, a different binary):

build2_dr: designated => identifier "com.tock.trackd" and certificate leaf = H"6a0b051872072fc35c1dd1892d44cb7ccbd25e09"
build2_cdhash: 9dc3bb71210a1d6068d947269fccca3a0cbcbb5d
dr_identical: yes
cdhash_differs: yes
ax_after_rebuild: 1
result: PASS

No new grant was made between builds; the 8 s dump of build 2 (banner
`version=spike-rebuild ... ppid=1`) reported ax=1 on every sample.

## Lock delivery

Date: 2026-10-10. Build 1 dump, about 4 minutes, two lock/unlock cycles
(Ctrl+Cmd+Q, the first held about 37 s).

lock_events: 2
unlock_events: 2
locked_samples: 26
loginwindow_samples: 24
activation_samples: 26
ax_flip_without_restart: untested
fallback_applied: none
result: PASS

All three lock sources fired: `com.apple.screenIsLocked`/`Unlocked`
distributed notifications reached the faceless process with the existing
block observers, `CGSSessionScreenIsLocked` gave locked=1 samples, and
com.apple.loginwindow was frontmost while locked. ax_flip_without_restart is
untested because Accessibility was already granted before the dump started
(every sample was ax=1), so no live grant transition occurred.

## Assumptions resolved

| ID | Status | Assumption |
|----|--------|------------|
| A1 | confirmed: build 2 (CDHash 9dc3bb71…, identical DR) reported ax=1 with no re-grant | A self-signed cert trusted for codeSign gives a cert-anchored DR, and TCC keeps Accessibility/Automation grants across rebuilds on macOS 26.6 |
| A2 | pending plan 01-11 checklist | The Automation prompt from the launchd-run bundle is attributed to com.tock.trackd |
| A3 | confirmed: 2 lock and 2 unlock events, 26 locked=1 samples, no fallback needed | com.apple.screenIsLocked/Unlocked reach a faceless run-loop process, and the CGSSessionScreenIsLocked key name is correct |
| A5 | pending: not exercised, the grant already existed before the spike | AXIsProcessTrusted flips to true after a later grant without a restart |
| A6 | confirmed for files: `open --stdout <file>` via make dump-bg captured banner and samples; tty path not exercised | `open --stdout /dev/ttysNNN` works for `make dump` |
| A8 | confirmed: 24 loginwindow samples during the two locks | com.apple.loginwindow is frontmost while locked |
