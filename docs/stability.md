# Stability

What tock promises about your data, what it doesn't, and what has to be
true before a release ships. Where a promise has a test, the test is named,
so you can check that it still holds.

## What is stable and what is not

**Stable** means it keeps working across releases, and a change that
breaks it is called out in CHANGELOG.md with what to do.

| Stable | Experimental |
| --- | --- |
| Recording: front app, window title, browser URL, idle, sleep and lock | Screen text (`[ocr]`) |
| `tock.db`: the schema rules below, backup, restore | Calendar meetings (`[calendar]`) |
| Rules, tags, the inbox, `tock edit`, `tock undo` | Claude summaries and suggestions (`[llm]`) |
| Issued invoices: numbers, amounts, files | Slack status (`[slack]`) |
| `tock today`, `week`, `export`, `status`, `doctor` and their flags | Block distractions, urge surfing, wake and break screens |
| `config.toml` keys documented in the guide | The notch |
| | Linux self-update (`[sync] update`) |

Experimental features are off unless you turn them on, except the notch on
a Mac with one. They can change shape between minor releases, and the
CHANGELOG says when they do. Turning one off never changes what was
recorded.

Focus, Day routine, Billing and AI are groups you turn on or off with
`tock choice`. Turning a group off hides it; it never changes what trackd
records or how time is labelled (`TestGroupsKeepLabels`).

Retired commands stay one release hidden from `--help`, with a one-line
hint saying what replaces them, and are removed in the next.

## Platforms

| | Tested on | Notes |
| --- | --- | --- |
| macOS | macOS 26 on Apple Silicon | Built on your Mac (Homebrew or `make install`), signed with a local certificate; no notarized app |
| Linux | Hyprland on amd64 | Built without cgo; needs Hyprland for window events and hypridle for idle |

Other Linux desktops record nothing yet. Windows is not supported. CI
builds and tests every change on Linux without cgo and on macOS with it.

## Your data

### What tock guarantees

- **Nothing is rewritten in the background.** trackd only adds spans and
  extends the open one. Labels are worked out each time a day is read, so
  a rule change relabels past time, but the recorded spans stay as they
  were. Deleting titles (`[privacy] keep_titles`, `tock prune`) is the one
  thing that removes recorded detail, and it keeps the labels the titles
  gave.
- **A copy before every schema change.** Before a migration, tock writes
  a compressed, checked copy of the database to `backups/auto/`. If it
  can't (disk full, damage found), the migration doesn't run and tock says
  why (`TestOpenCopyFailureBlocksMigration`).
- **A copy every day.** trackd takes a daily copy while you're away and
  checks it; the newest 7 are kept (`[backup] keep`). Damage found in a
  copy shows in `tock status` (`db!`), `tock doctor` and the menu bar.
- **Restore never races a writer.** `tock restore` stops trackd and the
  menu bar, refuses while anything else holds the database and names it,
  and keeps the database it replaces (`TestRestoreRefusesWhileHeld`).
- **Issued invoices don't change.** The database refuses any change to an
  issued invoice, from tock or from SQL (`TestInvoiceTriggers`).
  Its PDF and JSON are written next to it, and `tock invoice verify`
  compares both with the hashes stored when it was issued.
- **Outages are visible.** When trackd isn't recording, the day says so
  and why: not running, crashed, computer off, logged out, paused by a
  config error, or apps only without Accessibility
  (`TestRunCrashThenRestart`).
- **A config error stops recording, never privacy.** If `config.toml`
  doesn't load, trackd records nothing until it does, so hidden apps are
  never recorded with their titles (`cmd/trackd/configpause_test.go`).

### What it doesn't

- Copies on the same disk don't survive the disk. Time Machine or the
  sync folder does.
- A crash loses at most the last 10 seconds of the open span: trackd
  writes it every 10 seconds.
- An invoice's files can be edited or deleted outside tock. `tock invoice
  verify` notices; it can't undo it.
- Restoring a copy loses what was recorded after it. `tock restore` says
  how much and asks first.

## Upgrading

- **From v5.0.0 or later:** install the new version; the database is
  migrated on first open. CI opens a database written by every release
  since v5.0.0 (`internal/store/testdata/upgrade`, `TestUpgradeFixtures`)
  and checks every row is still there.
- **From v4.x or older:** `tock backup` with the old version, install the
  new one, and restore if anything looks wrong. This path is not tested.
- **Downgrading:** an older tock opens a newer database as long as the
  changes were additive, which is the rule (next section). If they were
  not, the older trackd stops, records why, and `tock status` shows `old!`
  with the command that updates it.

### Schema rules

Every program that opens `tock.db` (trackd, tock, the menu bar) may run a
different version for a while: `brew upgrade` replaces `tock` before
trackd restarts. So:

1. A migration only adds: new tables, new columns with a `DEFAULT`, new
   indexes. `TestMigrationsAreAdditive` fails on `DROP`, `RENAME` and
   `NOT NULL` without a default.
2. A change that has to break older programs raises `compat` in
   `schema_meta`, carries `-- breaks-older-writers` and says so in
   CHANGELOG.md. No release has needed one yet.
3. trackd checks `compat` every 10 seconds, so an older trackd stops
   cleanly instead of writing rows a newer schema doesn't expect.
4. A new `config.toml` key defaults to the old behaviour.

## Cost

Measured on an Apple Silicon Mac (M2), November 2026, on the developer's
own month of data (28,000 spans):

| | Measured | Budget |
| --- | --- | --- |
| trackd CPU, averaged over 2.6 days | 0.5 % | under 1 % |
| trackd CPU, a browser in front | 1.3 % (15 minutes) | under 1 %: not met |
| trackd memory | 36 MB footprint (peak 48 MB) | 40 MB |
| Menu bar app memory | 54 MB footprint | |
| A focus tick (every 5 s), real month of data | 3.4 ms (`BenchmarkGuardTick` on 180 synthetic days: 2.7 ms) | |
| Building a day, 180 days of history | 2.0 ms (`BenchmarkDay`) | |
| Building a week, 180 days of history | 4.8 ms (`BenchmarkWeek`) | |
| Turning a workday of samples into spans | 0.8 ms for 14,400 samples (`BenchmarkMergerWorkday`) | |

With a browser in front, trackd asked it for the tab's address every 2
seconds (an Apple Event), which is most of the difference between the two
CPU lines. It now asks when the window or its title changes, and otherwise
at most every 10 seconds; the 1.3 % line predates that and is due a
re-measure. Reading a day takes the same time after a year as after a week: range
reads are bounded on both sides by an index (`TestSpansRangeIsBounded`).
`make bench` runs the benchmarks. They are not a CI gate yet.

Measure trackd yourself with `footprint $(pgrep trackd)`; `ps` misses the
memory SQLite allocates outside the Go heap and counts shared framework
pages.

## Before a release

A release candidate ships only when each of these holds. Passing them is
evidence, not proof.

1. `make test` and CI green on Linux (no cgo) and macOS; `make web-check`
   clean.
2. The migration lint passes, and the upgrade fixtures open with every
   row, including one written by the previous release.
3. Restore round trip: snapshot, restore, then equal row counts and
   invoice hashes.
4. Billing: the invoice tests pass, and an issue, relabel, show and check
   run by hand on a copy of a real database gives the issued numbers.
5. Diagnostics: the crash and outage tests pass; `tock bug-report` leaves
   out every title, URL and config value.
6. `govulncheck ./...` clean, or each finding written down with why it
   doesn't apply.
7. The cost table above measured on this release.
8. README, the guide, privacy.md, this page and SECURITY.md read through
   against what the release does.
9. No open report of lost data or wrong numbers.
10. The maintainer signs off.

After a release candidate, four weeks of bug fixes only before the final
release.
