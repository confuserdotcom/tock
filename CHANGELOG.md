# Changelog

What changed for you in each release. Dates are UTC.

## Unreleased

### Added
- A new dashboard, in its own tock window from the menu bar (with a Dock
  icon while it's open). Today has a donut of the kinds of time, goal
  rings and minutes per hour next to the timeline; Week adds a heat map of
  when you focus and a projects donut; a new **Trends** view shows the
  month as a calendar, deep time per day and your best hours. ⌘K jumps to
  a view or a date, and every change can be undone from its toast.
- The open notch has pages: Today, This week and Goals, plus the session
  while one runs. Swipe sideways with two fingers, drag with the mouse, or
  use ← and → to move between them. Drag a bottom corner to make the panel bigger, like
  resizing a picture, or pinch: medium adds a chart per hour (or per day),
  large adds a second column. It remembers the size and the page.
- Install with Homebrew: `brew install confuserdotcom/tock/tock`, then
  `tock setup`. Homebrew builds tock on your Mac, so there is no
  "unidentified developer" warning, and `tock setup` makes and trusts the
  signing certificate, installs both apps and starts them. Run it again
  after `brew upgrade`; permissions stay granted. `tock setup --check`
  shows what is missing.
- A new icon, the day ring: a clock whose ring is split into deep, shallow
  and distracted time. It shows in the menu bar next to today's time, in
  the Dock while the dashboard is open, and as the dashboard's tab icon.
- tock lives in the notch. The menu bar app draws its own panel there: the
  running session, a break or a
  meeting either side of the notch, and moments that open from it and go
  back into it: a drift nudge, the end of a session, a meeting starting and
  the evening recap against your usual day.
- Labelling from the notch: the biggest unlabelled entries one at a time,
  the guess first, 1 to 3 to pick. Each pick saves a rule and says how much
  past time it labelled.
- `[notch] enabled`, `show_in_recordings`, `all_displays` and
  `open_on_hover`, and **Settings › Notch**. `tockbar notch-demo` plays
  sample moments for recordings.
- Every display gets the notch: one without a camera cutout gets a notch
  drawn at the top of its menu bar, and moments open where the pointer is.
- The notch grows a little under a resting pointer and opens on a click or
  a two-finger swipe down; a quick pass does nothing. A panel goes back once
  the pointer leaves it, and a session end or recap nobody saw waits as a
  dot. In full screen a drawn notch steps aside while nothing runs.

### Changed
- New kind colours everywhere (dashboard, terminal UI, menu bar and notch):
  teal for deep work, blue for shallow, mauve for browsing, brick for
  distraction.

## v5.1.0 — 2026-11-05

### Added
- The dashboard compares each number with your usual same weekday: the
  middle of the last four weeks.
- The dashboard's sidebar shows a month calendar and whether trackd is
  recording and rules.toml loads; a running session shows its goal and time
  left.
- `tock version` prints the build's version.
- A documentation site on GitHub Pages: an overview, the guide with a
  section list, and an architecture page with measured costs.
- A first run says what happens next: the dashboard shows three steps
  until anything is recorded, and `tock today`, `tock inbox` and the terminal
  inbox say "Nothing recorded yet" instead of "everything is labelled".
- CONTRIBUTING.md, SECURITY.md (private reporting through GitHub) and issue
  forms.

### Changed
- A new look for the dashboard: IBM Plex, greys, colour only for kinds of
  time, a timeline row per category, and tables with totals instead of bar
  lists. The terminal UI and the menu strip use the same kind colours.
- The README is short; the full manual moved to `docs/guide.md`.
- The score breakdown in the terminal says what each part is: deep time,
  long blocks, few switches, longest run. Session cards read "on task ·
  longest · few interruptions · few switches". The config keys are the same.
- `tock help` groups the commands, with the optional extras last, and now
  lists `tock web`.
- The dashboard's hint when trackd is down points to `tock doctor`; it named
  a `tock install` command that does not exist.
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
