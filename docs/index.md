# tock

<p class="lead">tock records what you work on, on a Mac or on Linux under Hyprland, and labels it with rules you write. You never start a timer. Your data stays on your machine, in one SQLite file.</p>

<picture class="shot">
  <source media="(prefers-color-scheme: dark)" srcset="readme/web-dark.png">
  <img src="readme/web-light.png" alt="The tock dashboard for one day: the day's numbers compared with a usual Saturday, a timeline with one row per category, and tables of categories and apps.">
</picture>

## What it does

- **Records without a timer.** `trackd` notes the app and window in front,
  the browser URL, and when you are away. It uses under 1 % CPU and about
  36 MB of memory, and it has no network code.
- **Labels with your rules.** `rules.toml` maps apps, titles and sites to
  categories and projects. Change a rule and past days follow.
- **Asks once.** Time no rule covers goes to an inbox. Labelling it there
  writes a rule, so the same thing is never asked again. A model trained on
  your own labels, on your machine, suggests the rest.
- **Shows the day plainly.** A terminal UI, a dashboard on 127.0.0.1, and a
  menu bar app. Every number is compared with your usual same weekday.
- **Focus sessions.** `tock session start 50` or `⌥⌘F` from any app. tock
  calls you back when you drift.

## Install

On macOS you need Go 1.26 and the Xcode command line tools.

```sh
git clone https://github.com/confuserdotcom/tock && cd tock
make cert && make cert-trust   # once: a local code-signing certificate
make install                   # builds, signs, starts trackd, runs tock doctor
```

On Linux under Hyprland, run `make install-linux`, or download a tarball
from [Releases](https://github.com/confuserdotcom/tock/releases). Coming from
ActivityWatch? `tock import activitywatch export.json` brings your history.

## Privacy

- Activity is stored in `~/.local/share/tock/tock.db` (mode 0600) and leaves
  the machine only if you export it.
- A test fails the build if `trackd` ever links an HTTP or TLS client.
- Private windows, and apps and sites you hide, are recorded without title
  or URL.
- Claude suggestions, summaries and Slack status are the only network
  features. All are off by default, and `--dry-run` prints exactly what
  would be sent.

The whole list is in [Privacy in detail](guide#privacy-in-detail).

## Read next

- [Guide](guide): every command, file and setting.
- [Architecture](architecture): how the pieces fit, and what they cost.
- [Changelog](https://github.com/confuserdotcom/tock/blob/main/CHANGELOG.md).
