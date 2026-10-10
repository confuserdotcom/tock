# v2 tier 1 checklist (user-run)

Checks that need a person at the Mac. Do not paste real titles, URLs, dirs or
rule files into issues or commits.

1. **Install.** On `main`, run `make install`. `tock doctor` passes.
2. **Calls.** If your rules.toml predates this, add a `Meeting` category
   (kind `shallow`) and `[calls] category = "Meeting"`. `tock rules check`
   prints `calls: labelled Meeting`.
   - Join a call (or open Voice Memos and record) for 3 minutes with the
     camera off. `tock today` shows that stretch as Meeting.
   - Turn on the camera alone (Photo Booth) for 3 minutes: same result.
   - Stay idle in a call for 6 minutes: the timeline shows `Call`, not idle.
3. **Shell hook.** Add `eval "$(tock shell-init zsh)"` to `~/.zshrc` and open
   a new terminal.
   - `cd` into a project and run a command. `tail -1
     ~/.local/state/tock/shell.log` shows the directory.
   - Add a `dir` rule for it (docs/shell-hook.md). After a few minutes
     there, `tock today` labels that terminal time with the project.
   - Prompts feel as fast as before.
4. **Backup.** `tock backup` prints the archive path.
   `tar tzf <path>` lists `tock.db` and your rule files.
   `ls -l <path>` shows `-rw-------`.
5. **Rule stats.** `tock rules stats` lists every rule with its time. Rules
   you know are dead say `unused`. The rules tab (`5`) shows the same times.
