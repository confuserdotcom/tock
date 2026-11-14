# tock

[![ci](https://github.com/confuserdotcom/tock/actions/workflows/ci.yml/badge.svg)](https://github.com/confuserdotcom/tock/actions/workflows/ci.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg)](LICENSE)

tock records where your working time goes without a timer. A small daemon
notes the app, window and site in front of you; rules you write in a TOML
file turn that into categories and projects. It runs on macOS and on Linux
under Hyprland, Sway, i3 or X11, with Windows experimental, and everything
stays in one SQLite file on your machine.

<img src="docs/readme/today.webp" width="100%" alt="The tock dashboard for one day: a timeline with one row per category, a focus score, minutes per hour, a donut of the kinds of time, and goals.">

What's in it:

- `trackd`, the recorder. No network code, under 1 % CPU and about 45 MB.
- `tock`, the CLI and a terminal UI, plus a dashboard on 127.0.0.1.
- `tockbar`, a macOS menu bar app that also draws a panel around the notch.
- Rules in `~/.config/tock/rules.toml`. They apply to past days too, so
  fixing a rule fixes the record. Time no rule covers lands in an inbox;
  labelling it there writes a rule for next time.
- A small local model trained on your own labels, for suggestions.
- Rates per project and numbered invoices (`tock invoice issue`) that
  don't change when you relabel later.
- Optional focus sessions, work hours, calendar, sync between machines,
  and Claude suggestions. All of the network ones are off by default.

## Install

macOS, with Homebrew:

```sh
brew tap confuserdotcom/tock
brew trust confuserdotcom/tock
brew install tock
tock setup
```

`tock setup` makes a local code-signing certificate (so the Accessibility
grant survives upgrades), installs and starts trackd and the menu bar app,
and writes starter rules. Run it again after `brew upgrade tock`. trackd
asks for Accessibility on first start; browsers ask for Automation.

There's also a `.dmg` on the [releases page](https://github.com/confuserdotcom/tock/releases).
It isn't notarized (no Apple Developer ID), so the first open needs
System Settings → Privacy & Security → Open Anyway.

From source, with Go 1.26 and the Xcode command line tools:

```sh
git clone https://github.com/confuserdotcom/tock && cd tock
make cert cert-trust   # once
make install
```

Linux (Hyprland, Sway, i3 or X11; see the guide's Linux section): `make install-linux`, or the tarball from the releases
page. [docs/verify.md](docs/verify.md) shows how to check any download
against `SHA256SUMS` and GitHub's signed build record:

```sh
tar xzf tock-v*-linux-amd64.tar.gz && cd tock-v*-linux-amd64
install -m 755 trackd tock ~/.local/bin/ && ~/.local/bin/trackd install && tock init
```

Windows 10 and 11 are experimental, not in a release yet. Build it with
`make build-windows` (binaries land in `build/windows-amd64`), keep
`tock.exe` and `trackd.exe` in one folder, then run `tock init` and
`trackd install`: trackd starts now and whenever you sign in
(`trackd uninstall` undoes it). Bug reports from Windows are especially welcome.

Coming from ActivityWatch: `tock import activitywatch export.json`.

`tock uninstall` removes it again (`--purge` deletes your data too).

## Using it

```sh
tock                      # terminal UI (? for keys)
tock today                # the day as text
tock web                  # the dashboard
tock inbox                # unlabelled time, grouped
tock tag 1 Code/tock      # label it and learn a rule
tock rules check          # after editing rules.toml
tock session start 50     # a 50-minute focus session
```

<img src="docs/readme/tui.webp" width="100%" alt="The tock terminal UI: the today tab, labelling a block, the score breakdown, a focus session, the command palette and the inbox.">

## Docs

- [Guide](docs/guide.md): every command, file and setting.
- [Architecture](docs/architecture.md): how the pieces fit together.
- [Privacy](docs/privacy.md): what is recorded and what can leave the machine.
- [Stability](docs/stability.md): what stays the same between releases, and how well each platform is supported.
- [CHANGELOG](CHANGELOG.md)

The same pages are at [confuserdotcom.github.io/tock](https://confuserdotcom.github.io/tock/).

## Development

```sh
make test                                   # vet, build, tests
go test ./internal/tui -run Golden -update  # regenerate TUI snapshots
make web-check                              # rebuild the dashboard bundle
```

See [CONTRIBUTING.md](CONTRIBUTING.md). tock is in a feature freeze until
ten people besides its author use it: fixes and polish are welcome, new
platforms and integrations wait. If you use it, say so in an issue; that is
what ends the freeze.

## License

MIT
