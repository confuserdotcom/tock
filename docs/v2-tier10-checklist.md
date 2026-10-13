# v2 tier 10 checklist (user-run)

Work hours and goal streaks, on real data.

1. **No schedule.** Without `[hours]`, `tock today` and the today tab look
   as before: no hours on the score line.
2. **Hours.** Add to `~/.config/tock/config.toml`:

   ```toml
   [hours]
   start = "09:00"
   end = "18:00"
   ```

   On a weekday, `tock today` ends its score line with `09:00–18:00` and,
   if you worked outside them, `1h20 outside hours`. The score and focus
   time only count 09:00–18:00; the category totals still cover the whole
   day.
3. **TUI and web.** The today tab shows `· hours 09:00–18:00 +1h20
   outside` after the score word; `tock web` shows `scored 09:00–18:00` under
   the title.
4. **Day off.** On a Saturday or Sunday (or set `days` to leave out today),
   `tock today` says `day off · not scored`, the today tab says `day off`,
   and `tock recap` says `Day off: … tracked, not scored.`
5. **Recap.** With trackd running, the recap notification arrives at
   18:00 on a weekday, and none comes on a day off. Setting
   `[focus] recap = "21:00"` moves it back to 21:00.
6. **Errors.** `end = "08:00"` with `start = "09:00"`, or
   `days = ["monday"]`, makes `tock doctor` and the TUI banner name the
   problem; the last good settings stay.
7. **Streaks.** With a `[[goal]]` met two days in a row, the goals line in
   `tock today` and the today tab ends that goal with `· 2d`; the week tab
   and `tock review` show `streak 2d (best …)`, and the dashboard's Goals
   card shows `2d streak`.
8. **Streak rules.** A day with no activity, or a day off, does not break
   a streak. A limit passed today drops its streak to 0 at once.
