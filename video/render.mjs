// node render.mjs [id ...]
// Renders each clip to out/<id>.mp4 (1080p60, the launch-video masters) and
// to docs/readme/<id>.webp (a looping animation the README shows inline).
import { bundle } from "@remotion/bundler";
import { renderMedia, selectComposition } from "@remotion/renderer";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, rmSync, statSync } from "node:fs";
import { join } from "node:path";

const here = new URL(".", import.meta.url).pathname;
const out = join(here, "out");
const readme = join(here, "../docs/readme");
mkdirSync(out, { recursive: true });

const serveUrl = await bundle({ entryPoint: join(here, "src/index.ts"), publicDir: join(here, "footage") });
const ids = process.argv.slice(2);
const all = ["today", "edit", "inbox", "week", "trends", "library", "palette", "notch", "launch"];
for (const id of ids.length ? ids : all) {
  if (id === "notch" && !existsSync(join(here, "footage/notch/screen.mp4"))) {
    console.log("notch: no footage yet, run capture/notch.sh");
    continue;
  }
  let comp;
  try {
    comp = await selectComposition({ serveUrl, id });
  } catch {
    console.log(`${id}: no composition, skipped`);
    continue;
  }
  const mp4 = join(out, `${id}.mp4`);
  await renderMedia({ composition: comp, serveUrl, codec: "h264", crf: 16, outputLocation: mp4, concurrency: 6, jpegQuality: 92 });
  if (id !== "launch") {
    const webp = join(readme, `${id}.webp`);
    // This ffmpeg has no webp encoder: frames out, img2webp (libwebp) in.
    const tmp = join(out, `${id}-frames`);
    rmSync(tmp, { recursive: true, force: true });
    mkdirSync(tmp);
    execFileSync("ffmpeg", ["-loglevel", "error", "-i", mp4, "-vf", "fps=24,scale=1200:-1:flags=lanczos", join(tmp, "f%04d.png")]);
    const frames = readdirSync(tmp).sort().map((f) => join(tmp, f));
    execFileSync("img2webp", ["-loop", "0", "-lossy", "-q", "62", "-m", "4", "-d", "42", ...frames, "-o", webp]);
    rmSync(tmp, { recursive: true });
    console.log(`${id}: ${(statSync(mp4).size / 1e6).toFixed(1)} MB mp4, ${(statSync(webp).size / 1e6).toFixed(1)} MB webp`);
  } else console.log(`${id}: ${(statSync(mp4).size / 1e6).toFixed(1)} MB mp4`);
}
