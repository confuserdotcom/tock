# v2 tier 8 checklist (user-run)

Timeline edits and undo, on real data.

1. **Add.** `tock edit add 08:00-08:30 <Category> commute` for a morning
   you were away. `tock today --keys` shows an `Entry` row; the tracked
   time grows by 30 min; `tock edit` lists it with its id.
2. **Split.** In the TUI's today tab, select a long row, press `s`, keep
   the suggested second half, enter, pick a label. The row splits in two
   with the second half labelled.
3. **Delete.** `d` on a row: its time leaves the day. `u` brings it back.
4. **Add in the TUI.** `+` on an idle row fills its range; pick a label.
5. **Relabel an entry.** `t` on an `Entry` row changes its label.
6. **Undo a rule.** Tag an inbox key with a new rule (`tock tag 1 <Cat>`),
   then `tock undo`: learned.toml is as before and the key is back in the
   inbox. Edit learned.toml by hand after a tag: `tock undo` refuses.
7. **Web.** In `tock web`, open a row, **Delete time**, then **Undo** in the
   toast (or `u`).
8. **Week and export.** Edited time shows in the week tab and in
   `tock export` (entries have bundle `tock.entry`).
