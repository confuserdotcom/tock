# v2 tier 3 checklist (user-run)

Checks that need a person at the Mac. Do not paste real titles, event names,
rates, repo paths or rule files into issues or commits.

1. **Install.** On `main`, run `make install`. `tock doctor` passes. The
   database migrates to version 8 on first open.
2. **Commits.** Add `repos = ["~/tock"]` to a `[[project]]` in rules.toml.
   - `tock doctor` shows `ok  git: 1 repo`.
   - After a commit in that repo, `tock today` and the today tab show
     `commits  <project> N` within a minute.
   - The week tab shows a `Commits` line, and `tock review` a `commits` line.
   - A misspelled repo path makes `tock doctor` report it.
3. **Billing.** Add `rate = 50` to a project.
   - `tock export` rows of that project have `rate` and `amount`.
   - `tock export --by project` prints one row per project. The amount equals
     the rate times the hours.
4. **Calendar.** Add your Google account under System Settings › Internet
   Accounts with Calendars on, and check that Calendar.app shows your events.
   - Set `[calendar] enabled = true` in config.toml, and `[calendar] category`
     in rules.toml (README "Calendar").
   - `make install`. macOS asks whether trackd may access your calendars;
     allow it.
   - Within a minute, `tock doctor` shows `ok  calendar: synced HH:MM`.
   - Lock the Mac during a calendar event of at least 15 minutes. Afterwards
     the today tab shows a `Meeting` row titled with the event and labelled
     with your calendar category. No away notification comes for that time.
   - `t` on the meeting row relabels it.
   - An event during which you worked at the Mac leaves that work labelled as
     it was.
   - Event titles do not appear in `~/.local/state/tock/trackd.log`.
   - Denying access in System Settings › Privacy & Security › Calendars makes
     `tock doctor` say `access denied`.
