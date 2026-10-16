# v3 checklist (user-run)

AFK, categories, modules and the clickable TUI, on real data.

1. **Video keeps you present.** `make install`, then play a YouTube video in
   Arc full-window for 8 minutes without touching anything. The today tab
   shows the video, not an idle row. `trackd -dump` prints `video=1` while
   it plays and `video=0` once paused. Spotify playing alone stays
   `video=0`.
2. **AFK windows.** `tock category set Reading --afk 10m`. Read a PDF in
   Books without input for 7 minutes, then touch the trackpad: one Reading
   row, no idle. Leave for 15 minutes: an idle row starting when you
   stopped.
3. **ryuk.** `make deploy-linux` from the Mac, then add the hypridle
   listener (README › Linux) and `systemctl --user restart hypridle`.
   `tock doctor` on ryuk has no `!! idle`; after its next sync the Mac's
   doctor has no `!! afk: ryuk does not report input`. Leave ryuk for 6
   minutes: an idle row on ryuk's timeline.
4. **Categories.** `tock category add Podcasts neutral --afk 30m`, then
   `tock category rename Podcasts Audio`, then `tock category rm Audio`.
   rules.toml keeps its comments; rules.toml.bak holds the last version.
5. **Modules.** Open a PDF titled with a module's name (or alias) in any
   app: the row is `Study / <module>`. In the plan tab's slot for `+ana1`,
   an Obsidian note with no module in its title lands in Analyse 1.
6. **Labels tab.** `7`: categories with their modules. `a` adds one; click
   its kind chip to change it, its afk chip for the window; `m` adds a
   module; `e` renames; `d` on a category in use says to merge.
7. **Mouse.** Hover a category in Where it went: the footer says what a
   click does. Click it: only its rows. Right-click a row: its actions;
   pick one. Click a cell of the day strip: the row at that time. After a
   tag, click `u undo` in the footer.
8. **Wide terminal.** At 130 columns or more, Where it went, goals and
   work blocks sit in a sidebar left of the timeline.
