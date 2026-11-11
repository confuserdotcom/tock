# Product

What tock is for, who it is for first, and what it will not do. This page
guides what gets built; the [guide](guide.md) says how things work.

## Vision

A record of working time that is automatic, stays on your machine, and
can be explained line by line, so you can bill from it without
reconstructing your week from memory.

## Positioning

**Automatic, local, explainable time records, for developers first, with
client billing you can trust.**

- *Automatic*: trackd records the app, window and site in front of you.
  There is no timer to forget.
- *Local*: one SQLite file. Nothing leaves the machine unless you turn on
  Claude or Slack, or export ([privacy](privacy.md)).
- *Explainable*: every label is a rule you can read, a label you gave, or
  a local model's guess, and it says which.
- *Billing you can trust*: an issued invoice is a snapshot. Later
  relabelling, rate changes or config edits never change it, and
  `tock invoice check` shows the drift instead.

## Who it is for

Primary:

1. **Developers and builders.** They live in a terminal, edit TOML, use
   git, and some run Hyprland. They install from Homebrew or a tarball and
   read the rules file. This is the audience that will try tock first.
2. **Freelancers who bill by the hour**, often the same people. Open
   source trackers record time but don't produce billing records; tock's
   invoices are numbered, stored and immutable.

Secondary:

3. **Students.** Courses as projects with weekly goals fit the model.
   Paper, phone and tablet study is not captured, and use is seasonal, so
   this is not a launch audience.
4. **Mixed use**: a job plus side projects on one machine. Billing can be
   off and focus features on, or the other way round.

Not targeted: tracking employees. tock records the person who installed
it, for that person.

## How the product adapts

There are no profiles. Three groups can be turned off with `tock choice`:
**Focus** (sessions, the score, break and distraction notices), **Day
routine** (the day's notices) and **Billing** (rates and invoices). A
group that is off does not run and does not show. Everything else is
plain settings. Labels and spans never depend on the groups.

## Non-goals

- A cloud service, accounts or team dashboards.
- Screenshots, keystrokes or anything inside a window (screen text reads
  keywords only, and is off by default).
- Windows, GNOME or KDE for now.
- A full accounting tool: tock issues invoices from tracked time with one
  tax rate; payments, reminders and multi-currency books are elsewhere.
- Rewriting what was issued: a correction is a void plus a new invoice.

## Around it

- **ActivityWatch**: open source and local, with categories by regex. No
  billing records.
- **Timing**: automatic, Mac only, closed source, with billing.
- **Rize**: automatic and polished, cloud-based, no Linux.
- **RescueTime**: automatic, cloud-based.
- **Toggl, Clockify**: billing and teams, built around a timer you start.

tock's place is the overlap none of these covers: automatic and local,
open source, on Mac and Linux, with billing-grade records.
