# Videos

The clips in the README and the masters for a launch video. Everything is
made from real tock running on made-up data, so it can be redone after any
UI change.

```
capture/   records footage: the dashboard (headless Chrome) and the notch (screen)
src/       the edit (Remotion): titles, eased camera, captions, the launch cut
footage/   recordings, not committed
out/       rendered .mp4 masters, not committed
```

## Make them

You need Go, Node and ffmpeg, and `img2webp` (`brew install webp`).

```sh
cd video && npm ci
node capture/scenes.mjs          # every dashboard scene; or name some: today edit
capture/notch.sh                 # the notch; takes 80 s, hands off the Mac
node render.mjs                  # out/<id>.mp4 and ../docs/readme/<id>.webp
npx remotion studio src/index.ts # preview and tweak the edit
```

`node render.mjs launch` renders the whole film to `out/launch.mp4`: an
opening card, each clip with its title, the notch, and a closing card with
the install line.

## How it works

- **Data.** `internal/web/showcase_test.go` (build tag `demo`) serves six
  weeks of a freelancer's month: two clients with hourly rates, goals, a
  meeting, some leaks. Each scene starts it fresh, so edits from one scene
  don't show in the next.
- **Dashboard footage.** `capture/lib.mjs` drives Chrome at 1440×900, 2× pixels,
  with a drawn cursor. The page runs six times slower than real time (its
  clock, timers and CSS animations are all slowed), so the screencast still
  catches every frame of a transition; the frames are then retimed to 60 fps.
  Every hover and click logs its time and the box it touched to
  `events.json`.
- **Camera.** `src/clips.ts` lists each clip's shots: when (seconds, or a
  logged event), what to frame (a logged box, a box in page pixels, or
  `rest`) and the caption. The camera moves between shots on a deep ease and
  zooms in log space, so a push feels straight.
- **Notch footage.** `capture/notch.sh` stops the menu bar app, puts the
  dashboard full screen on the display with the notch, parks the pointer
  there (moments open under it), plays `tockbar notch-demo` and records that
  display, then starts the menu bar app again. `src/Notch.tsx` crops it to the
  notch; `BEATS` there says when each moment opens.
- **Look.** The tock dark palette, Inter, kind colours only where they mean
  a kind of time (DESIGN.md).
