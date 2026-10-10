# v2 tier 7 checklist (user-run)

The focus guard acts on real apps, tabs, Shortcuts and Slack. Check it at
the Mac. Do not paste titles, URLs or the Slack token into issues.

## Blocking

1. Label a site as a distraction (or `block_hosts = ["youtube.com"]`), set
   `[focus] block = true`, then `make install`.
2. `tock session start 25`. Open the site in Chrome (or Arc, Brave,
   Safari) and leave it in front.
   - Within about 15 s the tab closes and a notification says
     `Closed a distracting tab in Chrome. N min left in your session.`
   - Switching to another tab before it closes keeps that tab open.
3. Open a distracting app (for example one in `block_apps`).
   - It is hidden and the notification says `Hid <App>.`
4. Join a call (camera on): nothing is blocked during it.
5. `tock session stop`: nothing is blocked any more.
6. trackd.log has `focus guard blocked a distraction` lines without the
   site.

## Focus mode

1. Make the `Focus On` / `Focus Off` shortcuts (README, Focus guard) and set
   `focus_on` / `focus_off`.
2. `tock doctor` shows `ok  focus mode: shortcuts "Focus On" / "Focus Off"`.
3. Start a session from the menu bar: Focus turns on within a few seconds.
   Let it end on its own: Focus turns off.

## Slack

1. `tock slack login`, `[slack] enabled = true`, `tock slack` says
   `token: ok`.
2. Start a 25 min session: your Slack status shows 🎯 In a focus session,
   expiring at the session's end, and notifications are paused.
3. Stop it: the status clears and notifications resume.
4. Set your own status during a session, then stop it: your status stays.
