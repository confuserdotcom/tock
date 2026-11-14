# Stability and support

This page says what you can rely on from one tock release to the next, how
the version number tells you when that changes, and which platforms are
supported how well. It is a promise: a release that breaks it without a new
major version is a bug.

## Version numbers

tock uses [semantic versioning](https://semver.org) from v1.0.0 on:

- **Patch** (1.2.3 to 1.2.4): fixes only. Nothing you rely on changes.
- **Minor** (1.2 to 1.3): new commands, flags, config keys, API fields and
  migrations. Everything below keeps working.
- **Major** (1.x to 2.0): something below stops working the way it did.
  The CHANGELOG lists each break and what to do about it.

Features ship in minor releases, however large. A major release is only for
a real break, so it should be rare.

Releases before v1.0.0 (0.1.0 to 0.6.1) came before this promise and are
not covered by it.

## What is stable

Within one major version:

### Config

- Every key documented in the [guide](guide.md) for `config.toml` and
  `rules.toml` keeps its name, its type and its meaning.
- A key may gain new accepted values. Its default may change only in a
  minor release that says so in the CHANGELOG, and never in a way that
  sends more data off the machine or records more than before.
- tock rejects unknown keys. A config written for 1.N loads in every later
  1.x; one written for a later minor may not load in an earlier one.

### Command line

- Every command and flag listed by `tock help` keeps its name and meaning.
- Exit codes keep their meaning: 0 ok, 1 error, 2 usage, 3 `tock invoice
  check` found a problem, 4 `tock invoice verify` found a file that differs.
- Machine-readable output keeps its fields: `tock doctor --json`, `tock
  export --format csv|json`, and the other `--json` outputs. Fields may be
  added; none are removed or renamed, and none change type.
- Text meant for people (tables, colours, wording, the TUI) may change in
  any release. Scripts should use the machine-readable output.
- A command or flag to be retired is first hidden from help with a one-line
  hint naming its replacement, and still works, for the rest of the major
  version. It is removed only in the next major.

### Database

- `tock.db` only moves forward through numbered migrations, applied when a
  newer tock or trackd opens it.
- Additive migrations (new tables, columns, indexes) leave the database
  usable by older 1.x binaries: an older trackd keeps recording into a
  database a newer tock migrated.
- A migration that older binaries cannot work with raises the database's
  compat level. That only happens in a major release; older binaries then
  refuse to write and `tock status` says why.
- trackd snapshots the database before each migration, so `tock restore`
  can go back.
- The tables themselves are not an API. Read your data through `tock
  export`, which is stable, rather than with SQL, which is not.

### Sync folder

Machines on different 1.x releases can share one sync folder: each writes
files the others read, and fields are only ever added.

### Dashboard API

The HTTP API under `/api/` in `tock web` exists for the dashboard that ships
in the same binary. It needs the per-run token, may change in any release,
and is **not** stable. Use `tock export` or the `--json` outputs for
scripts. A stable, documented API may be added in a later minor release; it
will be listed here when it is.

### Not covered

Go packages under `internal/` (not importable from other modules), log
formats, the files in the state folder other than the ones the guide
documents, and anything marked *experimental* or *preview* in the guide.

## Support tiers

| Tier | Platforms | What it means |
| --- | --- | --- |
| **1** | macOS on Apple Silicon and Intel | The daily-driver platform. Every feature, including the menu bar app and the notch. CI and end-to-end tests on every commit; release builds and the `.dmg`. Bugs here block a release. |
| **2** | Linux on Hyprland, Sway, i3 and X11 window managers | Recording, the CLI, the TUI, the dashboard and sync. CI on every commit and end-to-end tests on Sway, i3 and an X11 window manager; release tarballs. No menu bar app or notch. Bugs are fixed, but may wait for a later release. |
| **Experimental** | Windows 10 and 11 | Builds from source and records the front window. CI runs the tests, but there is no release build and no promise: anything on Windows may change or break in any release, and it is not covered by the stability rules above. |

Other Linux desktops, such as GNOME and KDE on Wayland, are not supported
yet: trackd has no way to read their front window.
