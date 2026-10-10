# v2 tier 9 checklist (user-run)

Work blocks, summaries and questions, on real data. Do not paste summaries,
answers, titles or rule files into issues or commits.

1. **Install.** On the branch, run `make install`. `tock doctor` passes. The
   database migrates to version 12 on first open.
2. **Blocks.** After a morning of work, `tock blocks` lists stretches on one
   project or category, each at least 15 min. A 5-minute detour stays inside
   a block; a 10-minute break ends it. The names, deep shares and top apps
   match what you did.
3. **Dry runs.** With `ANTHROPIC_API_KEY` set:
   - `tock summary --dry-run` prints the blocks as JSON: clock times, labels
     and apps with hosts or titles. No date, no password manager, mail or
     bank. Nothing is sent.
   - `tock ask --dry-run "how much video this week?"` prints only the
     question, today's date and your category and project names.
4. **Summary.** `tock summary` prints a few sentences about the day and a
   line under each block. It reads right and invents nothing.
   - `tock blocks` shows it again without a new call.
   - The TUI's today tab shows a `summary` line; `tock web` shows it in the
     **Work blocks** card, with each block's line.
   - `tock summary --date <yesterday>` summarizes yesterday.
5. **Ask.** Try a few questions and check them against `tock top` or the
   week tab:
   - `tock ask "how much youtube last week?"`
   - `tock ask "which projects did I work on this month?"`
   - `tock ask "deep work per day this week"`
   - `tock ask "what is the weather"` says `tock cannot answer that`.
6. **Off.** With `[llm] enabled = false`, `tock summary` and `tock ask` say
   Claude is off and send nothing; `tock blocks` still works.
