# tock

tock tracks where your time goes on a Mac. A small daemon, `trackd`, records
which app and window is in front, all day, into a local SQLite database.
The `tock` command and its terminal UI label that time with your own rules,
score your focus, run focus sessions and help you label what is left.

## Privacy

- Everything stays on your Mac, in `~/.local/share/tock/tock.db` (mode 0600).
- `trackd` contains no network code at all; a test fails the build if it ever
  links an HTTP or TLS client.
- Private-browsing windows are recorded as "<app> (private)", with no title or
  URL.
- Claude suggestions are the only feature that talks to the internet, and they
  are off unless `ANTHROPIC_API_KEY` is set in the shell that runs `tock`.
  - When on, at most once an hour, tock sends the grouped inbox keys: app name, URL host, cleaned title and minutes. It also sends your category and project names.
  - Password managers, mail and messaging apps, banks and webmail are never sent. Extend the deny lists in `config.toml [llm]`.
  - `tock suggest --dry-run` prints the exact payload without sending it.
  - The key is read from the environment only and is never written anywhere.
- `tock export` prints titles and URLs to stdout. Where that output goes is up to you.

## Requirements

- macOS (developed and tested on macOS 26)
- Go 1.26 and the Xcode command line tools (`xcode-select --install`), since
  `trackd` uses cgo

## Install

```sh
make cert          # once: creates the local code-signing identity
make cert-trust    # once: you approve a macOS dialog to trust it
make install       # builds, signs, installs, starts trackd, runs tock doctor
```

`make install` puts:
- `tock` in `~/.local/bin`, so add that directory to `PATH`;
- the signed `trackd.app` in `~/.local/share/tock`;
- a launchd agent, `com.tock.trackd`, that starts at login.

It also writes the starter `~/.config/tock/rules.toml` if you have none.

trackd asks for **Accessibility** (window titles) on first start. `tock doctor --open` opens the right System Settings pane if it is still missing. Browsers ask for **Automation** (URLs) the first time trackd reads them. Run `tock doctor` any time:

```
ok  rules: 22 rules, 0 learned
ok  config: defaults (no config.toml)
ok  trackd: running (version 3f2a1c0)
ok  accessibility: granted
ok  automation: no browser denied
ok  suggestions: off (set ANTHROPIC_API_KEY to enable)
```

Permissions are tied to the signing certificate, so they survive rebuilds.
`SIGN_MODE=adhoc make install` works without a certificate, but macOS then
forgets the grants on every rebuild.

## First day

1. Work normally. `tock` opens the TUI. `tock today` prints the day.
2. `tock inbox` lists time no rule labels yet, grouped by key. Label a key with
   `tock tag 1 Code/tock`. This also writes a rule to
   `~/.config/tock/learned.toml`, so the same activity is labelled from now on.
   In the TUI, use the inbox tab's `enter`.
3. Edit `~/.config/tock/rules.toml` for anything broader, then run
   `tock rules check`. You can also use the rules tab: `J`/`K` reorder, `e`
   opens the file in `$EDITOR`. A rule changes past days as well as future
   ones, except spans you tagged by hand.
4. Start a focus session with `tock session start 50` or the focus tab's `s`.
   trackd notifies you when you drift to a distraction and when a break is due.
5. `tock status` prints one line for a status bar, and `tock recap` prints the
   day's recap. trackd also shows the recap at 21:00.

## TUI keys

```
1-5 / tab        today · week · inbox · focus · rules
r                refresh now (also every 5 s)
?                today: score breakdown (? again for keys) · other tabs: keys
q / ctrl+c       quit

today            j/k move · g/G first/last · ctrl+d/u half page
                 h/l previous/next day · . today · / filter
                 t tag row · p project only · T tag and learn a rule
week             j/k day · h/l week · . this week · enter open day
inbox            enter learn a rule · a accept suggestion · o tag once · x ignore · u undo
focus            s start a session (presets or custom) · x stop
rules            j/k move · g/G first/last · J/K move rule down/up · e edit file
pickers          type a prefix · ↑/↓ · enter · esc
```

The mouse works too. Click a tab name to switch tabs, click a row to select
it, and click the selected row again for `enter` (open a day, label an inbox
key, pick an option, start a session). Click a key in the footer, such as
`s start`, to press it. The wheel moves the cursor.

While tock has the mouse, dragging no longer selects text. Hold Shift (WezTerm,
kitty, most terminals) or Option (Terminal.app, iTerm2) while you drag to
select text anyway.

## Commands

```
tock                                   open the TUI (today, week, inbox, focus, rules)
tock status                            one line for a status bar
tock init                              write starter ~/.config/tock/rules.toml
tock today [--date YYYY-MM-DD] [--keys]
tock inbox [--days N] [--all]          unlabelled activity, numbered
tock tag [--once] <n> <Category>[/<Project>]
tock tag --span <id> <Category>[/<Project>]
tock ignore <n> | tock ignore --undo <key>
tock rules check
tock session [start [MIN] | stop]      focus session: show, start (default 25), stop
tock recap [YYYY-MM-DD]                the daily recap
tock suggest [--dry-run | --force]     Claude label suggestions for the inbox (needs ANTHROPIC_API_KEY)
tock doctor [--wait] [--open]          check trackd, permissions, rules and config
tock export [--from D] [--to D] [--format csv|json]
                                       labelled spans, default the last 7 days as csv
```

Names match on a unique prefix: `tock tag 2 stu/atl`.

The export has one row per span:
`id,start,end,duration_s,app,bundle_id,title,url,private,category,kind,project,category_source,project_source`.
Times are RFC 3339 with your local offset.

## Files

| Path | What |
|------|------|
| `~/.config/tock/rules.toml` | your categories, projects and rules; first match wins |
| `~/.config/tock/learned.toml` | rules written by `tock tag`, checked before rules.toml |
| `~/.config/tock/config.toml` | optional settings, see below |
| `~/.local/share/tock/tock.db` | the spans (SQLite) |
| `~/.local/state/tock/trackd.log` | trackd's log |

`TOCK_CONFIG_DIR`, `TOCK_DATA_DIR` and `TOCK_STATE_DIR` override the three
directories.

`config.toml` sections. All keys are optional and the values below are the
defaults:

```toml
[focus]
presets = [25, 50, 90]   # session lengths in minutes; the first is the default
break_after = 50         # minutes of focus before a break reminder
break_repeat = 10        # minutes between reminders
distract_grace = 60      # seconds on a distraction before a nudge
distract_repeat = 120    # seconds between nudges
auto_min = 25            # minutes of deep work that count as an unplanned session
recap = "21:00"          # daily recap notification, "" to disable
notify = true
# meeting_apps / meeting_hosts replace the built-in lists of meeting apps
# (bundle ids) and hosts during which break and distraction nudges stay quiet

[llm]
enabled = true                # still needs ANTHROPIC_API_KEY
model = "claude-haiku-5-5"
min_minutes = 2               # inbox time a key needs before it is sent
deny_apps = []                # bundle ids added to the built-in deny list
deny_hosts = []               # host regexes added to the built-in list
deny_titles = []              # title regexes

[score]                       # weights of the focus score
depth = 0.5
blocks = 0.3
calm = 0.2
```

## Uninstall

`make uninstall` stops trackd and removes its launchd agent. Your data stays.
To delete the data too, remove `~/.local/share/tock`, `~/.local/state/tock`
and `~/.config/tock`.

## Development

```sh
make test                                  # vet, build, all tests
go test ./internal/tui -run Golden -update # regenerate TUI snapshots
make dump                                  # print trackd's live samples
```

Design notes are in `docs/superpowers/specs`. Checklists for things only a
person at the Mac can verify are in `docs/*-checklist.md`.
