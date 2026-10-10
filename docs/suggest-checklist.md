# Claude suggestions checklist (user-run)

Run after `make install`, with `ANTHROPIC_API_KEY` exported in your shell
(never in a file tock reads, never in the launchd plist). Do not paste real
payloads, inbox lines or suggestions into issues or commits.

1. `tock suggest --dry-run`. Read the whole payload: no `(private)` key, no
   mail, chat, password manager or bank key, no raw window title, only
   hosts with one path segment and normalised titles.
2. `tock suggest`. It prints `sent N keys · M suggested`.
3. `tock inbox`: suggested rows end with `→ Category/Project`.
4. `tock` → `3`: the title shows `· M suggested`; rows show `→ …`. Press `a`
   on one, check the rule and its impact, press `y`. The key leaves the
   inbox.
5. `tock suggest` again: `next batch after HH:MM` or `nothing to send`.
6. `tock suggest --force`: only keys never sent before go out.
7. `unset ANTHROPIC_API_KEY`, open `tock`: everything works, no banner, no
   suggestion run.
8. `go list -deps ./cmd/trackd | grep -c net/http` prints 0.
