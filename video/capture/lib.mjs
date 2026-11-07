// Records the dashboard as the README and launch videos use it: a CDP
// screencast at 2x, a drawn cursor (headless Chrome has none), and an event
// log (time + box of each thing touched) that the edit aims its zooms at.
import { chromium } from "playwright-core";
import { mkdirSync, writeFileSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { clockScript } from "./clock.mjs";

export const W = 1440, H = 900;
// SLOW is how much slower the page runs while recording; see clock.mjs.
export const SLOW = 6;
const ROOT = new URL("../footage/", import.meta.url).pathname;
const TOKEN = "0".repeat(64);

const cursorScript = `(() => {
  const add = () => {
    if (document.getElementById("__cur")) return;
    const c = document.createElement("div");
    c.id = "__cur";
    c.innerHTML = '<svg width="22" height="26" viewBox="0 0 22 26"><path d="M2 2 L2 21 L7 16.5 L10.5 24 L13.6 22.6 L10.2 15.3 L17 15.3 Z" fill="#111" stroke="#fff" stroke-width="1.6" stroke-linejoin="round"/></svg>';
    Object.assign(c.style, { position: "fixed", left: "0", top: "0", zIndex: 2147483647, pointerEvents: "none",
      transform: "translate(-100px,-100px)", transition: "scale .12s ease-out", transformOrigin: "2px 2px", filter: "drop-shadow(0 1px 2px rgba(0,0,0,.25))" });
    const ring = document.createElement("div");
    Object.assign(ring.style, { position: "fixed", left: "0", top: "0", width: "34px", height: "34px", marginLeft: "-17px", marginTop: "-17px",
      borderRadius: "50%", border: "2px solid rgba(17,17,17,.35)", zIndex: 2147483646, pointerEvents: "none", opacity: "0" });
    document.documentElement.append(ring, c);
    addEventListener("mousemove", (e) => { c.style.transform = "translate(" + e.clientX + "px," + e.clientY + "px)"; }, true);
    addEventListener("mousedown", (e) => {
      c.style.scale = "0.85";
      ring.style.transition = "none"; ring.style.opacity = "1"; ring.style.transform = "translate(" + e.clientX + "px," + e.clientY + "px) scale(.4)";
      requestAnimationFrame(() => { ring.style.transition = "transform .45s cubic-bezier(.2,.8,.2,1), opacity .45s"; ring.style.opacity = "0"; ring.style.transform = "translate(" + e.clientX + "px," + e.clientY + "px) scale(1.4)"; });
    }, true);
    addEventListener("mouseup", () => { c.style.scale = "1"; }, true);
  };
  if (document.readyState === "loading") addEventListener("DOMContentLoaded", add); else add();
})();`;

const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2);

export async function record(name, { dark = false } = {}, scene) {
  const dir = join(ROOT, name);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(join(dir, "frames"), { recursive: true });
  const browser = await chromium.launch({ channel: "chrome" });
  const page = await browser.newPage({ viewport: { width: W, height: H }, deviceScaleFactor: 2, timezoneId: "Africa/Johannesburg", colorScheme: dark ? "dark" : "light" });
  await page.addInitScript(clockScript(SLOW));
  await page.addInitScript(cursorScript);
  await page.goto(`http://127.0.0.1:7799/#t=${TOKEN}`);
  await page.waitForTimeout(1200);

  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Animation.enable");
  await cdp.send("Animation.setPlaybackRate", { playbackRate: 1 / SLOW });
  const frames = [];
  let t0 = 0;
  cdp.on("Page.screencastFrame", async (f) => {
    const ts = f.metadata.timestamp;
    if (!t0) t0 = ts;
    const file = `f${String(frames.length).padStart(5, "0")}.jpg`;
    writeFileSync(join(dir, "frames", file), Buffer.from(f.data, "base64"));
    frames.push({ file, t: (ts - t0) / SLOW });
    await cdp.send("Page.screencastFrameAck", { sessionId: f.sessionId }).catch(() => {});
  });
  const events = [];
  let mouse = { x: W / 2, y: H + 40 };
  const start = Date.now();
  const now = () => (Date.now() - start) / 1000 / SLOW;

  const r = {
    page,
    wait: (ms) => page.waitForTimeout(ms * SLOW),
    /** mark logs a moment, with a box when given a locator, for the edit. */
    async mark(label, target) {
      let box = null;
      if (target) box = await target.boundingBox();
      events.push({ t: now(), label, box });
    },
    async moveTo(x, y, ms = 700) {
      const from = { ...mouse };
      const steps = Math.max(8, Math.round(ms / 8));
      for (let i = 1; i <= steps; i++) {
        const k = ease(i / steps);
        await page.mouse.move(from.x + (x - from.x) * k, from.y + (y - from.y) * k);
        await page.waitForTimeout((ms * SLOW) / steps);
      }
      mouse = { x, y };
    },
    async hover(loc, label, ms) {
      const b = await loc.boundingBox();
      await r.mark(label ?? "hover", loc);
      await r.moveTo(b.x + b.width / 2, b.y + b.height / 2, ms);
    },
    async click(loc, label, ms) {
      await r.hover(loc, label ?? "click", ms);
      await page.waitForTimeout(120 * SLOW);
      await page.mouse.down();
      await page.waitForTimeout(90 * SLOW);
      await page.mouse.up();
    },
    async type(text, delay = 70) {
      await page.keyboard.type(text, { delay: delay * SLOW });
    },
    async drag(x1, y1, x2, y2, ms = 900) {
      await r.moveTo(x1, y1);
      await page.mouse.down();
      await r.moveTo(x2, y2, ms);
      await page.mouse.up();
    },
  };
  await page.mouse.move(mouse.x, mouse.y);
  // Show the first frame of the scene before recording, so the clip opens settled.
  if (scene.setup) await scene.setup(r);
  await cdp.send("Page.startScreencast", { format: "jpeg", quality: 92, everyNthFrame: 1 });
  const rec0 = Date.now();
  events.push({ t: 0, label: "start", box: null });
  await r.wait(300);
  try {
    await scene.run(r);
  } catch (e) {
    await page.screenshot({ path: join(dir, "fail.png") });
    await browser.close();
    throw e;
  }
  await r.wait(600);
  await cdp.send("Page.stopScreencast");
  const total = (Date.now() - rec0) / 1000 / SLOW;
  await browser.close();

  // Constant 60 fps from the change-driven frames.
  const lines = [];
  frames.forEach((f, i) => {
    const next = i + 1 < frames.length ? frames[i + 1].t : total;
    lines.push(`file 'frames/${f.file}'`, `duration ${Math.max(0.001, next - f.t).toFixed(4)}`);
  });
  writeFileSync(join(dir, "list.txt"), lines.join("\n"));
  execFileSync("ffmpeg", ["-y", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", join(dir, "list.txt"),
    "-t", total.toFixed(3), "-vf", "fps=60,scale=2880:1800:flags=lanczos,format=yuv420p", "-c:v", "libx264", "-crf", "14", "-preset", "slow", join(dir, "raw.mp4")]);
  // Events are in page-time since launch; shift to clip time.
  const shift = (rec0 - start) / 1000 / SLOW;
  writeFileSync(join(dir, "events.json"), JSON.stringify({ duration: total, viewport: { w: W, h: H }, events: events.map((e) => ({ ...e, t: Math.max(0, e.t - shift) })) }, null, 1));
  rmSync(join(dir, "frames"), { recursive: true });
  console.log(`${name}: ${total.toFixed(1)} s, ${frames.length} frames, last frame at ${frames.at(-1).t.toFixed(2)}`);
}
