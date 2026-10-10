# Focus engine checklist (user-run)

Run after `make install`. Nothing here is automated: it needs real
notifications, a real call and a real camera. Do not paste notification text
from real days into issues or commits.

1. `tock session start 2`. Open a distraction site for 70 s. One notification
   "tock · focus" arrives; `tock session` shows 1 interruption.
2. `tock` → `4`: the countdown runs every second; the header shows ◷.
3. Wait for the end: one "tock · session done" notification with deep %,
   interruptions and a break suggestion.
4. Start another session, join a Zoom or Meet call (or turn the camera on in
   any app), open a distraction site for 2 min: no notification.
5. The first notification may make macOS ask to allow notifications for
   Script Editor (osascript). Allow it in System Settings › Notifications.
6. Recap: set `[focus] recap` in `~/.config/tock/config.toml` one minute
   ahead. One "tock · recap" notification arrives. Restart trackd
   (`launchctl kickstart -k gui/$UID/com.tock.trackd`): no second one.
7. Work 30 min on something deep with no session; after you switch away for
   over a minute, `tock` → `4` lists an `auto` session.
8. `grep -c 'focus notification' ~/.local/state/tock/trackd.log` counts the
   notifications; the log shows titles only, never a window title or URL.
