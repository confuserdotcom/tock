// Holds the showcase dashboard full screen (kiosk) behind the notch while
// capture/notch.sh records. BACKDROP_AT="x,y" (top-left, global points)
// picks the display. Runs until killed.
import { chromium } from "playwright-core";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { clockScript } from "./clock.mjs";

const [x, y] = (process.env.BACKDROP_AT || "0,0").split(",").map(Number);
// A persistent context opens Chrome's first window itself, so --kiosk and
// --window-position apply to it (a window made over CDP ignores them).
const ctx = await chromium.launchPersistentContext(mkdtempSync(join(tmpdir(), "tock-backdrop-")), {
  channel: "chrome",
  headless: false,
  viewport: null,
  colorScheme: "dark",
  timezoneId: "Africa/Johannesburg",
  args: ["--kiosk", "--hide-scrollbars", `--window-position=${x + 40},${y + 60}`],
  ignoreDefaultArgs: ["--enable-automation"],
});
await ctx.addInitScript(clockScript(1));
const p = ctx.pages()[0] ?? (await ctx.newPage());
await p.goto(`http://127.0.0.1:7799/#t=${"0".repeat(64)}`);
await new Promise(() => {});
