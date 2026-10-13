# v2 tier 9 checklist (user-run)

Work blocks, AI summaries and `tock ask`, on real data.

1. **Blocks.** After a few hours of work, `tock blocks` lists stretches of
   work on one thing, each 15 min or more, with focus share and top apps. A
   short detour (under 10 min) stays inside the block around it; a 10 min
   break starts a new one.
2. **TUI.** The today tab shows a `work` line with the same blocks:
   `09:00 tock 1h40 · …`.
3. **Web.** `tock web` shows a **Work blocks** card under the timeline.
4. **Dry run.** `tock summary --dry-run` prints the payload: weekday, clock
   times, labels and per-block items. No dates, no private windows, nothing
   from the `[llm]` deny lists.
5. **Summary.** With `ANTHROPIC_API_KEY` set, `tock summary` prints the day's
   line and one line per block. The TUI shows the day's line after `✎`,
   `tock web` at the top of the Work blocks card. Running it again replaces
   them.
6. **Another day.** `tock summary --date <yesterday>` then
   `tock blocks --date <yesterday>` shows yesterday's lines.
7. **Ask.** `tock ask --dry-run "how much video last week?"` prints only the
   question, today's date and your category and project names.
   `tock ask "how much video last week?"` answers with a total for last
   week; spot-check it against last week in the TUI's week tab.
8. **Off switch.** With `[llm] enabled = false`, `tock summary` and
   `tock ask` send nothing and say Claude is off.
