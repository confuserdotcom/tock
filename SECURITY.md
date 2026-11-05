# Security

tock records which apps and sites you use, so a bug that leaks that data is
a security bug even if it needs local access.

## Reporting

Please report privately through GitHub:
[Report a vulnerability](https://github.com/confuserdotcom/tock/security/advisories/new).
Don't open a public issue. You should hear back within a week.

Useful things to include: the version (`tock version` and `tock doctor`), the OS, what an
attacker can read or change, and steps to reproduce.

## In scope

- Activity leaving the machine when it shouldn't: from trackd or tockbar at
  all, or from `tock` beyond the Claude and Slack features you turned on.
- The dashboard on 127.0.0.1: other pages or local users reading it without
  the per-run token, or the page loading anything from the internet.
- File permissions on the database, backups and sync files.
- Private-window and hidden-app titles or URLs being stored.

## Supported versions

Only the latest release gets fixes.
