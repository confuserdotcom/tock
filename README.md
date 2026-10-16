# tock

tock tracks where your time goes on a Mac, and on Linux under Hyprland. A
small daemon, `trackd`, records
which app and window is in front, all day, into a local SQLite database.
The `tock` command and its terminal UI label that time with your own rules,
score your focus, run focus sessions and help you label what is left.

## Privacy

- Everything stays on your Mac, in `~/.local/share/tock/tock.db` (mode 0600).
- `trackd` contains no network code at all; a test fails the build if it ever
  links an HTTP or TLS client.
- Private-browsing windows are recorded as "<app> (private)", with no title or
  URL.
- Claude suggestions, summaries and `tock ask` are the only features that talk
  to the internet, and they are off unless `ANTHROPIC_API_KEY` is set in the
  shell that runs `tock`.
  - When on, at most once an hour, tock sends the grouped inbox keys: app name, URL host, cleaned title and minutes. It also sends your category and project names.
  - Password managers, mail and messaging apps, banks and webmail are never sent. Extend the deny lists in `config.toml [llm]`.
  - `tock summary` runs only when you type it. It sends the day's work blocks: the weekday (never the date), clock times, minutes, labels and, per block, at most 8 apps with their URL host or cleaned title, through the same deny lists. The answer is saved in tock.db.
  - `tock ask` sends only your question, today's date and your category and project names. Claude answers with a query, and tock runs it on this machine: no activity is sent.
  - `--dry-run` on `tock suggest`, `tock summary` and `tock ask` prints the exact payload without sending it.
  - The key is read from the environment only and is never written anywhere.
- Slack status (opt-in) is the only other network use: `tock slack`, run by
  trackd when a session starts and ends, sets your status and
  do-not-disturb. Nothing about your activity is sent, only the status text
  you chose and the session's end time.
- `tock export` prints titles and URLs to stdout. Where that output goes is up to you.
- `[privacy] keep_titles = 30` deletes window titles, and cuts URLs to their
  host, once they are 30 days old (see Deleting old titles).
- Calendar events (opt-in) are read through macOS Calendar, never fetched, and
  stay in tock.db. They are never sent to Claude.
- Commit counts come from `git log` on your local repos: only author times
  are read, never messages or code.
- Sync between machines (opt-in) only writes and reads files in a folder you
  choose; moving that folder between machines is up to your own sync tool.
- `tock web` listens on 127.0.0.1 only, while it runs. Each run makes a new
  secret token that the page needs for every request. The page loads nothing
  from the internet.

## Requirements

- macOS (developed and tested on macOS 26), or Linux with Hyprland (see
  [Linux](#linux-hyprland))
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
- a launchd agent, `com.tock.trackd`, that starts at login;
- the menu bar app `tock.app` and its agent `com.tock.bar` (see
  [Menu bar and notifications](#menu-bar-and-notifications)).

It also writes the starter `~/.config/tock/rules.toml` if you have none.

trackd asks for **Accessibility** (window titles) on first start. `tock doctor --open` opens the right System Settings pane if it is still missing. Browsers ask for **Automation** (URLs) the first time trackd reads them. Run `tock doctor` any time:

```
ok  rules: 22 rules, 0 learned
ok  config: defaults (no config.toml)
ok  trackd: running (version 3f2a1c0)
ok  accessibility: granted
ok  notifications: native, with buttons (menu bar)
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
   When a key looks like time you already labelled (the same site or app,
   similar titles, or between two spans with one label), the inbox shows a
   guess such as `→ Code (same app, 82%)`. `tock tag 1` with no label
   accepts it. Guesses are computed on your machine; nothing is sent.
   Tagged it wrong? Tag it again (or `t` on the row in today): the new rule
   replaces the mistaken learned one, so every span it labelled changes.
3. Edit `~/.config/tock/rules.toml` for anything broader, then run
   `tock rules check`. You can also use the rules tab: `J`/`K` reorder, `e`
   opens the file in `$EDITOR`. A rule changes past days as well as future
   ones, except spans you tagged by hand.
4. Start a focus session with `tock session start 50` or the focus tab's `s`.
   trackd notifies you when you drift to a distraction and when a break is due.
5. `tock status` prints one line for a status bar, and `tock recap` prints the
   day's recap. trackd also shows the recap at 21:00 (or when your work
   hours end, see Work hours).

## TUI keys

You don't need to learn the keys:

- The footer shows only the keys that work on the selected row.
- `?` lists every key, by tab.
- `:` (or `ctrl+k`) opens the command palette. Type a few letters of what
  you want, such as `week`, `50 min` or `edit rules`, and press enter.

```
1-7 / tab        today · week · inbox · focus · rules · plan · labels
: / ctrl+k       command palette
?                all keys
r · q            refresh · quit

today            enter label the row (accepts a ★ suggestion, or labels time away)
                 t tag · p project only · T tag and learn a rule · i why this score
                 a label the latest time away · A leave it idle
                 s split (label part of the row) · d delete its time · + add time
                 j/k move · h/l previous/next day · . today · / filter
                 c every window ↔ compact rows (labelled rows in a row merge)
week             enter open day · h/l week · . this week · m month trends
inbox            enter accept the suggestion, else learn a rule · T pick a label
                 o tag once · x ignore
anywhere         u undo the last change (tag, rule, ignore, time away, edit)
focus            s start a session · x stop
rules            e edit rules.toml · J/K move a rule down/up
plan             enter/space/x done ↔ not done · a add · d delete · e edit the file
                 f focus session · u undo the tab's last edit
labels           a add a category · m add a module under it · e rename · K kind
                 w afk window · f file a project under a category · l other names
                 x archive · M merge into another · d delete · enter fold
pickers          ★ suggested and ↺ recent labels come first · type to filter · enter · esc
```

Colours follow the kind of time: teal deep, sand shallow, slate neutral,
apricot browsing, coral distraction, violet unlabelled. The palette has light and dark
variants and follows your terminal's background.

The first time you run `tock` without rules, it offers to write the starter
rules.

The mouse works everywhere. Anything that shows a key is a button: the
footer, the help, a dialog's `enter ok`, the `a label` of time away, the
`u undo` after a change. Click a tab to switch, a row to select it and the
selected row again for `enter`. Right-click a row for a menu of what you
can do with it. Hovering shows what a thing is in the footer.

On the today tab, the strip under the score is the day: one cell per slice
of time, coloured by its kind (faint dots are idle). Click a cell, a work
block or an hour of the heatmap to jump to that time; click a category or
a module in Where it went to show only its rows, and again to show all; the
score opens its breakdown. A terminal 130 columns wide or more puts Where
it went, with every module, the goals and the work blocks in a sidebar.

The labels tab (`7`) is where your categories and modules live: each with
its kind, AFK window and 30 days of time. Click a kind or an AFK window to
change it. The label picker (`t`) offers a module as you type its name or
an alias: `ana` → `Study / Analyse 1`.

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
tock tag [--once] <n> [<Category>[/<Project>]]   no label: accept the → suggestion
tock tag --span <id> <Category>[/<Project>]
tock ignore <n> | tock ignore --undo <key>
tock away [--date D] [<HH:MM> <Category>[/<Project>] | <HH:MM> same | --skip <HH:MM>]
                                       label time away (idle or asleep) or leave it idle
tock edit add <HH:MM-HH:MM> <Category>[/<Project>] [note]
                                       add time you forgot to track
tock edit label <HH:MM-HH:MM> <Category>[/<Project>]
                                       label part of a span (a split) or several at once (a merge)
tock edit delete <HH:MM-HH:MM>         remove time, or trim a span's start or end
tock edit [--date D] | tock edit remove <id>   list the day's edits | drop one
tock undo [--list]                     reverse the last change
tock rules check | tock rules stats [--days N]   validate | time each rule decided
tock rules dupes                       look-alike category and project names
tock rules merge [--project] <from> <into>
                                       fold one name into another everywhere
tock session [start [MIN] | stop]      focus session: show, start (default 25), stop
tock recap [YYYY-MM-DD]                the daily recap
tock trends [--month YYYY-MM]          a month against the one before
tock top [--days N | --week | --month] [--by category|app|site|project] [--limit N]
                                       where the time went: top categories, apps and sites
tock web [--port N] [--no-open]       the dashboard in your browser, on this machine only
tock review [--week YYYY-MM-DD] [--plan TEXT]
                                       weekly recap, then next week's focus
tock suggest [--dry-run | --force]     Claude label suggestions for the inbox (needs ANTHROPIC_API_KEY)
tock blocks [--date D]                 the day's work blocks: stretches of work on one thing
tock summary [--date D] [--dry-run]    Claude writes what you worked on (needs ANTHROPIC_API_KEY)
tock ask [--dry-run] "<question>"      ask about your time in plain words; answered on this machine
tock doctor [--wait] [--open]          check trackd, permissions, rules and config
tock export [--from D] [--to D] [--format csv|json] [--by project]
                                       labelled spans, default the last 7 days as csv
tock backup [--out FILE]               archive tock.db and the rule, config and shell files
tock slack [login | logout | status]   Slack status during focus sessions (see Focus guard)
tock shell-init zsh                    shell hook for terminal projects (docs/shell-hook.md)
```

Names match on a unique prefix: `tock tag 2 stu/atl`.

The export has one row per span:
`id,start,end,duration_s,app,bundle_id,title,url,private,category,kind,project,category_source,project_source,rate,amount`.
Times are RFC 3339 with your local offset. `rate` and `amount` are empty
(`null` in JSON) unless the row's project has a rate (see Billing).

## Files

| Path | What |
|------|------|
| `~/.config/tock/rules.toml` | your categories, projects and rules; first match wins |
| `~/.config/tock/learned.toml` | rules written by `tock tag`, checked before rules.toml |
| `~/.config/tock/config.toml` | optional settings, see below |
| `~/.config/tock/slack.token` | the Slack token, when you use Slack |
| `~/.local/share/tock/tock.db` | the spans (SQLite) |
| `~/.local/state/tock/trackd.log` | trackd's log |
| `~/.local/state/tock/shell.log` | working directories from the shell hook, see [docs/shell-hook.md](docs/shell-hook.md) |

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
block = false            # close distracting tabs and hide distracting apps in sessions
block_apps = []          # bundle ids (Hyprland classes) blocked besides distractions
block_hosts = []         # sites blocked besides distractions; subdomains too
block_grace = 10         # seconds a distraction may stay in front
focus_on = ""            # Shortcuts shortcut run when a session starts
focus_off = ""           # ... and when it ends

[hours]                  # off unless set; see Work hours
days = ["mon", "tue", "wed", "thu", "fri"]
start = "09:00"          # start and end are both required
end = "18:00"

[privacy]
keep_titles = 30         # off unless set: days window titles are kept

[invoice]                # for tock invoice; all optional
name = "Your Name"
address = ["1 Main Street", "City"]
currency = "EUR"         # printed after amounts; "" by default
note = "Payment within 30 days."

[slack]                  # see Focus guard
enabled = false
status = "In a focus session"
emoji = ":dart:"
dnd = true               # pause Slack notifications for the session

[llm]
enabled = true                # still needs ANTHROPIC_API_KEY
model = "claude-haiku-5-5"
min_minutes = 2               # inbox time a key needs before it is sent
deny_apps = []                # bundle ids added to the built-in deny list
deny_hosts = []               # host regexes added to the built-in list
deny_titles = []              # title regexes

[shell]
terminals = [...]             # bundle ids that get shell dirs; replaces the built-in list

[away]                        # which absences tock asks about
min = 10                      # minutes; at least 5
max = 240                     # minutes; at most 720

[afk]                         # when time in front of an app stops counting, see AFK
video = 180                   # minutes without input while a video plays; trackd reads it at start
stale = 20                    # minutes; see "Machines that report no input"

[calendar]                    # meetings from macOS Calendar, see Calendar
enabled = false               # true: trackd asks for calendar access once
calendars = []                # calendar names to read; empty = all

[score]                       # weights of the focus score; they sum to 1
depth = 0.5
blocks = 0.3
calm = 0.2
stretch = 0.0                 # longest run without a context switch, full at 1h30
```

## Plan

The plan tab (`6`, or `tock plan`) shows a todo.txt file by day: Overdue,
Today, Tomorrow, Later, No date, and what you ticked today. Inside a day,
tasks go by the time in a context (`@07h30`, `@mon-07h30`, `@07:30`).
Ticking writes `x DATE` and keeps the priority as `pri:A`; unticking puts it
back. A future `t:YYYY-MM-DD` hides a task until that day. Piped, `tock
plan` prints the open tasks.

```toml
[plan]
file = "~/todo.txt"
# optional: run when the tab opens, after each change and once a minute
command = "my-planner --tock"
```

The command gets `TOCK_PLAN_FILE` and may rewrite the file (a planner that
rolls missed tasks over, imports ticks). Its output shows above the list:
plain lines, or JSON `{"lines": ["…"], "seen": {"project": seconds}}`,
where `seen` puts the time you spent on a `+project` next to its first open
task today. Edits re-read the file and find the line by its text, so a
command rewriting the file never makes tock tick the wrong task.

## Focus guard

A focus session can do more than nudge you.

**Blocking.** With `block = true` under `[focus]`, a distraction that stays
in front for 10 seconds (`block_grace`) during a session is taken away:

- in Chrome, Arc, Brave and Safari, the front tab is closed, if it still
  shows that site;
- any other app (and Firefox) is hidden. On Hyprland its windows move to
  the special workspace `tock` (`hyprctl dispatch togglespecialworkspace
  tock` brings them back).

A distraction is time whose category has `kind = "distract"`, plus the apps
in `block_apps` and the sites in `block_hosts`. Nothing is blocked during a
call or a meeting, and Finder and tock never are. A notification names the
app, never the site. Closing a tab uses the same Automation permission trackd
already has for reading URLs.

**Focus mode.** macOS has no command for Focus, but Shortcuts does. In the
Shortcuts app, make a shortcut named `Focus On` with the action *Set Focus*
(Do Not Disturb, or your Work focus, *On*), and a `Focus Off` shortcut that
turns it off. Then:

```toml
[focus]
focus_on = "Focus On"
focus_off = "Focus Off"
```

trackd runs them when a session starts and when it ends, whether you
started it in the TUI, the menu bar or `tock session`. `tock doctor` checks
that both shortcuts exist.

**Slack.** tock can set your Slack status and pause Slack notifications
for the session:

1. Create a Slack app for your workspace at api.slack.com/apps, add the
   user token scopes `users.profile:read`, `users.profile:write` and
   `dnd:write`, install it and copy the user token (`xoxp-…`).
2. `tock slack login` and paste it. It is stored in
   `~/.config/tock/slack.token` (owner-only); `TOCK_SLACK_TOKEN` overrides
   it.
3. `[slack] enabled = true` in config.toml. `tock slack` checks the token.

The status expires on its own at the planned end. When the session ends,
tock clears it only if it is still tock's status, so a status you set
yourself in the meantime stays. trackd never talks to the network itself: it
runs `tock slack start` and `tock slack stop`.

## Goals

`[[goal]]` tables in `rules.toml` set goals (`at_least`) and limits
(`at_most`) per day or week, for a category, a project or both:

```toml
[[goal]]
category = "Study"
at_least = "2h"
per = "day"            # the default; or "week" (Monday to Sunday)

[[goal]]
project = "tock"
at_least = "10h"
per = "week"

[[goal]]
category = "Entertainment"
at_most = "45m"
```

The today tab and `tock today` show a goals line, the week tab shows each
goal over the week (`met 3 of 5 days` for a day goal), and `tock status`
adds `goals 2/3` plus `over Entertainment` when a limit is passed.
`tock rules check` lists the goals.

You don't have to edit the file. `tock goal set ana1 6h/week` sets a goal on
a module (by name or other name), `tock goal set Entertainment 45m --max`
sets a limit, `tock goal rm ana1 week` removes one, and `tock goal` lists
them with this week's progress. On the labels tab, `o` (or a click on a row's
goal) sets a module's weekly goal or a category's daily one. The week tab
has a **Modules** bar per module, filled out of its weekly goal
("1h40 of 6h00 · 4h20 to go"); click one to change its goal.

Each goal keeps a streak: days in a row it held (met, or under a limit),
or weeks for a week goal. Days with no activity and days off (see Work
hours) neither count nor break it. Today counts once a goal is met; a limit
counts once the day's hours are over, and breaks the streak as soon as it
is passed. The goals line shows `· 5d` from two days on, the week tab and
`tock review` add `streak 5d (best 12d)`, and the dashboard shows
`5d streak`.

## Work hours

With `[hours]` in `config.toml`, the focus score only looks at your work
hours:

```toml
[hours]
days = ["mon", "tue", "wed", "thu", "fri"]   # the default
start = "09:00"
end = "18:00"
```

trackd still records everything, and totals, categories and goals still
cover the whole day; only the score, the focus numbers and the adaptive
target are limited to the hours. A day not in `days` is a day off: it is
not scored and the target skips it. `tock today` adds `09:00–18:00` and
`1h20 outside hours` to the score line (or says `day off · not scored`),
and the today tab and dashboard show the same.

The recap closes the day: it goes out when the hours end, unless
`[focus] recap` sets its own time, and is skipped on days off.

## Time away

An absence is idle time, or time with no record at all (asleep, locked),
between two stretches of activity: 10 minutes to 4 hours by default
(`[away]` in config.toml). When you come back, trackd sends one
notification. Label the absence with `a` on the today tab, `t` on its idle
row, or `tock away 12:10 Meeting`; `A` or `tock away --skip 12:10` leaves it
idle. When the time before the absence had a label (often reading with no
input past its AFK window), tock asks whether you were still on it: `y`, the
notification's first button or `tock away 12:10 same` gives the absence that
label. A labelled absence counts towards totals and goals, but not towards
focus blocks, context switches or break reminders. Absences that cross
midnight are never asked about.

## Categories

Categories are yours: `rules.toml` declares them, and `tock category`
changes them without opening the file.

```
tock category                              each category, its projects and 30 days of time
tock category add Podcasts neutral --afk 30m
tock category set Reading --kind deep      or --afk 15m, --afk none
tock category rename Communication Messages
tock category rm Podcasts                  only when nothing names it
```

A rename reaches everything: both rule files, goals, `[calls]`, projects
filed under the category, and in tock.db the time you tagged by hand, away
labels and timeline edits. To fold one category into another, use `tock
rules merge`. Every change keeps the previous file as `rules.toml.bak`.

Each category has a kind:

| kind | what it is | counts as |
|------|------------|-----------|
| `deep` | focused work | focus blocks and the score's depth |
| `shallow` | email, planning, looking things up | work, not focus |
| `neutral` | utilities, reading for pleasure | neither |
| `browse` | search, feeds, shopping | a third of it weighs on the score like a distraction; never nudged or blocked |
| `distract` | entertainment | the score's penalty, nudges and blocking in sessions |

The starter rules have Reading (neutral, for books and long reads), Research
(shallow: encyclopedias, papers, dictionaries) and Browsing.

## Modules

A project filed under a category is a module: a course, a client, a side
project, whatever groups your time inside that category.

```toml
[[project]]
name = "Analyse 1"
category = "Study"
aliases = ["ana1", "Nombres réels", "Suites numériques"]
```

- tock finds a module by its name and aliases in window titles and terminal
  directories, whatever the app: `Cours Complet ALGEBRE 1.pdf` is Algèbre
  1 and `~/Licence/S1/Analyse 1` in a terminal is Analyse 1. Case and
  accents don't matter, and words match whole (`Analyse 1` never matches
  `Analyse 12`). This comes before every rule; `match = false` turns it off
  for one module, and a hand tag still wins.
- Time with the module and no category of its own takes the module's
  category, so `p` (project only) on a row also makes it Study.
- With a `[plan]` file, a timed task whose `+tag` names a module (`+ana1
  @ven-19h30 due:2026-10-16`) claims that module for Study time without
  one, from its time to the next timed task that day (3 hours at most).
  Notes titled `TD 1` then still land in the right module.

```
tock project scan ~/Licence/S1 --category Study   a module for each folder
tock project add Thermodynamique --category Study --alias thermo
tock project set ana --alias ana1,AF               --category none unfiles it
tock project rename "Analyse 1" "Analyse I"
tock project refile Licence                        hand tags of a catch-all go to the module in their title
tock project                                       categories with their modules
```

A module named in a window title also wins over the label another machine
synced for it, so an older machine with a catch-all project can't undo
your modules.

Projects without a category work as before: rules set them, and they can
list `repos` and a `rate`.

## AFK

Time counts while you are at the machine. trackd notices when there has
been no key or mouse input for 5 minutes, and the time since your last
input becomes idle. Some things need longer: give a category its own
window in `rules.toml`:

```toml
[[category]]
name = "Reading"
kind = "deep"
afk = "10m"          # 5m to 2h; the default is 5m
```

A pause shorter than the window of what was in front stays with it: eight
minutes on one page of a book is still Reading. A longer one is AFK, and all
of it, back to your last input, is idle. This is decided when tock reads the
day, so changing `afk` changes past days too.

While a video plays in the front app (it keeps the display awake), trackd
waits `[afk] video` minutes (3 hours by default) before it goes idle. Music
does not count. During a call you are never AFK (see Calls).

**Machines that report no input.** On Hyprland trackd only learns about
input through the hypridle hook (see Linux). Without it, and for another
machine whose sync folder doesn't say it reports input (an older tock),
tock guesses: one window left unchanged for longer than `[afk] stale`
minutes (20) keeps its category's window and the rest is idle. A page turn
in a PDF reader changes the title, so reading still counts. `tock doctor`
names those machines.

## Editing the timeline

What trackd recorded never changes; edits sit on top of it and every view
uses them (today, the week, the score, goals, the inbox, exports).

- **Add** time you forgot to track, such as a meeting away from the Mac:
  `tock edit add 14:00-15:00 Meeting/ATLAS standup`. The entry replaces
  whatever was recorded in that range and shows as `Entry`.
- **Label** part of the timeline: `tock edit label 11:14-11:20 Study`. The
  apps keep their names; idle time and gaps inside become entries. Labelling
  the end of a span is a split, and labelling several spans at once is a
  merge.
- **Delete** time: `tock edit delete 09:00-09:15`. Deleting the first or last
  minutes of a span trims it.

`--date YYYY-MM-DD` edits another day, and the end may be `now`. `tock edit`
lists the day's edits and `tock edit remove <id>` drops one. When edits
overlap, the newest wins. In the TUI's today tab, `s` splits the row (type
where the labelled part starts and ends), `d` deletes the row's time and `+`
adds time (an idle row's range is filled in); `t` on an entry relabels it.
In `tock web`, a row's sheet has **Delete time**.

## Undo

`tock undo` reverses the last change: a tag, a learned rule (learned.toml
goes back as it was; tock refuses if you edited the file since), an ignore,
a label for time away, or a timeline edit. Run it again to go further back;
`tock undo --list` shows the last 20 changes. `u` does the same in the TUI
and in `tock web`. tock keeps the last 200 changes. `tock rules merge`
is not in the list: it keeps `rules.toml.bak` instead.

## Work blocks, summaries and questions

A work block is a stretch of work on one thing: a project, else a category.
A break of 10 min ends a block, and so does other work that lasts 10 min;
shorter detours stay inside the block around them. Blocks under 15 min are
not listed. `tock blocks` lists the day's blocks with their focus share and
top apps; the TUI's today tab shows them on the `work` line and `tock web`
in the **Work blocks** card.

`tock summary` asks Claude what the day was about and saves one line for the
day and one per block. Every view shows them from then on (the `✎` line in
the TUI). Run it again to replace them; `--date` summarizes another day.

`tock ask "how much video last week?"` answers questions about your time:
totals by category, project, app, site, kind or day, over a range. Claude
only turns the question into a query; tock runs it on your data.

Both need `ANTHROPIC_API_KEY`, and `[llm] enabled = false` turns them off; see
Privacy for what they send.

## Trends and review

`tock trends` (and `m` on the week tab) compares a month with the previous
one: the whole month for a past month, the same days so far for the
current one. It shows tracked and focus time, the average workday score
with a daily sparkline, switches per hour and each category's change.

`tock review` recaps a week (this week on a weekend, else last week):
totals, each goal, the top distractions and the best day. It then asks
for next week's focus, or takes `--plan`. The week tab shows the plan of
the week on screen. Plans stay in tock.db.

`tock top` ranks the last 7 days (or `--week`, `--month`, `--days N`):
categories with their kind, apps and sites (URL hosts) with the category
most of their time went to, each with its share of tracked time. `--by
project` lists projects.

## Web dashboard

`tock web` opens a dashboard in your browser and runs until you press
ctrl-c:

- **Today:** the focus score and its four parts, a timeline ribbon (hover a
  block for its app and title), time by hour, categories, goals and the
  day's activity. Click any block or row to label it. Use the arrows or
  ← → to move between days.
- **Week:** each day's time by kind and score (click a day to open it),
  categories, projects, apps and sites, distractions, goals and the plan.
- **Top:** `tock top` for today, 7 days, this week, this month or 30 days.
- **Inbox:** unlabelled activity with the local guess or Claude's
  suggestion. **Accept** learns the suggestion's broadest safe rule, as
  `tock tag <n>` does. **Label…** picks a category and project, then
  labels just these spans or previews the rules to learn. **Ignore** has
  an undo. Keys: `j`/`k` move, `a` accept, `l` label, `i` ignore.

Keys `1`–`4` switch views, `r` refreshes and `u` undoes the last change. Today refreshes itself every
minute.

The server listens on 127.0.0.1 only, on a free port (`--port N` to choose
one). The link it prints carries a random token after the `#`. Browsers
never send that part to a server, and the page keeps it for the tab only.
Every request needs the token, so other sites open in your browser can
neither read nor label anything, and requests that name another host are
refused. The page, its styles and scripts are built into `tock` and load
nothing from the internet. trackd is not involved. `--no-open` prints the
link without opening a browser.

For working on the page, `TOCK_DEMO_PORT=7788 go test -tags demo -run
TestDemo -timeout 0 ./internal/web` serves synthetic data at
`http://127.0.0.1:7788/#` followed by 64 zeros.

## Duplicate names

`tock rules dupes` lists categories, and projects, whose names differ only
by case, punctuation, `&` for "and", a plural ending or a typo, with the
time each labelled in the last 30 days. `tock doctor` mentions them.
`tock rules merge Studies Study` (or `--project`) removes the first name's
table from rules.toml, renames it in rules.toml and learned.toml, and
renames manual tags, away labels and suggestions in tock.db. The old
rules.toml is kept as rules.toml.bak. A project with repos or a rate must
have them moved by hand first.

## Calls

trackd notices when a camera or microphone is in use, in any app. It only
reads whether a device is running, so it needs no camera or microphone
permission and records no audio or video, just start and end times. During
a call, break and distraction nudges stay quiet. With this table in
`rules.toml` (the starter rules have it), calls of at least two minutes are
labelled, including time you only listened and would otherwise be idle:

```toml
[calls]
category = "Meeting"   # a declared category
```

A manual tag still wins. `tock rules check` says whether calls are labelled.

## Commits

List a project's local git repositories in `rules.toml` and tock counts
your commits beside its time:

```toml
[[project]]
name = "tock"
repos = ["~/tock"]
```

The today tab and `tock today` show `commits  tock 4 · ATLAS 1`, and the
week tab and `tock review` show the week's commits. tock runs
`git log --all --no-merges` and counts only commits by the repo's
`user.email`, including ones fetched from another machine. It reads
author times only and never fetches. `tock doctor` checks every repo.

## Billing

Give a project an hourly rate and `tock export` bills its time:

```toml
[[project]]
name = "ATLAS"
rate = 80
```

Each span row gets `rate` and `amount` (rate × hours, to the cent).
`tock export --by project --from 2026-10-01 --to 2026-10-31` writes one row
per project instead: `project,duration_s,hours,rate,amount`, with the
amount computed from the total time. Time without a project is left out.
`tock export` has no currency: amounts are in the rate's unit.

`tock invoice --project ATLAS` writes `invoice-202609-atlas.pdf` for last
month: one line per day with hours (two decimals), rate and amount, then the
totals. `--from` and `--to` pick other days, `--client` adds who it is to,
`--number` sets the number (default `YYYYMM-project`) and `-o` the file.
Your name, address, currency and a closing note come from `[invoice]` in
`config.toml`. The PDF is plain A4 in Helvetica; characters outside Western
European alphabets print as `?`.

## Week report

`tock report` writes a week as one HTML page to stdout: tracked time,
average score, each day with its score, focus and a bar by kind,
categories, projects, apps and sites, distractions, goals with streaks, the
plan and commits. It defaults to the week `tock review` shows; `--week D`
picks another. `-o week.html` writes a file and `--open` opens it in the
browser. The page has no scripts and loads nothing, so it can be mailed or
archived as it is; window titles never appear in it.

## Deleting old titles

```toml
[privacy]
keep_titles = 30
```

Once an hour trackd deletes the window titles of activity older than 30
days and cuts its URLs to the host (`mail.google.com/mail/u/0/#inbox`
becomes `mail.google.com`). Each span keeps the category and project its
rules gave it, so no report changes; a later rule on the app or the URL
host still fills a label it had left empty. Other machines' activity and
this machine's own files in the sync folder older than that are cleaned
too. `tock prune --dry-run` counts what would go, `tock prune` does it now
and `tock prune --days 7` uses another age once. Saved Claude summaries
(`tock summary`) are kept. Deleted titles cannot be brought back, except
from a backup made before.

## Calendar

Calendar events can label the time you spend away from the Mac in a
meeting. tock reads them through macOS Calendar, so a Google Calendar
works once its account is added under System Settings › Internet Accounts
(Calendars on). No Google credential ever reaches tock.

1. In `config.toml`: `[calendar]` `enabled = true` (and optionally
   `calendars = ["Work"]`).
2. In `rules.toml`:

   ```toml
   [[category]]
   name = "Meetings"
   kind = "shallow"

   [calendar]
   category = "Meetings"
   ```

3. `make install` (or restart trackd) and allow the calendar prompt.
   `tock doctor` shows the state and the last sync.

trackd copies timed events from two days back to one day ahead every 10
minutes, skipping all-day, free and declined ones. An event only labels
time when you were not at the Mac: idle time and gaps inside it become
`Meeting` rows titled with the event. What you did at the Mac during an
event keeps its own label. Rules can set a meeting's project:

```toml
[[rule]]
bundle = ["tock.meeting"]
title = '^ATLAS'
project = "ATLAS"
```

`t` on a meeting row relabels it. Answered time away wins over an event,
and tock does not ask about time an event explains.

## Menu bar and notifications

`make install` also installs `tock.app`, a small menu bar app that runs at
login. The first time it starts, macOS asks whether **tock** may send
notifications; allow it.

The menu bar shows today's tracked time (`◷ 2h54`), or the countdown while
a focus session runs (`◉ 18:42`). A `!` means trackd is not running. The
menu has:

- the day at a glance: focus, score and unlabelled time;
- items to start a session (your presets) or stop the running one.

With the app running and allowed to notify, focus notifications come from
tock with buttons:

| Notification | Buttons |
|---|---|
| distracted in a session | Stop session |
| session done | Start again |
| break due | Snooze 30 min |
| time away | your recent away labels, then Leave idle |

Without the app, or with its notifications turned off, trackd sends plain
notifications as before. `tock doctor` says which is in use. **Quit tock
menu bar** keeps it closed until the next login; `make install` starts it
again. Its log is `~/.local/state/tock/tockbar.log`, and it never logs
notification text.

## Linux (Hyprland)

On Linux, trackd reads the focused window from Hyprland's sockets. It needs
no cgo, no bundle and no permissions.

```sh
make install-linux   # trackd and tock in ~/.local/bin, trackd.service enabled
```

No Go on the Linux machine? Build and install it from the Mac over ssh
instead; this also updates it later:

```sh
make deploy-linux                      # LINUX_HOST=ryuk, LINUX_ARCH=amd64
make deploy-linux LINUX_HOST=box LINUX_ARCH=arm64
```

trackd runs as the systemd user unit `trackd.service` and starts with your
graphical session. Its log is `~/.local/state/tock/trackd.log`, and
`systemctl --user status trackd` shows the unit.

Wayland gives no app the idle time, so hypridle tells trackd. Add this to
hypridle's config, keeping your other listeners. That is
`~/.config/hypr/hypridle.conf`, or the file hypridle runs with when it is
started with `-c` (some shells do this); `tock doctor` checks the one that
is running:

```
general {
    before_sleep_cmd = trackd idle
    after_sleep_cmd = trackd active
}

listener {
    timeout = 300
    on-timeout = trackd idle --after 300
    on-resume = trackd active
}
```

Keep `--after` equal to the timeout. While `hyprlock` (or `swaylock`) runs,
the session counts as locked.

When your desktop writes hypridle's config itself (some Hyprland shells
render it from their own settings and restart hypridle), an edit there does
not last. Run a second hypridle just for trackd instead, under another name
so the desktop leaves it alone:

```sh
ln -s /usr/bin/hypridle ~/.local/bin/tock-hypridle
# ~/.config/tock/hypridle.conf: the general and listener blocks above, and in
# general: ignore_systemd_inhibit = true and ignore_dbus_inhibit = true, so a
# caffeine mode cannot hide that you left (a video's Wayland inhibitor still
# counts as watching)
cat > ~/.config/systemd/user/tock-idle.service <<'EOF'
[Unit]
Description=tock: tell trackd when you are idle (a hypridle of its own)
PartOf=graphical-session.target
After=graphical-session.target
ConditionEnvironment=WAYLAND_DISPLAY

[Service]
ExecStart=%h/.local/bin/tock-hypridle -c %h/.config/tock/hypridle.conf
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical-session.target
EOF
systemctl --user daemon-reload && systemctl --user enable --now tock-idle
```

`tock doctor` finds the hook in the config of any running hypridle,
renamed or not.

An always-on idle inhibitor (a "caffeine" or keep-awake mode) also stops a
shared hypridle from ever reporting you idle; a separate one for trackd with
those two lines does not have that problem.

- **Rules.** On Linux, a window's bundle id and app name are its Hyprland
  class, such as `kitty` or `firefox`. `hyprctl clients` lists the classes.
  Write rules with `app = "firefox"`, or with titles, as on the Mac.
- **Terminals.** The zsh hook works the same. kitty, WezTerm, Alacritty,
  foot and ghostty are known terminals.
- **Notifications** go through `notify-send`.
- **Not on Linux:** browser URLs, the calendar, call detection and the menu
  bar app.

## Sync between machines

tock can merge the time you spend on two machines, for example the Mac and
a Linux desktop, into one view on each. tock does not move any data itself.
Pick a folder that your own tool keeps in sync, such as Syncthing or a
network share, and name it on both machines in `config.toml`:

```toml
[sync]
dir = "~/Sync/tock"
machine = "mac"        # optional; default: the host name
rules = true           # optional; share rules.toml (see Rules below)
```

Every minute, trackd writes this machine's last two days to
`<dir>/<machine>/<YYYY-MM-DD>.jsonl`, and reads the other machines' files.

- **Labels.** Each machine labels its own time with its own rules. The other
  machine uses those labels when it has a category and project of the same
  name; otherwise its own rules apply.
- **Merging.** Another machine's time shows in the timeline as
  `ryuk · kitty`. It only fills time when this machine was idle or off, so
  local activity always wins. Totals, the score, goals and the week include
  it.
- **Relabelling.** To relabel another machine's time, do it on that machine.
  Its time never enters this machine's inbox or Claude suggestions.
- **Rules.** Each machine also shares its `rules.toml`. When another
  machine's copy changed later than yours, has fully arrived and parses with
  your `learned.toml`, tock takes it and keeps yours as `rules.toml.bak`.
  `learned.toml` stays per machine. Clocks decide which edit is newer, so
  keep them in time. `rules = false` under `[sync]` turns this off.
- **Commands.** `tock sync` syncs now and lists the machines. Use
  `tock sync --days 30` once to send older days. `tock doctor` shows a
  `sync` line.

The day files hold titles and URLs (never those of private windows). Keep
the folder private.

## Backup

`tock backup` writes `~/.local/share/tock/backups/tock-YYYYMMDD-HHMMSS.tar.gz`
(owner-only) with a consistent snapshot of `tock.db`, safe while trackd runs,
plus `rules.toml`, `learned.toml`, `config.toml` and `shell.log` when they
exist. `--out FILE` writes elsewhere; an existing file is never overwritten.
The archive holds your titles and URLs, so keep it out of git and shared
folders.

To restore:

1. `make uninstall` to stop trackd.
2. Move `~/.local/share/tock/tock.db` and its `-wal` and `-shm` files aside.
3. Extract the archive in a temp directory. Put `tock.db` in
   `~/.local/share/tock/`, the `.toml` files in `~/.config/tock/` and
   `shell.log` in `~/.local/state/tock/`.
4. `make install`.

## Uninstall

`make uninstall` stops trackd and the menu bar app and removes their launchd
agents. Your data stays.
To delete the data too, remove `~/.local/share/tock`, `~/.local/state/tock`
and `~/.config/tock`.

## Development

```sh
make test                                  # vet, build, all tests
go test ./internal/tui -run Golden -update # regenerate TUI snapshots
make dump                                  # print trackd's live samples
```

Checklists for things only a person at the Mac can verify are in
`docs/*-checklist.md`.
