// Replay as a video: the browser paints the day frame by frame onto a canvas
// and records it with MediaRecorder. No server work, no extra dependency.
import type { Row } from "./api";
import { dur, hhmm } from "./format";
import { kindLabel } from "./kinds";
import { frame } from "./replay";

export const VIDEO_W = 1280;
export const VIDEO_H = 720;
export const VIDEO_FPS = 30;
/** CLIP_S is the clip's length whatever the speed picker says: a long day
 *  at 1m/s would otherwise be hours of video. */
export const CLIP_S = 20;
/** HOLD_MS is how long the last frame stays before the clip ends. */
const HOLD_MS = 1500;

/** MIMES are the recorder formats tried in order; WKWebView before Safari
 *  18.4 records mp4 only. */
const MIMES = ["video/webm;codecs=vp9", "video/webm;codecs=vp8", "video/webm", "video/mp4"];

/** pickMime is the first format the recorder takes, or "" for none. */
export function pickMime(supported: (m: string) => boolean): string {
  return MIMES.find((m) => supported(m)) ?? "";
}

/** canRecord reports whether this browser can record a canvas at all. */
export function canRecord(): boolean {
  return typeof MediaRecorder !== "undefined" && typeof HTMLCanvasElement !== "undefined" &&
    typeof HTMLCanvasElement.prototype.captureStream === "function" && pickMime((m) => MediaRecorder.isTypeSupported(m)) !== "";
}

/** videoName is the file a replay of the day holding dayMS saves as. */
export function videoName(dayMS: number, mime: string): string {
  const d = new Date(dayMS);
  const p = (n: number) => String(n).padStart(2, "0");
  const ext = mime.startsWith("video/mp4") ? "mp4" : "webm";
  return `tock-replay-${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}.${ext}`;
}

/** clipTime is the replay time shown elapsedMS into a clip of seconds that
 *  plays from to to: real time drives it, so a dropped frame never
 *  stretches the clip. */
export function clipTime(elapsedMS: number, seconds: number, from: number, to: number): number {
  return from + Math.min(Math.max(elapsedMS / (seconds * 1000), 0), 1) * (to - from);
}

/** Palette is the clip's colours, read from the dashboard's theme. */
export type Palette = { bg: string; fg: string; muted: string; faint: string; track: string; kinds: Record<string, string> };

/** readPalette turns the theme's CSS variables ("172 36% 48%") into colours. */
export function readPalette(el: Element = document.documentElement): Palette {
  const cs = getComputedStyle(el);
  const v = (name: string, alpha = 1) => {
    const raw = cs.getPropertyValue(name).trim();
    return raw ? `hsl(${raw} / ${alpha})` : "";
  };
  const kinds: Record<string, string> = {};
  for (const k of ["deep", "shallow", "browse", "neutral", "distract"]) kinds[k] = v(`--k-${k}`) || "#7d8088";
  kinds[""] = v("--faint", 0.55) || "#5d5f66";
  return {
    bg: v("--background") || "#0f1011", fg: v("--foreground") || "#e6e6e8",
    muted: v("--muted-foreground") || "#8b8d93", faint: v("--faint") || "#5d5f66",
    track: v("--faint", 0.18) || "rgba(93,95,102,0.18)", kinds,
  };
}

/** Ctx is the slice of CanvasRenderingContext2D drawFrame uses. */
export type Ctx = Pick<CanvasRenderingContext2D,
  "fillRect" | "fillText" | "measureText" | "save" | "restore"> & {
  fillStyle: CanvasRenderingContext2D["fillStyle"];
  font: string; globalAlpha: number; textAlign: CanvasTextAlign; textBaseline: CanvasTextBaseline;
};

const FONT = "Inter, -apple-system, system-ui, sans-serif";
const PAD = 80;

/** fit cuts s with an ellipsis to fit width at the context's font. */
function fit(ctx: Ctx, s: string, width: number): string {
  if (ctx.measureText(s).width <= width) return s;
  let lo = 0, hi = s.length;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (ctx.measureText(s.slice(0, mid) + "…").width <= width) lo = mid;
    else hi = mid - 1;
  }
  return s.slice(0, lo) + "…";
}

/** drawFrame paints the replay at at onto a VIDEO_W x VIDEO_H context:
 *  the clock, the app under the playhead, the day's strip with the playhead,
 *  the running totals and the last few apps. rows must be ordered. */
export function drawFrame(ctx: Ctx, rows: Row[], from: number, to: number, at: number, pal: Palette, title: string) {
  const f = frame(rows, at, 4);
  ctx.save();
  ctx.globalAlpha = 1;
  ctx.fillStyle = pal.bg;
  ctx.fillRect(0, 0, VIDEO_W, VIDEO_H);
  ctx.textBaseline = "alphabetic";
  ctx.textAlign = "left";

  ctx.font = `500 22px ${FONT}`;
  ctx.fillStyle = pal.muted;
  ctx.fillText(title, PAD, 100);
  ctx.textAlign = "right";
  ctx.fillStyle = pal.faint;
  ctx.fillText("tock replay", VIDEO_W - PAD, 100);
  ctx.textAlign = "left";

  ctx.font = `600 132px ${FONT}`;
  ctx.fillStyle = pal.fg;
  ctx.fillText(hhmm(at), PAD - 6, 290);

  // The app under the playhead.
  const x = 520, w = VIDEO_W - PAD - x;
  const now = f.now;
  ctx.fillStyle = now && !now.idle ? pal.kinds[now.kind] ?? pal.kinds.neutral : pal.track;
  ctx.fillRect(x, 186, 18, 18);
  ctx.font = `600 44px ${FONT}`;
  ctx.fillStyle = pal.fg;
  ctx.fillText(fit(ctx, now ? (now.idle ? "Away" : now.app) : "Nothing recorded", w - 34), x + 34, 206);
  if (now && !now.idle) {
    ctx.font = `400 24px ${FONT}`;
    ctx.fillStyle = pal.muted;
    ctx.fillText(fit(ctx, [now.category || kindLabel(now.kind), now.project, now.text].filter(Boolean).join(" · "), w - 34), x + 34, 246);
  }
  // The apps before it, newest last.
  ctx.font = `400 20px ${FONT}`;
  f.trail.forEach((r, i) => {
    const y = 296 + i * 30;
    ctx.globalAlpha = 0.35 + (0.65 * (i + 1)) / f.trail.length;
    ctx.fillStyle = pal.faint;
    ctx.fillText(hhmm(r.start), x + 34, y);
    ctx.fillStyle = pal.kinds[r.kind] ?? pal.kinds.neutral;
    ctx.fillRect(x + 104, y - 13, 12, 12);
    ctx.fillStyle = pal.fg;
    ctx.fillText(fit(ctx, r.app, w - 260), x + 128, y);
    ctx.textAlign = "right";
    ctx.fillStyle = pal.muted;
    ctx.fillText(dur(r.end - r.start), VIDEO_W - PAD, y);
    ctx.textAlign = "left";
  });
  ctx.globalAlpha = 1;

  // The strip: the day's rows, the ones still to come faded.
  const sx = PAD, sw = VIDEO_W - 2 * PAD, sy = 448, sh = 44;
  const span = Math.max(to - from, 1);
  const px = (t: number) => sx + ((Math.min(Math.max(t, from), to) - from) / span) * sw;
  ctx.fillStyle = pal.track;
  ctx.fillRect(sx, sy, sw, sh);
  for (const r of rows) {
    const a = px(r.start), b = px(r.end);
    if (b - a <= 0) continue;
    ctx.globalAlpha = (r.start <= at ? 1 : 0.25) * (r.idle ? 0.35 : 1);
    ctx.fillStyle = r.idle ? pal.faint : pal.kinds[r.kind] ?? pal.kinds.neutral;
    ctx.fillRect(a, sy, Math.max(b - a, 1), sh);
  }
  ctx.globalAlpha = 1;
  ctx.fillStyle = pal.fg;
  ctx.fillRect(Math.round(px(at)) - 1, sy - 8, 2, sh + 16);
  ctx.font = `400 18px ${FONT}`;
  ctx.fillStyle = pal.faint;
  ctx.fillText(hhmm(from), sx, sy + sh + 32);
  ctx.textAlign = "right";
  ctx.fillText(hhmm(to), sx + sw, sy + sh + 32);
  ctx.textAlign = "left";

  // The running totals.
  const stats: [string, string][] = [
    ["Worked", dur(f.workedMS)], ["Deep", dur(f.deepMS)], ["Distracting", dur(f.distractMS)],
    ["Away", dur(f.awayMS)], ["Switches", String(f.switches)], ["Longest run", dur(f.longestMS)],
  ];
  const cw = sw / stats.length;
  stats.forEach(([k, v], i) => {
    ctx.font = `500 16px ${FONT}`;
    ctx.fillStyle = pal.faint;
    ctx.fillText(k.toUpperCase(), sx + i * cw, 612);
    ctx.font = `600 34px ${FONT}`;
    ctx.fillStyle = pal.fg;
    ctx.fillText(v, sx + i * cw, 654);
  });
  ctx.restore();
}

export type RecordOptions = {
  seconds?: number;
  title: string;
  palette: Palette;
  onProgress?: (share: number) => void;
  signal?: AbortSignal;
};

/** recordReplay records rows played from from to to as a clip of
 *  opts.seconds and resolves the video. It takes as long as the clip. */
export async function recordReplay(rows: Row[], from: number, to: number, opts: RecordOptions): Promise<{ blob: Blob; mime: string }> {
  const mime = pickMime((m) => MediaRecorder.isTypeSupported(m));
  if (!mime) throw new Error("this browser cannot record video");
  const seconds = opts.seconds ?? CLIP_S;
  await document.fonts?.ready;
  const canvas = document.createElement("canvas");
  canvas.width = VIDEO_W;
  canvas.height = VIDEO_H;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no canvas");
  drawFrame(ctx, rows, from, to, from, opts.palette, opts.title);
  const stream = canvas.captureStream(VIDEO_FPS);
  const rec = new MediaRecorder(stream, { mimeType: mime, videoBitsPerSecond: 5_000_000 });
  const chunks: Blob[] = [];
  rec.ondataavailable = (e) => { if (e.data.size) chunks.push(e.data); };
  const total = seconds * 1000 + HOLD_MS;
  return new Promise((resolve, reject) => {
    let timer = 0;
    let aborted = false;
    const finish = () => {
      clearTimeout(timer);
      stream.getTracks().forEach((t) => t.stop());
    };
    rec.onstop = async () => {
      finish();
      if (aborted) {
        reject(new DOMException("cancelled", "AbortError"));
        return;
      }
      const type = mime.split(";")[0];
      let blob = new Blob(chunks, { type });
      if (type === "video/webm") {
        blob = new Blob([withDuration(new Uint8Array(await blob.arrayBuffer()), total) as BlobPart], { type });
      }
      resolve({ blob, mime });
    };
    rec.onerror = () => {
      finish();
      reject(new Error("recording failed"));
    };
    opts.signal?.addEventListener("abort", () => {
      aborted = true;
      if (rec.state !== "inactive") rec.stop();
    }, { once: true });
    rec.start(250);
    const t0 = performance.now();
    const tick = () => {
      const el = performance.now() - t0;
      drawFrame(ctx, rows, from, to, clipTime(el, seconds, from, to), opts.palette, opts.title);
      opts.onProgress?.(Math.min(el / total, 1));
      if (el >= total) {
        if (rec.state !== "inactive") rec.stop();
        return;
      }
      timer = window.setTimeout(tick, 1000 / VIDEO_FPS);
    };
    tick();
  });
}

// EBML ids MediaRecorder's webm needs patched: it streams, so it never
// writes the clip's Duration and players cannot seek.
const ID_SEGMENT = 0x18538067, ID_INFO = 0x1549a966, ID_DURATION = 0x4489;

/** readVint reads an EBML variable-size integer at i: its value (marker bit
 *  kept for ids, dropped for sizes) and length; unknown sizes are -1. */
function readVint(b: Uint8Array, i: number, id: boolean): { v: number; n: number } {
  const first = b[i];
  let n = 1;
  while (n <= 8 && !(first & (0x80 >> (n - 1)))) n++;
  if (n > 8 || i + n > b.length) return { v: -1, n: 1 };
  let v = id ? first : first & (0xff >> n);
  let all = v === (0xff >> n);
  for (let k = 1; k < n; k++) {
    v = v * 256 + b[i + k];
    if (b[i + k] !== 0xff) all = false;
  }
  return { v: !id && all ? -1 : v, n };
}

/** withDuration returns the webm with a Duration of ms written into its
 *  Info, or the input untouched when it has one or is not laid out as
 *  expected. */
export function withDuration(b: Uint8Array, ms: number): Uint8Array {
  let i = 0;
  const head = readVint(b, 0, true);
  if (head.v !== 0x1a45dfa3) return b;
  const hs = readVint(b, head.n, false);
  i = head.n + hs.n + hs.v;
  const seg = readVint(b, i, true);
  if (seg.v !== ID_SEGMENT) return b;
  const segSize = readVint(b, i + seg.n, false);
  const segSizeAt = i + seg.n;
  i += seg.n + segSize.n;
  while (i < b.length) {
    const id = readVint(b, i, true);
    const sz = readVint(b, i + id.n, false);
    if (sz.v < 0) return b;
    if (id.v !== ID_INFO) {
      i += id.n + sz.n + sz.v;
      continue;
    }
    const body = i + id.n + sz.n, end = body + sz.v;
    for (let j = body; j < end;) {
      const cid = readVint(b, j, true), cs = readVint(b, j + cid.n, false);
      if (cid.v === ID_DURATION) return b;
      j += cid.n + cs.n + Math.max(cs.v, 0);
    }
    // Duration: id 0x4489, size 8, a big-endian float64 in timecode units
    // (the default scale, a millisecond).
    const dur = new Uint8Array(11);
    dur.set([0x44, 0x89, 0x88]);
    new DataView(dur.buffer).setFloat64(3, ms);
    const size = sz.v + dur.length;
    const sizeBytes = new Uint8Array(8);
    sizeBytes[0] = 0x01;
    for (let k = 7, s = size; k > 0; k--, s = Math.floor(s / 256)) sizeBytes[k] = s % 256;
    const out = new Uint8Array(b.length + dur.length + 8 - sz.n);
    out.set(b.subarray(0, i + id.n));
    out.set(sizeBytes, i + id.n);
    out.set(b.subarray(body, end), i + id.n + 8);
    out.set(dur, i + id.n + 8 + sz.v);
    out.set(b.subarray(end), i + id.n + 8 + sz.v + dur.length);
    // A Segment of known size grows by what was added.
    if (segSize.v >= 0) {
      const grown = segSize.v + out.length - b.length;
      for (let k = segSize.n - 1, s = grown; k >= 0; k--, s = Math.floor(s / 256)) {
        out[segSizeAt + k] = k === 0 ? (0x80 >> (segSize.n - 1)) | (s % 256) : s % 256;
      }
    }
    return out;
  }
  return b;
}
