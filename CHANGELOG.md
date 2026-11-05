# Changelog

What changed for you in each release. Dates are UTC.

## Unreleased

### Changed
- trackd uses about 10 MB less memory: its focus, sync and title-deletion
  loops share one database session.
- Pushing a version tag publishes the GitHub release with the Linux tarballs.

## v5.0.0 — 2026-11-05

The first public release.

### Added
- `tock import activitywatch <export.json>` brings your ActivityWatch history
  in. Your rules label it, `[privacy]` hide lists apply, and importing again
  replaces it instead of doubling it.
- `tock uninstall` stops trackd and the menu bar and removes the apps and
  `tock`. `--purge` deletes your data, rules and config too, after you type yes.
- ⌥⌘F starts a focus session with a goal from any app, and `tock://` links for
  Shortcuts and Raycast.
- A wake screen for sessions that drift off task, `tock break`, and
  `session start --paper` for work away from the screen.
- One privacy list of hidden apps and sites, managed from **Settings ▸ Privacy**
  in the menu bar.
- `tock choice` turns features on and off.
- An inbox that stays nearly empty: one key labels a block and learns a rule.
- `make release` builds the Linux tarballs with SHA256SUMS; CI runs on Linux
  and macOS.

### Changed
- A calm menu bar menu: today's time, a colour strip of the day, one Start
  button and a Settings submenu.
- One design system (paper, ink and brass) for the dashboard, the TUI and the
  menu.
- Mouse jigglers no longer end an absence, and notices have a daily budget.
- `make deploy-linux` needs `LINUX_HOST=<host>`; it has no default machine.

### Before v5
v1 to v4 (October 2026) were built before this changelog. See `git log`.
