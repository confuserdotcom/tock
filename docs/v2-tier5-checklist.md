# v2 tier 5 checklist (user-run)

Checks that need a person at both machines. Do not paste titles, URLs,
rule files or day-file contents into issues or commits.

## Linux (ryuk, Hyprland)

1. **Install.** Clone the repo on ryuk and run `make install-linux`.
   - `systemctl --user status trackd` shows the unit active.
   - `tock doctor` shows `ok  trackd: running` and
     `ok  notifications: notify-send`.
2. **Windows.** Switch between kitty and firefox for a few minutes.
   - `tock today` shows both, with their Hyprland classes as app names.
   - Window titles appear on the today tab of the TUI.
3. **Idle.** Add the hypridle listener from the README, then
   `systemctl --user restart hypridle` (or log out and in). Leave the
   machine for 6 minutes. The today tab shows an idle row starting about 5
   minutes before the timeout ended.
4. **Lock.** Lock with hyprlock for a few minutes. No activity is recorded
   while locked.
5. **Restart.** Restart Hyprland (log out and in). trackd restarts by itself
   within a minute (`journalctl --user -u trackd` or trackd.log).
6. **Rules.** Write a rule for a Linux app, for example
   `app = "kitty"`, then run `tock rules check`. The time gets labelled.

## Sync

1. Put the same synced folder in `[sync] dir` on both machines, with
   `machine = "mac"` and `machine = "ryuk"`.
2. Work on ryuk for 15 minutes while the Mac sits idle or locked.
   - Within a couple of minutes of the files syncing, the Mac's today tab
     shows `ryuk · <app>` rows in that time.
   - The tracked time on the Mac grows by about 15 minutes.
   - `tock doctor` on the Mac shows `ok  sync: as mac with ryuk (last …)`.
3. The same works the other way: Mac time shows on ryuk.
4. **Labels.** With the same category names on both machines, ryuk rows
   show ryuk's labels. On a ryuk row, `t` says
   `recorded on another machine: label it there`.
5. **Backfill.** `tock sync --days 30` writes older days; the week and
   trends views pick them up on the other machine.
6. **Privacy.** trackd.log on both machines shows only counts
   (`synced written=… read=…`), never titles.
