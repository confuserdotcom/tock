---
# gstack: design-md-format=spec
name: tock
description: A quiet instrument for your working day. Paper by day, warm ink at night, one brass hand to look at.
colors:
  paper: "#f0f0ec"
  paper-surface: "#f8f8f5"
  paper-sunk: "#e6e6e0"
  paper-line: "#dcdcd5"
  paper-line-strong: "#c6c6be"
  paper-ink: "#1d1c19"
  paper-text: "#34343a"
  paper-muted: "#53575f"
  paper-faint: "#696d75"
  paper-brass: "#8a5e18"
  ink: "#12110e"
  ink-surface: "#1a1915"
  ink-sunk: "#232119"
  ink-line: "#2a2822"
  ink-line-strong: "#3b382f"
  ink-ink: "#f4ecdd"
  ink-text: "#ddd6c8"
  ink-muted: "#9ba1ad"
  ink-faint: "#7e838e"
  brass: "#e3a446"
  brass-fill-paper: "#d9993a"
  rust: "#b5462c"
  curtain: "#0d0b09"
  curtain-track: "#2b241b"
  curtain-text: "#f1e4cf"
  curtain-muted: "#8a7d68"
  deep-paper: "#2b7564"
  deep-ink: "#63bfa8"
  shallow-paper: "#5a78a3"
  shallow-ink: "#8fa9d1"
  browse-paper: "#9a6585"
  browse-ink: "#c993b4"
  distract-paper: "#b5462c"
  distract-ink: "#e57d62"
  neutral-paper: "#858991"
  neutral-ink: "#7d828c"
typography:
  display:
    fontFamily: Instrument Serif
    fontWeight: 400
    fontSize: 5.5rem
    letterSpacing: -0.03em
  title:
    fontFamily: Instrument Serif
    fontWeight: 400
    fontSize: 2.25rem
    letterSpacing: -0.01em
  body:
    fontFamily: Source Sans 3
    fontSize: 1rem
    lineHeight: 1.5
  label:
    fontFamily: Source Sans 3
    fontWeight: 600
    fontSize: 0.9375rem
  mono:
    fontFamily: JetBrains Mono
    fontFeature: tnum
rounded:
  sm: 4px
  md: 6px
  lg: 10px
  full: 9999px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 40px
  2xl: 64px
components:
  button-primary:
    backgroundColor: "{colors.brass-fill-paper}"
    textColor: "{colors.paper-ink}"
    rounded: "{rounded.md}"
  button-secondary:
    borderColor: "{colors.paper-line-strong}"
    textColor: "{colors.paper-ink}"
    rounded: "{rounded.md}"
  tab-active:
    textColor: "{colors.paper-ink}"
    underline: "{colors.paper-brass}"
---

# tock

## Overview

**Creative North Star:** a watchmaker's bench. Plain paper, dark ink, one brass hand. The day is shown like a dial is read: one big number, then the facts, then the detail.
**Product context:** tock is a local time tracker for one person on a Mac (and a headless Linux box). It runs a daemon, a menu bar app, a terminal UI and a local web page. No accounts and no cloud; nothing is loaded from the internet.
**Mode per surface:** web page and TUI are Operate (read the day, label blocks). The break screen is Experience (step away). The menu is Operate at a glance.
**Key characteristics:**
- One large serif number (the focus score) carries the page; everything else is smaller and quieter.
- Sections are separated by space and hairlines, not boxes.
- Brass marks the one thing to look at: the active tab, the "now" line, the main button. Never decoration.
- Activity kinds always use the same six colours on every surface.

## Colors

**Strategy:** restrained. Paper and ink neutrals, slate for secondary text, brass as the only accent, six kind colours as data.
**Light or dark:** follows the system. The owner uses the Mac in the evening, so ink (dark) is the main case; paper must hold up by day.

- `brass` is interaction and "now". On paper it darkens to `paper-brass` for text and lines; fills use `brass-fill-paper` with ink text.
- Kind colours are data, not accents: deep (teal), shallow (slate blue), browsing (mauve), distracted (rust), other (grey). "Not labelled" is grey hatching, not a hue (a lighter grey in the terminal and the menu strip, which cannot hatch). Brass is never a kind.
- Rust (`rust`) means overrun or distraction. Teal means done or met.
- Muted text is slate (cool) on both grounds, which keeps the warm ink from turning brown.
- The break screen keeps its own fixed curtain colours (always dark, whatever the system theme).

## Typography

- Instrument Serif: the score, the break clock and page titles only. Never body text, never italic, never under 2rem.
- Source Sans 3: all text and labels. Section titles are 0.9375rem semibold in ink, not small caps.
- JetBrains Mono: every time, duration and count, with tabular figures.
- The three faces are bundled in `internal/web/static/fonts` (OFL, about 75 KB, latin subset). Other scripts fall back to the system font.
- The menu bar and break screen are AppKit and use the system fonts (SF Pro, SF Mono); the TUI uses the terminal's font.

## Layout

- Web: max width 1120px. On Today, the score and the day's numbers come first, then the timeline across the full width, then a main column (by hour, work blocks, activity) and a narrow side column (categories, goals).
- Below 860px everything stacks in one column with 16px gutters.
- Space between sections is 40px; inside a section 16px.

## Elevation & Depth

Flat. Depth only for things above the page: the label sheet, the tooltip and the toast get an offset soft shadow. No glow.

## Shapes

Small radii: 4px for chips and bars, 6px for buttons and inputs, 10px for the sheet and the menu.

## Components

- Sections: a title row (title left, a quiet note right) over content. No border, no background. A hairline separates groups.
- Buttons: primary brass with ink text; secondary hairline outline; ghost text in brass. Focus ring is 2px brass.
- Tabs: text; the active one is ink and semibold with a 2px brass underline.
- Charts: one style. Flat stacked bars in kind colours, 1px gaps in the surface colour, 4px radius on the outer edge only, a 2px brass line for now, mono tick labels in faint.
- Lists: name, a mono duration, and a 3px bar in the kind colour.

## Do's and Don'ts

- Do keep the score the only big thing on the page.
- Do use the same kind colours in web, TUI, menu strip and summaries.
- Do write numbers in mono with tabular figures.
- Don't put a card around every section, or a card inside a card.
- Don't use brass for data, and don't add a second accent.
- Don't load anything from the internet, fonts included.
- Don't use a coloured left border to mark state; use a tint.

## Motion

- **Approach:** minimal-functional.
- **Easing:** enter ease-out, exit ease-in, move with the existing springs.
- **Duration:** 120ms hovers, 220ms page change, 500ms bars growing on first draw.
- **The one authored moment:** the score counts up to its value with a spring when the day loads.
- Reduced motion turns all of it off.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-11-05 | Initial design system: paper/ink neutrals, brass accent, bundled Instrument Serif + Source Sans 3 + JetBrains Mono | v5 Phase 5d. Unifies the web page, TUI and menu with the brass-and-ink break screen; indigo and violet dropped; the owner picked bundled faces and this palette from a preview |
