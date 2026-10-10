# Ship checklist (user-run)

Phase 7 checks that need a person at the Mac. Do not paste real titles, URLs,
export rows or rule files into issues or commits.

1. **Install.** On `main`, run `make install`. It ends with `tock doctor`.
   - Grant Accessibility when macOS asks. If you miss the prompt, run
     `tock doctor --open`.
   - Run `tock doctor` again. All six lines start with `ok`.
2. **Starter rules.** Do this with `~/.config/tock/rules.toml` moved aside.
   - `make install` writes a fresh one. Moving yours back restores it.
   - With rules.toml present, `make install` leaves it untouched.
3. **Rules tab: reorder.** Back up rules.toml with
   `cp ~/.config/tock/rules.toml /tmp/rules.bak`.
   - Run `tock` and press `5`. Learned rules (`L1`, `L2`, …) are listed
     first and live in learned.toml, so press `G` to jump to the last rule,
     which is in rules.toml. Press `K` (Shift+k; plain `k` only moves the
     cursor). The flash says `moved rules.toml#N up`.
   - `diff /tmp/rules.bak ~/.config/tock/rules.toml` shows only two rule blocks
     trading places, each with its own comment lines. Section banners stay put.
   - `K` on the first rule says `already first`.
4. **Rules tab: editor.** Press `e`. Your `$EDITOR` opens rules.toml.
   - Quit it: the TUI comes back and shows any edit.
   - Restore with `cp /tmp/rules.bak ~/.config/tock/rules.toml`.
5. **Export.** Tests already check that csv and json round-trip, so on your
   data only check that it runs, without writing it to a file:
   - `tock export --format json | jq length` prints the row count.
   - `tock export --from 2026-01-01 --to 2025-12-31` exits 2.
6. **Reboot.** Log in again. `tock doctor` passes with no action from you, and
   `tock status` shows today's time.
