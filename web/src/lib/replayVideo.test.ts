import { describe, expect, it } from "vitest";
import type { Row } from "./api";
import { clipTime, drawFrame, pickMime, videoName, withDuration, VIDEO_H, VIDEO_W, type Ctx, type Palette } from "./replayVideo";

const m = 60_000;
const row = (start: number, end: number, app: string, kind: string, idle = false): Row => ({
  ids: [start], start: start * m, end: end * m, key: app, app, text: app + " title",
  category: kind ? "Cat" : "", kind, project: "", source: "", idle, open: false,
});

describe("pickMime", () => {
  it("takes webm first and falls back to mp4", () => {
    expect(pickMime(() => true)).toBe("video/webm;codecs=vp9");
    expect(pickMime((m) => m === "video/webm")).toBe("video/webm");
    expect(pickMime((m) => m === "video/mp4")).toBe("video/mp4");
    expect(pickMime(() => false)).toBe("");
  });
});

describe("videoName", () => {
  it("names the day and the format", () => {
    const d = new Date(2026, 10, 12, 15, 0).getTime();
    expect(videoName(d, "video/webm;codecs=vp9")).toBe("tock-replay-2026-11-12.webm");
    expect(videoName(d, "video/mp4")).toBe("tock-replay-2026-11-12.mp4");
  });
});

describe("clipTime", () => {
  it("maps real time onto the day and stops at the end", () => {
    expect(clipTime(0, 20, 1000, 3000)).toBe(1000);
    expect(clipTime(10_000, 20, 1000, 3000)).toBe(2000);
    expect(clipTime(25_000, 20, 1000, 3000)).toBe(3000);
    expect(clipTime(-5, 20, 1000, 3000)).toBe(1000);
  });
});

/** stub records what drawFrame paints. */
function stub() {
  const texts: string[] = [];
  const rects: { x: number; y: number; w: number; h: number; fill: string; alpha: number }[] = [];
  const ctx: Ctx = {
    fillStyle: "", font: "", globalAlpha: 1, textAlign: "left", textBaseline: "alphabetic",
    fillRect(x, y, w, h) { rects.push({ x, y, w, h, fill: String(this.fillStyle), alpha: this.globalAlpha }); },
    fillText(t) { texts.push(t); },
    measureText: (t: string) => ({ width: t.length * 10 }) as TextMetrics,
    save() {}, restore() {},
  };
  return { ctx, texts, rects };
}

const pal: Palette = {
  bg: "bg", fg: "fg", muted: "muted", faint: "faint", track: "track",
  kinds: { deep: "deep", shallow: "shallow", browse: "browse", neutral: "neutral", distract: "distract", "": "none" },
};

describe("drawFrame", () => {
  const rows = [row(0, 30, "Code", "deep"), row(30, 40, "Idle", "", true), row(40, 60, "YouTube", "distract")];

  it("paints the background, the app under the playhead and the totals", () => {
    const { ctx, texts, rects } = stub();
    drawFrame(ctx, rows, 0, 60 * m, 45 * m, pal, "Thursday 12 November");
    expect(rects[0]).toMatchObject({ x: 0, y: 0, w: VIDEO_W, h: VIDEO_H, fill: "bg" });
    expect(texts).toContain("Thursday 12 November");
    expect(texts).toContain("YouTube");
    expect(texts).toContain("WORKED");
    expect(texts).toContain("35m"); // 30m of Code + 5m of YouTube
    // Every row is in the strip; the playhead is drawn after them.
    const strip = rects.filter((r) => r.y === 448);
    expect(strip.map((r) => r.fill)).toEqual(["track", "deep", "faint", "distract"]);
    expect(rects[rects.length - 1].fill).toBe("fg");
  });

  it("fades rows still to come and says when nothing is recorded", () => {
    const { ctx, texts, rects } = stub();
    drawFrame(ctx, rows.slice(2), 0, 60 * m, 10 * m, pal, "t");
    expect(texts).toContain("Nothing recorded");
    expect(rects.find((r) => r.fill === "distract")?.alpha).toBe(0.25);
  });

  it("cuts a long app name to the room it has", () => {
    const { ctx, texts } = stub();
    drawFrame(ctx, [row(0, 60, "A".repeat(200), "deep")], 0, 60 * m, 30 * m, pal, "t");
    const name = texts.find((t) => t.startsWith("AAA"))!;
    expect(name.endsWith("…")).toBe(true);
    expect(name.length * 10).toBeLessThanOrEqual(VIDEO_W);
  });
});

describe("withDuration", () => {
  // EBML header, then a Segment of unknown size holding Info (TimecodeScale
  // only) and a Cluster stub, as MediaRecorder writes them.
  const webm = () => new Uint8Array([
    0x1a, 0x45, 0xdf, 0xa3, 0x84, 0x42, 0x86, 0x81, 0x01,
    0x18, 0x53, 0x80, 0x67, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
    0x15, 0x49, 0xa9, 0x66, 0x87, 0x2a, 0xd7, 0xb1, 0x83, 0x0f, 0x42, 0x40,
    0x1f, 0x43, 0xb6, 0x75, 0x81, 0x00,
  ]);
  it("writes the duration into Info", () => {
    const out = withDuration(webm(), 21_500);
    expect(out.length).toBe(webm().length + 11 + 7);
    const info = 21;
    expect([...out.subarray(info, info + 4)]).toEqual([0x15, 0x49, 0xa9, 0x66]);
    expect(out[info + 4]).toBe(0x01); // an 8-byte size
    expect(out[info + 11]).toBe(7 + 11);
    const dur = info + 12 + 7;
    expect([...out.subarray(dur, dur + 3)]).toEqual([0x44, 0x89, 0x88]);
    expect(new DataView(out.buffer).getFloat64(dur + 3)).toBe(21_500);
    expect([...out.subarray(out.length - 6)]).toEqual([0x1f, 0x43, 0xb6, 0x75, 0x81, 0x00]);
  });
  it("leaves a file that has one, or is not webm, alone", () => {
    const once = withDuration(webm(), 1000);
    expect(withDuration(once, 2000)).toBe(once);
    const junk = new Uint8Array([1, 2, 3]);
    expect(withDuration(junk, 1000)).toBe(junk);
  });
});
