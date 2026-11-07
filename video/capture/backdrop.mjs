// Holds the showcase dashboard full screen (kiosk) behind the notch while
// capture/notch.sh records. Runs until killed.
import { chromium } from "playwright-core";
import { clockScript } from "./clock.mjs";

const b = await chromium.launch({ channel: "chrome", headless: false, args: ["--kiosk", "--hide-scrollbars"], ignoreDefaultArgs: ["--enable-automation"] });
const ctx = await b.newContext({ viewport: null, colorScheme: "dark", timezoneId: "Africa/Johannesburg" });
await ctx.addInitScript(clockScript(1));
const p = await ctx.newPage();
await p.goto(`http://127.0.0.1:7799/#t=${"0".repeat(64)}`);
await new Promise(() => {});
