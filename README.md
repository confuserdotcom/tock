# tock

[![ci](https://github.com/confuserdotcom/tock/actions/workflows/ci.yml/badge.svg)](https://github.com/confuserdotcom/tock/actions/workflows/ci.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg)](LICENSE)

tock records what you work on, on a Mac or on Linux under Hyprland, and
labels it with rules you write. You never start a timer. Your data stays on
your machine, in one SQLite file.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/readme/web-dark.png">
  <img src="docs/readme/web-light.png" width="100%" alt="The tock dashboard for one day: a timeline with one row per category; a focus score of 80, 17 above a usual Saturday, with focus time, longest block, distraction and switches; minutes per hour; a donut of the kinds of time; goal rings; tables of categories and apps with totals; work blocks.">
</picture>

<table>
  <tr>
    <td width="50%"><picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/readme/web-week-dark.png">
      <img src="docs/readme/web-week-light.png" alt="The week view: hours per day stacked by kind of time, a donut of the week, the focus score per day against its mean, and a weekday-by-hour heat map of when you focus.">
    </picture></td>
    <td width="50%"><picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/readme/web-trends-dark.png">
      <img src="docs/readme/web-trends-light.png" alt="The trends view: the month's tracked time, focus, average score and switches; a calendar shaded by deep time; deep time and focus score per day.">
    </picture></td>
  </tr>
</table>

## How it works

- `trackd`, a small daemon, records the app and window in front and the
  browser URL, and notices when you are away. It has no network code. It
  uses under 1 % CPU and about 36 MB of memory.
- `rules.toml` maps apps, titles and sites to categories and projects. A
  rule applies to past days as well as new ones.
- Time no rule covers goes to an inbox. Labelling an entry there also writes
  a rule, so you label each thing once. A model trained on your own labels,
  on your machine, suggests labels for the rest.
- `tock` is the CLI and a terminal UI. `tock web` serves the dashboard
  above on 127.0.0.1. A menu bar app shows today's time and starts focus
  sessions (`⌥⌘F` from any app).

The day gets a focus score out of 100 from deep time, long unbroken blocks
and few switches, minus distraction. Every number is compared with your
usual same weekday.

<img src="docs/assets/diagrams/pieces.svg" width="100%" alt="trackd takes events from the desktop (NSWorkspace, the Accessibility API, AppleScript, or Hyprland's socket) and writes spans to tock.db. tock and tockbar read tock.db and label it with rules.toml, learned.toml and config.toml. tockbar runs tock web as a child for the dashboard window. Only tock reaches the internet, for Claude and Slack, when you turn them on.">

How a span gets its label, how the notch is fed and how the install
signs the apps are drawn in [Architecture](docs/architecture.md).

Optional, and off until you turn them on: focus sessions that call you back
when you drift, goals, work hours, a calendar view, week reports, billing,
git commit counts, sync between machines, and Claude suggestions and
summaries. See [the guide](docs/guide.md).

## Privacy

- Activity is stored in `~/.local/share/tock/tock.db` (mode 0600) and never
  leaves the machine unless you export it.
- A test fails the build if `trackd` ever links an HTTP or TLS client.
- Private browsing windows, and apps and sites you hide, are recorded
  without title or URL.
- The only features that use the network are Claude suggestions and
  summaries and Slack status. All three are off by default. `--dry-run`
  prints exactly what would be sent.

The full list is in [Privacy in detail](docs/guide.md#privacy-in-detail).

## Install

**macOS, with Homebrew:**

```sh
brew tap confuserdotcom/tock
brew trust confuserdotcom/tock   # Homebrew loads no other tap until you trust it
brew install tock
tock setup
```

Homebrew builds tock on your Mac, so macOS never shows an "unidentified
developer" warning. `tock setup` makes a code-signing certificate that
stays in your login keychain (macOS asks for your password once to trust
it), installs `trackd.app` and the menu bar app, starts them, writes
starter rules to `~/.config/tock/rules.toml` and runs `tock doctor`. On
first start trackd asks for Accessibility, and browsers ask for
Automation. After `brew upgrade tock`, run `tock setup` again. macOS ties
the Accessibility grant to the certificate, so updates keep it.
`tock setup --check` shows what is missing.

**macOS, without Homebrew:** download `tock-<version>.dmg` from
[Releases](https://github.com/confuserdotcom/tock/releases) (Apple Silicon
and Intel). tock has no paid Apple Developer ID, so the download isn't
notarized and macOS stops it the first time:

1. Open the .dmg and drag tock to Applications.
2. Open tock from Applications. macOS says it can't check it for malicious
   software. Click Done.
3. Open System Settings, then Privacy & Security. Under Security, click
   Open Anyway next to the line about tock, and confirm.

tock then asks to set itself up and does what `tock setup` does, including
`tock` in `~/.local/bin`. Opened again later, it shows the dashboard. To
update, drag the new version over the old one and open it.

**macOS, from source:** you need Go 1.26 and the Xcode command line tools
(`xcode-select --install`).

```sh
git clone https://github.com/confuserdotcom/tock && cd tock
make cert          # once: a local code-signing certificate
make cert-trust    # once: approve the macOS dialog that trusts it
make install       # builds, signs, starts trackd, runs tock doctor
```

`make install` puts `tock` in `~/.local/bin` (add it to `PATH`) and does
the rest of what `tock setup` does.

**Linux (Hyprland):** `make install-linux`, or with Homebrew on Linux
the same `brew install` and `tock setup` as on macOS. Without Go, download
`tock-<version>-linux-amd64.tar.gz` (or `arm64`) from
[Releases](https://github.com/confuserdotcom/tock/releases), check it
against `SHA256SUMS`, and run:

```sh
tar xzf tock-v5.2.0-linux-amd64.tar.gz && cd tock-v5.2.0-linux-amd64
install -m 755 trackd tock ~/.local/bin/ && ~/.local/bin/trackd install && tock init
```

**From ActivityWatch:** `tock import activitywatch export.json` brings in
your history and labels it with your rules.

**Removing it:** `tock uninstall` stops trackd and the menu bar and
removes the apps. `--purge` also deletes your data. A Homebrew install
then needs `brew uninstall tock` for the command itself.

## First day

1. Work as usual. `tock` opens the terminal UI. `tock today` prints the day.
   `tock web` opens the dashboard.
2. `tock inbox` lists unlabelled time grouped by app, site or title.
   `tock tag 1 Code/tock` labels the first entry and learns a rule for it.
3. Edit `~/.config/tock/rules.toml` for broader rules, then run
   `tock rules check`.
4. `tock session start 50` starts a 50-minute focus session.

In the terminal UI, `?` lists the keys and `:` opens a command palette.

<img src="docs/readme/tui.webp" width="100%" alt="The tock terminal UI on a demo day: the today tab, labelling a block, the score breakdown, a focus session, the command palette and the inbox.">

## Design

<img src="docs/assets/diagrams/design.svg" width="100%" alt="The tock design system: colour means a kind of time (deep teal, shallow blue, browsing mauve, neutral grey, distracting brick, unlabelled faint grey, away hatched); near-black and white neutrals with ink as the only accent; Inter for everything with tabular figures; 2, 6 and 8 px corners.">

The dashboard, the terminal UI, the menu bar and the notch share one set of
colours: colour only ever means a kind of time. [DESIGN.md](DESIGN.md) has
the tokens, the type scale, the components and the motion rules.

## Documentation

- [confuserdotcom.github.io/tock](https://confuserdotcom.github.io/tock/): the guide and the architecture as a site.
- [Guide](docs/guide.md): every command, file and feature.
- [Architecture](docs/architecture.md): how the pieces fit, with diagrams, and what they cost.
- [CHANGELOG](CHANGELOG.md): what changed in each release.
- [DESIGN.md](DESIGN.md): the design system for the dashboard, TUI and menu.

## Development

```sh
make test                                  # vet, build, all tests
go test ./internal/tui -run Golden -update # regenerate TUI snapshots
TOCK_DEMO_PORT=7788 go test -tags demo -run TestDemo -timeout 0 ./internal/web
                                           # the dashboard on demo data
```

CI runs gofmt, vet, the build without cgo and the tests on Linux, and the
cgo build and tests on macOS. To release, add a `## vX.Y.Z` section to
CHANGELOG.md and push the tag. CI builds the Linux tarballs, the source
tarball and Homebrew formula, and the .dmg, and publishes the release.

## License

MIT. See [LICENSE](LICENSE).
