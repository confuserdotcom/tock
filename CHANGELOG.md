# Changelog

What changed for you in each release. Dates are UTC.

## Unreleased

### Added
- **Time per commit.** `tock commits` lists the commits of your project
  repos, each with the project time since the one before, and the time
  since each project's last commit. `--days N`, `--week`, `--project P`.
- **Agent ledger.** `tock agents` reads Claude Code's session logs and
  shows, per project, how long agents worked, how much of it you spent in
  their windows, and the prompts you typed. It reads times, directories
  and session titles only, never what was said.

### Fixed
- Commit counts were 0 when the repo's `user.email` has a `+` in it, as
  GitHub noreply addresses do.

## v5.4.0 — 2026-11-12

### Added
- **Why this label.** Clicking an entry on the dashboard timeline says
  what labelled it: the rule (file, number and what it matches), your
  tag, or the local model with its share and the title words behind it.
  The Inbox shows why it guesses a label under each item instead of in
  a tooltip.
- **Templates.** `tock setup --template client|own` adds the categories
  and rules for client work or your own projects that you don't have
  yet. It shows the plan first, never touches what is yours, labels from
  now on unless `--past`, and `tock undo` takes it back. A fresh
  `tock setup` asks which to start with.
- **Rules from a date.** `since = 2026-11-20T14:05:00` on a rule makes it
  label only time from then on, so a new rule need not relabel the past.
- **Dashboard settings.** The gear in the top bar (or `,`) turns the
  groups and features on and off as `tock choice` does, sets your work
  hours, session lengths and invoice details (with clients, which a
  project picks in the Library), edits the apps and sites tock never
  records, and picks the theme. config.toml keeps its comments.
- **Zoom.** The dashboard timeline zooms to a quarter of an hour (⌘-scroll,
  pinch, double click, shift-drag, or the − + buttons), with a strip of
  the whole day to drag. The tock window zooms with ⌘+, ⌘− and ⌘0 and
  remembers the size.
- **Time trackd did not record is shown, with why**: a crash, a stop,
  an update or `config.toml` not loading. The TUI's today tab lists it
  under the day, the dashboard draws a "Not recording" lane under the
  timeline and a line under the week's bars. Sleep, a locked screen, a
  logout and a shutdown are not counted; a restart only explains the time
  when the OS said the session was ending as trackd stopped, so a stop by
  hand before a restart still shows. Time with no Accessibility permission
  shows apart as "apps only". None of it is added to tracked or idle time.
- **Library** in the dashboard: categories, projects (with an hourly
  rate) and goals, with their time in the last 30 days and when each was
  last seen. Add, edit, rename, archive and merge look-alike names there.
- **Rules** in Library: every rule in the order tock checks them, with the
  time each labelled lately and which never match. Add, edit or delete a
  rule, learned ones too, with a preview of what it would change; every
  edit can be undone.
- **Invoice…** in the week's Export menu makes the `tock invoice` PDF for
  a project with a rate, showing hours and total before you save.
- trackd keeps a copy of your database every day while you're away, and
  tock keeps one before an update changes it. `tock restore` lists them
  and puts one back. `[backup] keep` sets how many daily copies stay
  (default 7).
- `tock doctor` shows the last copy and whether the database checked out;
  `--deep` runs a full check. A damaged database shows in `tock status`
  and as a notice.

### Changed
- trackd asks the browser for the tab's address only when the window or
  its title changes, and otherwise every 10 seconds instead of every 2.
  Less CPU with a browser in front.
- **The dashboard answers clicks.** Hours, work blocks and lane names zoom
  the timeline; kinds, categories and apps pick out their bars; a day in
  the week's bars, the score line or the month calendar opens it; Top rows
  lead to the Inbox or the Library; the status strip links to the rules.
  The date in the top bar steps by day, week or month with the view and
  opens a calendar. `?` lists every key and gesture.
- **Billed time says so everywhere.** Learning a rule from the inbox (TUI,
  CLI or dashboard) now says when it relabels time on an issued invoice,
  as tagging does, and the TUI marks billed rows with the invoice number.
- **Focus, Day routine and Billing are groups you can turn off.** `tock
  choice focus off` hides sessions, the focus score and every focus
  notice in the TUI, the dashboard, the menu bar and the notch, and trackd
  stops them; `tock choice billing off` hides rates and invoices and stops
  `tock invoice issue`; `tock choice routine off` stops the day's notices.
  Each is one line in config.toml, and the members keep their own settings
  while their group is off. All three are on unless you turn them off, so
  nothing changes for you on upgrade. `tock choice` now lists the switches
  by group.
- **Invoices are issued and kept, and never change after.** `tock invoice`
  now previews (a PDF marked DRAFT, nothing stored), and `tock invoice
  issue` issues: the next `YYYY-NNNN` number, stored with its PDF in the
  database and in `~/.local/share/tock/invoices/`. Later rule edits,
  relabels or a new rate change nothing in it; `tock invoice check` shows
  how today's labels differ. Corrections are a void with a reason and a
  replacement with `--supersedes`. `tock invoice show` prints an issued
  invoice; `--json` gives the stored record byte for byte. Amounts are exact (integer cents), days
  can be rounded (`[invoice] round`), and an invoice can carry your and the
  client's tax numbers and one tax rate. Projects can set `billable =
  false` and `client = "…"`. The old `YYYYMM-project` numbers are retired,
  and PDFs made before this release are not imported. The dashboard's
  **Invoice…** saves a DRAFT preview or issues, with the same checks as
  the CLI. Billed time is underlined on the timeline, and relabelling it
  says the invoice stays as issued.
- **Linux updates through the sync folder are off until you turn them
  on, and never trust a key by themselves.** Before, a Linux machine
  trusted the first key it found in the sync folder, so anyone who could
  write there first could run code on it. Now: `make bootstrap-hint` on
  the Mac prints the key's fingerprint, `tock update trust <fingerprint>`
  on the Linux machine pins it, and `update = true` under `[sync]` turns
  updates on. A machine that already pinned a key keeps it, but needs
  `update = true`. An update that fails part way puts both binaries back.
  The one-time install command now checks both files against sums it
  carries itself.
- **Claude is off until you turn it on.** Having Claude Code or
  `ANTHROPIC_API_KEY` no longer turns on suggestions, summaries or
  `tock ask`. `tock choice claude on` turns them on and lists what is sent.
  If you used them before, run it once; the TUI says so the first time.
- **trackd records nothing while `config.toml` does not load**, for
  example after a typo or with a setting from a newer tock. Before, hidden
  apps and sites could be recorded with their titles and URLs in that
  state. `tock status`, `tock doctor`, the TUI, the dashboard and the menu
  bar say why, and recording starts again within seconds of the fix.
- trackd upgrades the database in a short-lived child process, so the
  safety copy taken before an upgrade no longer leaves trackd about 7 MB
  larger until its next restart.
- **An older trackd keeps running after a newer `tock` updates the
  database**, as long as the update only adds to it (all of them so far).
  Before, a trackd restarted after `brew upgrade` but before `tock setup`
  refused the database and stayed down with no word outside `tock doctor`.
  When an update does need a newer trackd, trackd stops, and `tock
  status`, `tock doctor`, the TUI and the menu bar say "trackd is older
  than the database" with the command that fixes it for your install.
- **Failed writes and dropped samples are counted and shown.** `tock
  status` starts with `write failed!` or `dropped!` for 24 hours after
  one, and `tock doctor` says when and how many. Before, both were only in
  trackd's log.
- **`tock bug-report`** writes a file to attach to an issue: doctor's
  checks, your config with every text value removed, the end of the logs
  without titles or URLs, and when trackd ran. It sends nothing.
  `tock doctor --json` prints the checks for scripts.
- trackd and the menu bar keep their previous log as `trackd.log.1` and
  `tockbar.log.1` when the log passes 10 MB, instead of emptying it.
- Fix hints follow how tock was installed: `tock setup` for Homebrew,
  opening tock for the .dmg, `make install` from source.
- **`tock restore` refuses while something else has the database open**
  and names it, for example a `tock web` you started by hand. Before, the
  copy was put back under it and that process went on using the old file.

### Fixed
- **Private windows in more browsers.** Microsoft Edge, Vivaldi, Chromium
  and the beta, dev and nightly builds of Chrome, Edge and Brave were
  recorded like any app, so an InPrivate or incognito window's titles
  were kept. They now get the URL of the front tab, as Chrome does, and
  nothing at all from a private window. Firefox forks (Zen, LibreWolf,
  Waterfox, Floorp, Developer Edition, Nightly) are handled like Firefox.
  Opera, Dia, Orion, Tor Browser, Mullvad Browser and a few others keep
  the app only, without titles, because tock cannot tell their private
  windows apart. The first time you use one of the newly supported
  browsers, macOS asks whether trackd may control it.
- **Security:** the invoice preview and `tock report --open` wrote to the
  shared temp folder under a name anyone could guess, so on a Linux machine
  with other users a planted link could make them overwrite one of your
  files. They now write to `~/.local/state/tock/drafts` and `reports`.
- `tock invoice issue` refuses while `[invoice] name` is empty, and the
  preview says so: an issued invoice never changes, so one without your
  name could only be voided.
- `tock invoice check` shows minutes and seconds when a change is too
  small for hundredths of an hour, instead of two equal numbers.
- `tock project add` and `set` take `--rate`, `--client` and `--billable`,
  so billing no longer needs rules.toml edited by hand.
- `tock <command> --help` prints that command's lines and works before
  trackd has recorded anything; a mistyped flag shows the command's lines
  instead of all of the usage.
- Reading a day no longer slows down as history grows: it scanned every
  span before the day (8 ms after six months, 2 ms now).
- Two tock programs starting at once after an update could both try to
  update the database.

## v5.3.0 — 2026-11-06

### Added
- Edit the timeline from the dashboard. Click a bar or drag across the
  lanes to pick a range, then **Label**, **Add entry** or **Delete**; type
  in the sheet to change the times. **Add time** above the timeline adds
  time tock didn't see.
- **Export** in the week view saves the week report, the week's entries
  as CSV or JSON, or its hours by project: the same files `tock report`
  and `tock export` write. In the tock window a save sheet asks where each
  goes.
- `tock summary` without Claude prints the day's work blocks and says how
  to get a written summary, instead of stopping with an error.

### Changed
- Time without a label is called Unlabelled everywhere; the dashboard's
  tables said Unclassified.
- `tock export` errors now name the option at fault, the same way the
  dashboard does.

### Fixed
- Replacing a file from the dashboard's save sheet no longer deletes the
  old one before the new one is written, so a failed export keeps it. The
  toast says Saved only once the file is there, and nothing on Cancel.
- A slow save sheet can no longer cut a download short.
- An export range across a daylight saving change could cover a year and
  a day.

## v5.2.0 — 2026-11-05

### Added
- A new dashboard, in its own tock window from the menu bar (with a Dock
  icon while it's open). Today has a donut of the kinds of time, goal
  rings and minutes per hour next to the timeline; Week adds a heat map of
  when you focus and a projects donut; a new **Trends** view shows the
  month as a calendar, deep time per day and your best hours. ⌘K jumps to
  a view or a date, and every change can be undone from its toast.
- The open notch has pages: Today, This week and Goals, plus the session
  while one runs. Swipe sideways with two fingers, drag with the mouse, or
  use ← and → to move between them. Drag a bottom corner to make the
  panel bigger, like resizing a picture, or pinch it: it follows your
  fingers, gives less and less past the smallest and largest sizes, and
  settles on the nearest. Medium adds a chart per hour (or per day), large
  adds a second column. It remembers the size and the page.
- Install with Homebrew: `brew install confuserdotcom/tock/tock`, then
  `tock setup`. Homebrew builds tock on your Mac, so there is no
  "unidentified developer" warning, and `tock setup` makes and trusts the
  signing certificate, installs both apps and starts them. Run it again
  after `brew upgrade`; permissions stay granted. `tock setup --check`
  shows what is missing.
- A .dmg for Macs without Homebrew, for Apple Silicon and Intel. Drag tock
  to Applications and open it: it sets tock up the same way, and opened
  again it shows the dashboard. It isn't notarized, so the first open needs
  Open Anyway in System Settings (the README has the steps).
- `tock://dashboard` opens the dashboard.
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
