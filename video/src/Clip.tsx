// One README/launch clip: a title, then the recorded dashboard in a window
// with an eased camera that pushes in on what the cursor touches, and
// captions that say what you're looking at.
import React from "react";
import { AbsoluteFill, Easing, Freeze, interpolate, OffthreadVideo, Sequence, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { C, fontFamily, FPS, H, W } from "./theme";
import { DayRing } from "./DayRing";

export type Box = { x: number; y: number; width: number; height: number };
export type Ev = { t: number; label: string; box: Box | null };
export type Events = { duration: number; viewport: { w: number; h: number }; events: Ev[] };

export type Shot = {
  /** when: seconds into the footage, or an event label (its time minus lead). */
  at: number | string;
  /** what to frame: an event label (its box), a box in page pixels, or "rest". */
  frame?: string | Box | "rest";
  /** zoom override (1 = the window fills the frame width). */
  zoom?: number;
  /** offset the framing, in page pixels. */
  nudge?: [number, number];
  caption?: string;
  sub?: string;
  dot?: string;
};

export type ClipDef = {
  id: string;
  footage: string;
  kicker: string;
  title: string;
  shots: Shot[];
  /** trim footage seconds from the start/end. */
  trim?: [number, number];
};

export type ClipProps = { def: ClipDef; ev: Events };

export const INTRO = 1.7; // title, then the window lands
export const OUTRO = 0.9;

// Stage geometry: the 1440x900 page, a title bar above it.
const BAR = 38;
const PW = 1440;
const PH = 900;
const WX = (W - PW) / 2;
const WY = (H - PH - BAR) / 2;
const REST = { cx: W / 2, cy: H / 2, s: 0.9 };
const TRANS = 1.05; // camera move, seconds

export const clipFrames = (def: ClipDef, ev: Events) => {
  const [a, b] = def.trim ?? [0, 0];
  return Math.round((INTRO + ev.duration - a - b + OUTRO) * FPS);
};

type Cam = { cx: number; cy: number; s: number };

function eventOf(ev: Events, label: string): Ev {
  const e = ev.events.find((x) => x.label === label);
  if (!e) throw new Error(`no event ${label}`);
  return e;
}

function camFor(shot: Shot, ev: Events): Cam {
  const f = shot.frame ?? "rest";
  if (f === "rest") return REST;
  const box = typeof f === "string" ? eventOf(ev, f).box : f;
  if (!box) return REST;
  const [nx, ny] = shot.nudge ?? [0, 0];
  const bx = WX + box.x + nx;
  const by = WY + BAR + box.y + ny;
  const s = shot.zoom ?? Math.max(1, Math.min(2.3, (W * 0.74) / box.width, (H * 0.62) / box.height));
  // Keep the view inside the window once it's wider than the frame.
  const hw = W / 2 / s, hh = H / 2 / s;
  const clamp = (v: number, lo: number, hi: number) => (lo > hi ? (lo + hi) / 2 : Math.min(hi, Math.max(lo, v)));
  const cx = clamp(bx + box.width / 2, WX + hw, WX + PW - hw);
  const cy = clamp(by + box.height / 2 + 40 / s, WY + hh, WY + BAR + PH - hh);
  return { cx, cy, s };
}

const ease = Easing.bezier(0.65, 0, 0.35, 1); // a deep easy-ease

/** camera at footage time t, from the shots. */
function cameraAt(t: number, shots: { t: number; cam: Cam }[]): Cam {
  let cam = REST;
  let prev = REST;
  for (const sh of shots) {
    if (t < sh.t) break;
    prev = cam;
    const k = ease(Math.min(1, (t - sh.t) / TRANS));
    // Zoom in log space; move the focus point, so the push feels straight.
    const s = Math.exp(Math.log(prev.s) + (Math.log(sh.cam.s) - Math.log(prev.s)) * k);
    cam = { cx: prev.cx + (sh.cam.cx - prev.cx) * k, cy: prev.cy + (sh.cam.cy - prev.cy) * k, s };
    if (k < 1) return cam;
    cam = sh.cam;
  }
  return cam;
}

export function timeOf(at: number | string, ev: Events): number {
  return typeof at === "number" ? at : Math.max(0, eventOf(ev, at).t - 0.15);
}

export const Clip: React.FC<ClipProps> = ({ def, ev }) => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const [trimA] = def.trim ?? [0, 0];
  const now = frame / FPS;
  const t = Math.max(0, now - INTRO) + trimA; // footage time
  const shots = def.shots.map((s) => ({ t: timeOf(s.at, ev), cam: camFor(s, ev), shot: s })).sort((a, b) => a.t - b.t);
  const outroAt = durationInFrames / FPS - OUTRO;

  let cam = cameraAt(t, shots);
  // Ease back out at the end, so the loop meets its start.
  const back = ease(Math.min(1, Math.max(0, (now - outroAt + 0.35) / (OUTRO + 0.1))));
  cam = { cx: cam.cx + (REST.cx - cam.cx) * back, cy: cam.cy + (REST.cy - cam.cy) * back, s: Math.exp(Math.log(cam.s) * (1 - back) + Math.log(REST.s) * back) };
  // A slow push while holding.
  const land = spring({ frame: frame - Math.round((INTRO - 0.75) * FPS), fps: FPS, config: { damping: 22, mass: 0.9, stiffness: 120 } });
  const enterY = interpolate(land, [0, 1], [180, 0]);
  const enterS = interpolate(land, [0, 1], [0.92, 1]);
  const s = cam.s * enterS;
  const tx = W / 2 - cam.cx * s;
  const ty = H / 2 - cam.cy * s + enterY;
  const fadeOut = interpolate(now, [outroAt + OUTRO * 0.35, outroAt + OUTRO], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });

  return (
    <AbsoluteFill style={{ background: `radial-gradient(120% 90% at 50% 30%, #17181b 0%, ${C.bg} 60%, #060607 100%)`, fontFamily }}>
      <AbsoluteFill style={{ opacity: Math.min(land * 1.3, 1) * fadeOut }}>
        <div style={{ position: "absolute", left: 0, top: 0, transformOrigin: "0 0", transform: `translate(${tx}px, ${ty}px) scale(${s})` }}>
          <Window>
            <Sequence from={Math.round(INTRO * FPS)} layout="none">
              <OffthreadVideo src={staticFile(`${def.footage}/raw.mp4`)} startFrom={Math.round(trimA * FPS)} muted style={{ width: PW, height: PH, display: "block" }} />
            </Sequence>
            {frame < INTRO * FPS && <Poster footage={def.footage} />}
          </Window>
        </div>
      </AbsoluteFill>
      <Title kicker={def.kicker} title={def.title} />
      {shots.map((sh, i) =>
        sh.shot.caption ? (
          <Caption key={i} from={INTRO + sh.t - trimA + 0.35} to={INTRO + (shots[i + 1]?.t ?? Infinity) - trimA + 0.1} end={outroAt + 0.2} text={sh.shot.caption} sub={sh.shot.sub} dot={sh.shot.dot} />
        ) : null,
      )}
      <AbsoluteFill style={{ background: C.bg, opacity: 1 - fadeOut }} />
    </AbsoluteFill>
  );
};

function Poster({ footage }: { footage: string }) {
  // The footage's first frame stands in while the window lands.
  return (
    <Freeze frame={0}>
      <OffthreadVideo src={staticFile(`${footage}/raw.mp4`)} muted style={{ width: PW, height: PH, display: "block", position: "absolute", left: 0, top: 0 }} />
    </Freeze>
  );
}

function Window({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        position: "absolute",
        left: WX,
        top: WY,
        width: PW,
        height: PH + BAR,
        borderRadius: 14,
        overflow: "hidden",
        background: "#0f1011",
        boxShadow: "0 0 0 1px rgba(255,255,255,0.09), 0 40px 120px rgba(0,0,0,0.65), 0 12px 32px rgba(0,0,0,0.5)",
      }}
    >
      <div style={{ height: BAR, display: "flex", alignItems: "center", gap: 8, padding: "0 16px", background: "#151619", borderBottom: "1px solid #222327" }}>
        {["#ff5f57", "#febc2e", "#28c840"].map((c) => (
          <i key={c} style={{ width: 12, height: 12, borderRadius: 6, background: c, opacity: 0.9 }} />
        ))}
        <span style={{ flex: 1, textAlign: "center", color: C.muted, fontSize: 13, fontWeight: 500, marginRight: 60 }}>tock</span>
      </div>
      <div style={{ position: "relative", width: PW, height: PH }}>{children}</div>
    </div>
  );
}

function Title({ kicker, title }: { kicker: string; title: string }) {
  const frame = useCurrentFrame();
  const t = frame / FPS;
  const out = interpolate(t, [INTRO - 0.55, INTRO - 0.1], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: ease });
  if (t > INTRO) return null;
  const words = title.split(" ");
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", flexDirection: "column", gap: 26, opacity: 1 - out, transform: `translateY(${-60 * out}px)`, filter: `blur(${8 * out}px)` }}>
      <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
        <DayRing size={64} body={false} />
        <span style={{ color: C.muted, fontSize: 26, fontWeight: 500, letterSpacing: 0.4, opacity: interpolate(frame, [4, 16], [0, 1], { extrapolateRight: "clamp" }) }}>{kicker}</span>
      </div>
      <div style={{ color: C.ink, fontSize: 76, fontWeight: 600, letterSpacing: -2, display: "flex", gap: 20, flexWrap: "wrap", justifyContent: "center", maxWidth: 1500 }}>
        {words.map((w, i) => {
          const k = spring({ frame: frame - 6 - i * 3, fps: FPS, config: { damping: 18, mass: 0.6 } });
          return (
            <span key={i} style={{ display: "inline-block", opacity: k, transform: `translateY(${(1 - k) * 34}px)`, filter: `blur(${(1 - k) * 10}px)` }}>
              {w}
            </span>
          );
        })}
      </div>
    </AbsoluteFill>
  );
}

function Caption({ from, to, end, text, sub, dot }: { from: number; to: number; end: number; text: string; sub?: string; dot?: string }) {
  const frame = useCurrentFrame();
  const t = frame / FPS;
  const stop = Math.min(to, end);
  if (t < from || t > stop + 0.4) return null;
  const k = spring({ frame: frame - Math.round(from * FPS), fps: FPS, config: { damping: 20, mass: 0.7 } });
  const o = interpolate(t, [stop - 0.05, stop + 0.3], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "center", paddingBottom: 54, pointerEvents: "none" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          padding: sub ? "18px 30px 18px 26px" : "16px 30px 16px 26px",
          borderRadius: 18,
          background: "rgba(14,15,17,0.78)",
          backdropFilter: "blur(20px) saturate(140%)",
          boxShadow: "0 0 0 1px rgba(255,255,255,0.08), 0 18px 50px rgba(0,0,0,0.5)",
          opacity: Math.min(k * 1.2, 1) * o,
          transform: `translateY(${(1 - k) * 26 + (1 - o) * 10}px) scale(${0.96 + 0.04 * k})`,
          filter: `blur(${(1 - k) * 6}px)`,
        }}
      >
        <i style={{ width: 12, height: 12, borderRadius: 4, background: dot ?? C.ink, flex: "none", transform: `scale(${k})` }} />
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ color: C.ink, fontSize: 38, fontWeight: 600, letterSpacing: -0.7 }}>{text}</span>
          {sub && <span style={{ color: C.muted, fontSize: 24, fontWeight: 500 }}>{sub}</span>}
        </div>
      </div>
    </AbsoluteFill>
  );
}
