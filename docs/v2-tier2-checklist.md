# v2 tier 2 checklist (user-run)

Checks that need a person at the Mac. Do not paste real titles, URLs, plans
or rule files into issues or commits.

1. **Install.** On `main`, run `make install`. `tock doctor` passes. The
   database migrates to version 7 on first open.
2. **Goals.** Add two `[[goal]]` tables to rules.toml (README "Goals"), one
   goal and one limit. `tock rules check` lists them.
   - The today tab shows the goals line; a passed limit is red.
   - `tock status` ends with `goals N/M` (and `over <name>` once a limit is
     passed).
   - The week tab lists each goal over the week.
3. **Time away.** Lock the screen for 12 minutes, then come back and use the
   Mac for a minute.
   - One `tock · away` notification arrives, and only one.
   - The today tab shows `away HH:MM–HH:MM …`. `a`, pick a category: the
     row turns into `Away` with that label and the prompt goes.
   - Do it again and press `A`: the time stays a gap and is not asked again.
   - `tock away` lists nothing left to label.
   - A break reminder does not fire right after labelling a break.
4. **Stretch.** `?` on the today tab shows the `stretch` line with your
   longest run. Optionally set `[score] depth = 0.4` and `stretch = 0.1` in
   config.toml and check that the score changes.
5. **Trends.** `tock trends` prints this month against last month's same
   days. On the week tab, `m` shows the same, `h` goes back a month, and `m`
   returns to the week.
6. **Review.** `tock review` prints the week and asks for next week's focus.
   Type one line. Next week, the week tab shows `plan: …`.
