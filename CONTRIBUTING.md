# Contributing

Bug reports and small fixes are welcome. For anything bigger, open an issue
first so we can agree on the shape before you write it.

## Feature freeze

tock already does more than its users have asked for, so until ten people
other than the author use it, it grows no wider. Until then:

- **Welcome:** bug fixes, accuracy, speed and memory, install and first-run
  problems, docs, tests, and polish of what is already there.
- **Not taken:** new platforms, new integrations (services tock talks to or
  imports from), new interfaces, and new feature areas. Open an issue
  instead; it is a vote for what comes after the freeze.

A user counts once they have run tock for a week and said so in an issue
or a discussion. The README says when the freeze ends.

## Building

You need Go 1.26. On macOS you also need the Xcode command line tools;
Linux builds without cgo.

```sh
make test          # vet, build, all tests
make install       # macOS: build, sign with the local certificate, start trackd
```

`make install` needs the certificate from `make cert` and `make cert-trust`
(see the README). To try the dashboard without touching your own data:

```sh
TOCK_DEMO_PORT=7788 go test -tags demo -run TestDemo -timeout 0 ./internal/web
```

## Layout

| Path | What it is |
| --- | --- |
| `cmd/trackd` | the recorder daemon |
| `cmd/tock` | the CLI and terminal UI |
| `cmd/tockbar` | the macOS menu bar app |
| `internal/collect` | reading the front app, window and URL (the only cgo) |
| `internal/span`, `classify`, `stats`, `focus`, `report`, `suggest` | pure logic: no I/O, no clock |
| `internal/store` | SQLite; schema changes go in `migrations/NNNN_name.sql` |
| `internal/app` | what the CLI, TUI and web page share |
| `internal/tui`, `internal/web` | the two interfaces |

## Before you send a change

- `gofmt -l .` prints nothing, `go vet ./...` is clean.
- `CGO_ENABLED=0 go build ./...` still works. A `_darwin.go` file needs an
  `_other.go` stub next to it.
- Tests cover the change. The pure packages are easy to test because they
  take the time as an argument (`nowMS`) instead of reading the clock.
- TUI snapshots: `go test ./internal/tui -run Golden -update`, then read the
  diff before committing it.
- Colour only ever means a kind of time; keep new UI to the existing
  palette and type.
- A user-visible change gets a line in the unreleased section at the top of
  [CHANGELOG.md](CHANGELOG.md).
- A schema change is a new numbered file in `internal/store/migrations`.
  It must leave an older trackd able to write: new tables, new columns
  with a `DEFAULT`, new indexes. A newer `tock` migrates while the old
  trackd keeps running, and that trackd goes on with the tables it knows.
  `TestMigrationsAreAdditive` fails on `DROP`, `RENAME` and `ADD COLUMN …
  NOT NULL` without a `DEFAULT`. A change that has to break older writers
  raises `compat` in `schema_meta` (and `store.CompatLevel`), carries the
  comment `-- breaks-older-writers`, and says so in CHANGELOG.md; an older
  trackd then stops and tells the user to update it.
  After each release tag, `scripts/fixture-db.sh vX.Y.Z` writes a database
  with that release's code to `internal/store/testdata/upgrade/`; commit it,
  and `TestUpgradeFixtures` checks every later version still opens it.

## Releases

Add a `## vX.Y.Z` section to CHANGELOG.md and push the tag; CI builds and
publishes the release. Run `make bench` first and check the figures in
[docs/architecture.md](docs/architecture.md) still hold.

## Things that won't be merged

- Network code in `trackd` or `tockbar`. A test fails the build if trackd
  links an HTTP or TLS client.
- Anything that sends activity off the machine without the user turning it
  on, or without going through the privacy filters.
- Telemetry of any kind.

## Reporting bugs

Use the bug report form. Please don't paste real window titles, URLs or
rules into an issue; `tock doctor` output is safe to share.
