# v2 tier 11 checklist (user-run)

Week report, PDF invoices and deleting old titles, on real data.

1. **Report.** `tock report --open` opens last week (this week on a
   weekend) in the browser: totals, days with scores, categories, projects,
   apps and sites, goals with streaks. Switch the system to dark mode and
   reload: it follows. View the source: no `<script`, no `http`.
2. **Report file.** `tock report --week 2026-10-05 -o ~/Desktop/week.html`
   prints `wrote …`; mail it to yourself and check that it reads well in
   Mail.
3. **Invoice.** Give a project a `rate` in rules.toml and fill `[invoice]`
   in config.toml. `tock invoice --project <it> --client "Someone"`
   writes `invoice-YYYYMM-<project>.pdf` for last month and prints the
   day count, hours and total. Open it in Preview: name and address top
   left, INVOICE with number, date and period top right, one line per day,
   totals that match `tock export --by project` for the same days.
4. **Long invoice.** A period with more than ~30 working days spills onto a
   second page with the table header repeated and `page 2 of 2`.
5. **Invoice errors.** A project without a rate, or a month with no time on
   it, gives a clear error and writes no file.
6. **Backup first.** `tock backup` before trying title deletion.
7. **Dry run.** `tock prune --days 30 --dry-run` prints how many spans
   would lose their titles; nothing changes (`tock today --date <old day>`
   looks the same).
8. **Delete.** Set `[privacy] keep_titles = 30` and restart trackd
   (`launchctl kickstart -k gui/$UID/com.tock.trackd`). Within a minute
   trackd.log has `titles deleted … spans=N`. `tock today --date <a day 40
   days ago> --keys` shows the same categories and totals as before, keys
   now app or host only. `tock doctor` shows the privacy line.
9. **Sync.** With `[sync]` on, your own day files older than 30 days in
   the sync folder no longer hold `"t":` fields or URL paths.
