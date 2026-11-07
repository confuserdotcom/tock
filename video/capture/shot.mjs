// node capture/shot.mjs <hash> <out.png> [dark]
import { chromium } from "playwright-core";
import { clockScript } from "./clock.mjs";
const [hash, out, dark] = process.argv.slice(2);
const b = await chromium.launch({ channel: "chrome" });
const p = await b.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, timezoneId: "Africa/Johannesburg", colorScheme: dark ? "dark" : "light" });
await p.addInitScript(clockScript(1));
await p.goto(`http://127.0.0.1:7799/#t=${"0".repeat(64)}`);
await p.waitForTimeout(800);
if (hash) await p.evaluate((h) => (location.hash = h), hash);
await p.waitForTimeout(2500);
await p.screenshot({ path: out });
await b.close();
