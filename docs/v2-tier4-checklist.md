# v2 tier 4 checklist (user-run)

Checks that need a person at the Mac. Do not paste notification text, labels
or rule files into issues or commits.

1. **Install.** On `main`, run `make install`.
   - The database migrates to version 9 on first open.
   - A `◷` item appears in the menu bar.
   - macOS asks whether **tock** may send notifications; allow it.
   - Within a minute, `tock doctor` shows
     `ok  notifications: native, with buttons (menu bar)`.
2. **Menu bar.**
   - The title shows today's tracked time, the same as `tock status`.
   - The menu lists today, focus and score, and any unlabelled time.
   - **Start 25 min focus**: the title turns into a `◉ 24:59` countdown that
     ticks every second. `tock status` shows the session.
   - **Stop session**: the title goes back to today's time.
   - Stop trackd with `launchctl bootout gui/$UID/com.tock.trackd`. Within
     a minute the title ends in `!` and the menu says
     `trackd is not running`. Run `make install` to restore it.
3. **Notifications with buttons.** Hover over a banner, or open it in
   Notification Center, to see its buttons.
   - Start a 25 min session and open a distracting site for a minute or two.
     The **tock · focus** notification comes from *tock*, not Script
     Editor. **Stop session** ends the session.
   - Let a session run to its end. **Start again (N min)** starts a new one
     of the same length.
   - Work 50 minutes without a break. **Snooze 30 min** on **tock · break**
     holds the next reminder for 30 minutes.
   - Lock the Mac for 15 minutes or more, then come back. **tock · away**
     offers your recent away labels and **Leave idle**. Picking a label
     labels the time on the today tab.
4. **Fallback.**
   - Turn tock off in System Settings › Notifications. `tock doctor` says
     notifications are plain, and notifications arrive as before, without
     buttons.
   - **Quit tock menu bar**: the item goes away and stays away. Notifications
     still arrive, plain. `make install` brings the item back.
5. **Privacy.** `~/.local/state/tock/tockbar.log` holds topics, action ids
   and results, never notification text, window titles or URLs.
