# Security

tock records which apps and sites you use, so a bug that leaks that data is
a security bug even if it needs local access.

## Reporting

Please report privately through GitHub:
[Report a vulnerability](https://github.com/confuserdotcom/tock/security/advisories/new).
Don't open a public issue. You should hear back within a week.

Useful things to include: the version (`tock version` and `tock doctor`), the OS, what an
attacker can read or change, and steps to reproduce. For an ordinary bug,
`tock bug-report` writes a file without your titles or URLs to attach.

What has been checked, and where to look: [docs/security-review.md](docs/security-review.md).

## In scope

- Activity leaving the machine when it shouldn't: from trackd or tockbar at
  all, or from `tock` beyond the Claude and Slack features you turned on.
  trackd has no network code, but it runs `tock slack` at the start and
  end of a focus session when Slack is on; that child is in scope too.
  [docs/privacy.md](docs/privacy.md) lists what each feature sends.
- The dashboard on 127.0.0.1: other pages or local users reading it without
  the per-run token, or the page loading anything from the internet.
- File permissions on the database, backups and sync files.
- Private-window and hidden-app titles or URLs being stored, or recorded
  while `config.toml` does not load.
- Screen text (OCR) or calendar events leaving the machine, or OCR reading
  an app you did not list.
- Linux updates through the sync folder: a build installed without a valid
  signature from the key you trusted with `tock update trust`, a key
  trusted without that step, or updates running with `[sync] update` unset.

## Out of scope

- Someone who can already write to your sync folder reading your activity
  in it: the folder holds your spans by design. Pick a sync tool and
  sharing settings you trust.
- Someone with your user account on the machine: they can read tock.db
  like any of your files.

## Supported versions

Only the latest release gets fixes.
