# Privacy

tock records which app and window are in front, all day. That is private by
nature, so this page says exactly what is kept, where, and what can leave
your machine. Everything here is checked against the code; if you find a
difference, it is a bug (see [SECURITY.md](../SECURITY.md)).

## What is recorded

trackd writes spans to `~/.local/share/tock/tock.db`:

- start and end time;
- the app (bundle id and name);
- the window title, and the URL of the front browser tab;
- whether you were active, idle, asleep or locked.

It does not record keystrokes, screenshots, clipboard, or what is inside a
window. Optional extras, each off until you turn it on:

- **Calendar** (`[calendar]`): event titles and times from the calendars
  you pick, read through macOS's own calendar access.
- **Screen text (OCR)** (`[ocr]`): text read from the screen for the apps you
  list, to help label time. It stays in tock.db and is never sent anywhere.

## What is never stored

- **Private browser windows:** the URL is dropped, and on Chromium
  browsers the title too. On macOS tock reads the URL from Chrome, Arc,
  Brave, Edge, Vivaldi, Chromium and Safari (beta and nightly builds
  included), and only titles from Firefox and its forks (Zen, LibreWolf,
  Waterfox, Floorp). Browsers whose private windows it cannot recognise
  (Opera, Dia, Orion, Tor Browser, Mullvad Browser, Safari Technology
  Preview, Comet, Helium, SigmaOS) are recorded as the app only, never
  with a title or URL.
- **Hidden apps and sites** (`[privacy] hide_apps`, `hide_sites`): kept as
  time only, with no title or URL.
- **While `config.toml` does not load**, trackd records nothing, so a typo
  can never switch your hide lists off.

`[privacy] keep_titles = N` deletes titles and URL paths older than N days.

## Files

The data dir (`~/.local/share/tock`) and the state dir (`~/.local/state/tock`)
are readable by you only (0700 folders, 0600 files); trackd closes either
folder to other users at start if it finds it open. That covers the
database, its snapshots in `backups/`, the logs, and the invoice previews
and week reports tock writes for you (`drafts/`, `reports/` in the state
dir). Snapshots and `tock
backup` archives hold your titles and URLs too, so keep them out of git and
shared folders.

Issued invoices sit in the database and in `invoices/` in the data dir.
They hold your and your client's names, addresses and tax numbers, and the
time billed per day, but no window titles or URLs, so `tock prune` and
`keep_titles` leave them alone. They are never synced.

## What leaves the machine

trackd and the menu bar app have no network code at all; tests check that
they don't link any. Only `tock` itself talks to the network, and only for
these, each off until you turn it on:

| Feature | Goes to | What is sent | Turn on / off |
|---|---|---|---|
| Claude suggestions | Anthropic, through the `claude` CLI or `ANTHROPIC_API_KEY` | app names, sites and title snippets of unlabelled time, with minutes | `tock choice claude on` / `off` |
| `tock summary` | Anthropic | the day's work blocks | same |
| `tock ask` | Anthropic | your question and your label names | same |
| Slack status | Slack | a status text and do-not-disturb during a focus session; no activity | `tock slack login` / `logout` |

Never sent to Claude: private windows, hidden apps and sites, the `[llm]`
deny lists (password managers and mail by default), and screen text.
`--dry-run` on `tock suggest`, `tock summary` and `tock ask` prints what
would be sent without sending it.

The dashboard (`tock web`) listens on 127.0.0.1 only, needs a per-run
token, and loads nothing from the internet.

## The sync folder

With `[sync] dir` set, each machine writes its spans (titles and URLs
included), its `machine.json` and, unless `rules = false`, its
`rules.toml` to that folder. tock does not upload it anywhere: whatever
syncs the folder (Syncthing, iCloud Drive, Dropbox) decides where it goes.
Pick one you trust with your activity.

On Linux, the sync folder can also carry new tock builds from the Mac, if
you turn that on (`[sync] update = true`). A build is installed only when
it is signed by the key you trusted by fingerprint with `tock update
trust`; the folder alone can never make a machine trust a key. See
"Updates" in the [guide](guide.md).

## No telemetry

tock sends no usage data, crash reports or analytics. `tock bug-report`
writes a file for you to look through and attach to an issue yourself:
`tock doctor --json`, `config.toml` with every text value removed, the
end of the logs without lines that could hold a title or URL, and when
trackd ran in the last two weeks. Your home and sync folders are replaced
by `~` and `<sync>`. Your rules go in only with `--include-rules`.
