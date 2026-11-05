---
# gstack: design-md-format=spec
name: tock
description: A ledger of your working day. White page, plain type, numbers that line up, colour only for the kind of time.
colors:
  light-bg: "#ffffff"
  light-side: "#f7f7f8"
  light-sunk: "#f1f1f3"
  light-line: "#ececec"
  light-line-strong: "#d8d8d8"
  light-ink: "#1b1f24"
  light-text: "#2b3038"
  light-muted: "#5b636e"
  light-faint: "#6b7280"
  dark-bg: "#111315"
  dark-side: "#16181b"
  dark-sunk: "#1d2024"
  dark-line: "#25282d"
  dark-line-strong: "#33373d"
  dark-ink: "#eceef1"
  dark-text: "#d5d8dd"
  dark-muted: "#9aa1ab"
  dark-faint: "#868d97"
  focus: "#2f6fdb"
  good-light: "#2e7d4f"
  good-dark: "#5cc38a"
  bad-light: "#b3423f"
  bad-dark: "#e0726f"
  deep-light: "#2f6f8f"
  deep-dark: "#5fa3c4"
  shallow-light: "#7d8fa8"
  shallow-dark: "#8fa1ba"
  browse-light: "#8a6d9e"
  browse-dark: "#b39cc6"
  neutral-light: "#a3a8b0"
  neutral-dark: "#6b7079"
  distract-light: "#c0504d"
  distract-dark: "#e0726f"
  curtain: "#0d0b09"
  curtain-track: "#2b241b"
  curtain-text: "#f1e4cf"
  curtain-muted: "#8a7d68"
typography:
  title:
    fontFamily: IBM Plex Sans
    fontWeight: 600
    fontSize: 1.25rem
  body:
    fontFamily: IBM Plex Sans
    fontSize: 0.8125rem
    lineHeight: 1.45
  label:
    fontFamily: IBM Plex Sans
    fontWeight: 500
    fontSize: 0.6875rem
  number:
    fontFamily: IBM Plex Mono
    fontWeight: 500
    fontFeature: tnum
rounded:
  sm: 2px
  md: 6px
  lg: 8px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 32px
components:
  button-primary:
    backgroundColor: "{colors.light-ink}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.md}"
  button-secondary:
    borderColor: "{colors.light-line-strong}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.md}"
  nav-active:
    backgroundColor: "{colors.light-sunk}"
    textColor: "{colors.light-ink}"
---

# tock

## Overview

**North star:** a ledger. tock writes your day down; the page reads like a well-kept account book. Facts in rows, numbers aligned, totals at the bottom, nothing performed.
**Product context:** a local time tracker for one person on a Mac (and a headless Linux box): a daemon, a menu bar app, a terminal UI and a local web page. No accounts, no cloud, nothing loaded from the internet.
**References:** Timing.app (rows per category on a time axis), Linear (density, hairlines), plain accounting tables.
**Key characteristics:**
- No hero number. The score is one figure in a row of figures, each with its change against your usual day.
- Time runs left to right on one shared axis; each category gets its own row.
- Lists are tables: name left, time and share right-aligned in mono, a bold totals row.
- Colour means a kind of time. Everything else is grey.

## Colors

**Strategy:** neutral greys, one ink, kind colours as data. No accent colour.
**Light or dark:** follows the system; both are first-class. Light is the default in screenshots.

- Ink is the only "accent": the selected day, the primary button, the active nav item (on a sunk grey).
- Kind colours are data, never decoration: deep (blue-teal), shallow (slate), browsing (mauve), neutral (grey), distracting (brick). Unlabelled is grey hatching. The same colours on every surface, a lighter set in dark mode.
- Good and bad are for changes against the usual day, and follow meaning, not direction: less distraction is good.
- `focus` is only the keyboard focus ring.
- Text: ink for names and numbers, muted for secondary, faint (still 4.5:1) for axes and captions.
- The break screen keeps its fixed curtain colours (always dark).

## Typography

- IBM Plex Sans for all text: 13px body, 11px labels in muted (sentence case, no caps), 20px semibold page titles. No serif anywhere.
- IBM Plex Mono for every time, duration, count and percentage, tabular figures, so columns line up.
- Durations in tables read 2:24 (h:mm); in prose and the number row 2h24.
- Both faces are bundled in `internal/web/static/fonts` (OFL, latin subset). The menu bar and break screen use the system fonts; the TUI uses the terminal's.

## Layout

- Web: a 216px sidebar (logo, nav, month calendar, status) and a main column up to 1200px wide.
- Today, in order: title line with the running session; the number row; the category timeline; two tables side by side (categories, apps); then work blocks, goals and the activity log.
- The whole of Today above the activity log fits a 1280x800 window.
- Below 860px the sidebar becomes a top bar and tables stack.
- 24px between sections, 16px inside.

## Elevation & Depth

Flat. Hairline borders group things; no shadows except the tooltip, the label sheet and the toast.

## Shapes

2px on bars in the timeline, 6px on buttons, inputs and grouped boxes, 8px on the sheet.

## Components

- Number row: one bordered strip split by hairlines. Each cell: label, mono value, change vs usual in good or bad (hidden when there is no usual day).
- Category timeline: an hour axis on top, one row per category plus Away, bars in the kind colour, a 1px ink line for now. Hover shows app, title, time and label; click opens the label sheet.
- Tables: header in faint 11px, hairline rows, right-aligned mono time and share, a bold totals row with a stronger rule above.
- Sidebar status: a dot and a line each for trackd, rules and the session.
- Buttons: primary ink, secondary hairline outline, ghost text. Focus ring 2px `focus`.

## Do's and Don'ts

- Do give every number a unit and, where it exists, a comparison.
- Do keep numbers in mono and right-aligned.
- Do use the same kind colours in web, TUI, menu strip and summaries.
- Don't add a big number, a ring, a gauge or a gradient.
- Don't put a card around every section or an icon on every row.
- Don't use colour for anything that is not a kind of time or a good/bad change.
- Don't load anything from the internet, fonts included.

## Motion

- Minimal: 120ms hover, 160ms view change, no counting numbers, no growing bars.
- The label sheet keeps its spring; reduced motion turns everything off.
- The notch panel is the exception: it grows out of the hardware, so its
  shape and content move on springs that start from where they are. Moments
  tock opens: response 0.46 s, damping 0.78. Hover: 0.32 s, 1.0. Closing:
  0.36 s, 1.0, back up into the notch. Content fades in behind the shape.
  Numbers still never count and bars never grow. Reduced motion: the shape
  jumps and content cross-fades in 150 ms.
- The notch is always black, like the hardware, with the dark set of kind
  colours and system fonts, as the menu bar uses.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-11-05 | Initial design system: paper/ink neutrals, brass accent, Instrument Serif + Source Sans 3 + JetBrains Mono | v5 Phase 5d. Unified the web page, TUI and menu with the break screen |
| 2026-11-05 | Replaced by the Ledger system: IBM Plex Sans + Mono, white and grey, ink as the only accent, sidebar, per-category timeline, tables with totals, comparisons with the usual day | The owner picked the Ledger direction from four mockups; the serif-and-brass look read as made by AI |
| 2026-11-05 | The notch panel moves on springs | Owner approved the notch prototype; a surface that grows out of hardware reads as physical only with springs. Kept to the notch |
