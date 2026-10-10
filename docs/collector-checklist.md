# Phase 1 collector checklist (D-29)

Run every item against the launchd agent installed by `make install`, never
against a terminal-launched trackd: a terminal-launched process sees the
terminal's permissions, not trackd's (Pitfall 2).

Shorthand used below (paste once into your shell):

```sh
DB=~/.local/share/tock/tock.db
LOG=~/.local/state/tock/trackd.log
pid() { launchctl print gui/$(id -u)/com.tock.trackd | awk '/^[[:space:]]*pid = /{print $3; exit}'; }
recent() { sqlite3 -header -column $DB "select id, datetime(start/1000,'unixepoch','localtime') s, datetime(\"end\"/1000,'unixepoch','localtime') e, bundle_id, title, url, private from span order by id desc limit ${1:-10}"; }
```

Fill in each `result:` with pass, or fail plus the evidence.

## 1. Preconditions

- [ ] Agent running, Accessibility granted, screen unlocked.
  `launchctl print gui/$(id -u)/com.tock.trackd | grep 'state ='` and
  `sqlite3 $DB "select value from daemon_state where key='ax_trusted'"`.
  Expected: `state = running` and `1`.
  result:

## 2. App switch latency

- [ ] Note the clock, switch from WezTerm to Finder, wait 5 s, run `recent 3`.
  Expected: the Finder span starts within 1 s of the switch.
  result:

## 3. Titles every 2 s

- [ ] In one app, change window title (switch Finder folders or browser tabs)
  three times about 5 s apart, then `recent 5`.
  Expected: a new span per title, starts about 2 s or less after each change.
  result:

## 4. Browsers, normal and private windows

For each browser: open a normal window on a page with a query string (for
example a search results page), wait 10 s; then a private window, wait 10 s;
then `recent 5`. Expected for normal windows: title present, `url` is
host+path with no scheme, query or fragment. Expected for private windows:
`title` NULL, `url` NULL, `private` 1.

- [ ] Safari normal window. result:
- [ ] Safari Private Browsing window. result:
- [ ] Chrome normal window. result:
- [ ] Chrome incognito window. result:
- [ ] Arc normal window. result:
- [ ] Arc private window. result:
- [ ] Brave normal and private window. Brave was not installed on 2026-10-10;
  record "not installed" unless that has changed (RESEARCH A4). result:
- [ ] Firefox normal window: title present, `url` NULL (title-only browser).
  Firefox private window: title NULL, `private` 1. result:

## 4b. Safari private window with Accessibility revoked

- [ ] Turn trackd off in System Settings > Privacy & Security > Accessibility.
  Keep a Safari private window frontmost on a distinctive page for 1 minute.
  Before re-granting, run:
  `sqlite3 $DB "select count(*) from span where bundle_id='com.apple.Safari' and url is not null and start > (strftime('%s','now','-2 minutes')*1000)"`.
  Expected `0`: with no AX title, Safari privacy is unknown, so no AppleScript
  runs and no URL is stored (plan 01-06, D-07). Turn trackd back on and confirm
  `ax_trusted` returns to `1` (the agent may restart itself to pick the grant up).
  result:

## 5. Automation

- [ ] First browser use after install shows an Automation prompt that names
  trackd, not WezTerm or another app (RESEARCH A2). If grants already exist,
  check System Settings > Privacy & Security > Automation lists trackd with
  each browser beneath it. result:
- [ ] Turn one browser off under trackd in the Automation pane, use that
  browser for 30 s, then
  `sqlite3 $DB "select key,value from daemon_state where key like 'automation.%'"` and `recent 3`.
  Expected: `automation.<bundle_id>|denied`, and that browser's new spans have
  `url` NULL (Chromium browsers: `title` NULL too). `grep 'automation status changed' $LOG`
  shows one warning. Turn it back on afterwards. result:

## 6. Accessibility revoke and re-grant

- [ ] Turn trackd off in the Accessibility pane, switch apps for 20 s, then
  `grep -n 'accessibility' $LOG | tail -3`, `sqlite3 $DB "select value from daemon_state where key='ax_trusted'"` and `recent 5`.
  Expected: log line `accessibility not granted; recording apps only`,
  ax_trusted `0`, spans still carry bundle_id with title NULL. Turn trackd
  back on; within 20 s ax_trusted is `1` and titles return. result:

## 7. Idle

- [ ] Leave the Mac untouched (screen awake, unlocked) for 6 minutes, then `recent 3`.
  Expected: a `tock.idle` span that starts at your last input (within a few
  seconds), not 5 minutes later. result:

## 8. Sleep and wake

- [ ] Note the clock, close the lid (or Apple menu > Sleep) for 2 minutes, wake, then `recent 5`.
  Expected: the span open before sleep ends at the sleep time; no span covers
  the sleep interval; a new span starts after wake. result:

## 9. Lock and unlock

- [ ] Note the clock, press Ctrl+Cmd+Q, stay locked 1 minute, unlock, then `recent 5`.
  Expected: the span before the lock ends at the lock time and no span
  covers the locked minute (no `com.apple.loginwindow` rows). The 01-10
  spike recorded `fallback_applied: none`; if that ever changes to
  `deliver-immediately`, this item is the re-verification. result:

## 10. Crash recovery

- [ ] `P=$(pid); kill -9 $P; date`, wait 20 s, then `pid` and
  `sqlite3 $DB 'select count(*) from span where "end" is null'` and `recent 3`.
  Expected: a new pid; at most 1 open span; the span open at the kill ends
  within 10 s of the kill time; `grep 'recovered open spans' $LOG | tail -1`
  shows count=1. result:

## 11. launchctl bootout and bootstrap

- [ ] `launchctl bootout gui/$(id -u)/com.tock.trackd`, confirm `pid` prints
  nothing, then `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.tock.trackd.plist`.
  Expected: running again within 5 s; `recovered open spans count=0` in the
  log (a clean stop closes its span). result:

## 12. Reboot and login auto-start

- [ ] Restart the Mac and log in. Without running anything, check `pid` and `recent 3`.
  Expected: agent running and recording from login. result:

## 13. Background Items

- [ ] `sfltool dumpbtm | grep -A3 com.tock.trackd` lists the agent. In System
  Settings > General > Login Items & Extensions, toggle trackd off: the agent
  stops (`pid` empty). Toggle it on and run `make install`: running again.
  Note: while toggled off nothing is recorded and nothing warns you yet (the
  Phase 3 TUI banner covers this). result:

## 14. Reinstall keeps grants, no keychain dialog

- [ ] Run `make install` twice in a row.
  Expected: both succeed with no keychain dialog (spike record
  `key_access: always-allow`); afterwards ax_trusted is `1` and new browser
  spans still have URLs. result:

## 15. CPU and memory

- [ ] After 10 minutes of normal use including browsing, `ps -o rss,%cpu,time -p $(pid)`.
  Expected: RSS under 40000 KB, %cpu about 0.0 to 0.5, cumulative CPU time
  under 1 % of wall time since start (D-26). result:

## 16. No network

- [ ] `lsof -nP -i -a -p $(pid)`.
  Expected: no output (D-25). result:

## 17. Logs contain no titles or URLs

- [ ] Pick a distinctive window title and domain you used today, then
  `grep -c '<that title>' $LOG; grep -c '<that domain>' $LOG`.
  Expected: `0` for both. result:

## 18. Success criterion 1: an hour of real work

- [ ] Work normally for an hour, then
  `sqlite3 -column $DB "select datetime(start/1000,'unixepoch','localtime'), datetime(\"end\"/1000,'unixepoch','localtime'), app, title, url from span where start > (strftime('%s','now','-1 hour')*1000) order by id"`.
  Expected: the rows match what you did, with switches within a few seconds,
  titles for apps you used, URLs as host+path for browsers. result:

## Sign-off

Checked by: ________________  Date: ____________
