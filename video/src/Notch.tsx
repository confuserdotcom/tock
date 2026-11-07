// The notch clip: the screen recording from capture/notch.sh, cropped to
// the top middle of the display, with a camera that leans in on small
// moments and pulls back for the big panels.
import React from "react";
import { AbsoluteFill, Easing, interpolate, OffthreadVideo, Sequence, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { C, fontFamily, FPS, H, W } from "./theme";
import { DayRing } from "./DayRing";

// The built-in display records at 2940x1912 px; the crop is 2200x1238 px
// centred under the notch, which is 16:9.
const SRC_W = 2940;
const CROP_W = 2200;
const K = W / CROP_W;

export type Beat = { t: number; caption: string; sub?: string; zoom?: number; dot?: string };

// Seconds after NOTCH_START where each demo moment opens, measured from
// footage/notch/screen.mp4 (brightness jumps under the notch): session 2.4,
// drift 7.3, back 13.3, done 16.3, label 22.3, inbox ~30.8, meeting 34.3,
// recap 39.3, dot 47.3 s into the recording.
export const NOTCH_START = 1.8;
export const NOTCH_LEN = 51.5;
export const BEATS: Beat[] = [
  { t: 0, caption: "A focus session, beside the notch.", sub: "Its goal and the time left.", zoom: 1.9, dot: C.deep },
  { t: 5.4, caption: "Drift off, and it calls you back.", sub: "Snooze it, or go back to work.", zoom: 1.3, dot: C.distract },
  { t: 11.4, caption: "Back on task.", zoom: 1.9, dot: C.deep },
  { t: 14.4, caption: "Every session ends with how it went.", sub: "Deep time, longest run, switches.", zoom: 1.25, dot: C.deep },
  { t: 20.4, caption: "Label the day from the notch.", sub: "One pick each; every pick saves a rule.", zoom: 1.25, dot: C.browse },
  { t: 28.9, caption: "Inbox clear.", sub: "New rules apply to past days too.", zoom: 1.4, dot: C.deep },
  { t: 32.4, caption: "Meetings, before they start.", zoom: 1.9, dot: C.shallow },
  { t: 37.4, caption: "The evening recap.", sub: "Against your usual day.", zoom: 1.1, dot: C.ink },
  { t: 45.4, caption: "Missed something? It waits as a dot.", zoom: 1.9, dot: C.neutral },
];

const INTRO = 1.7;
const OUTRO = 0.9;
export const notchFrames = Math.round((INTRO + NOTCH_LEN + OUTRO) * FPS);
const ease = Easing.bezier(0.65, 0, 0.35, 1);

export const Notch: React.FC = () => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const now = frame / FPS;
  const t = Math.max(0, now - INTRO);
  // Zoom on the notch, which sits at the top middle; the pivot stays there.
  let z = 1;
  let prev = 1;
  for (const b of BEATS) {
    if (t < b.t) break;
    const k = ease(Math.min(1, (t - b.t) / 1.1));
    z = Math.exp(Math.log(prev) + (Math.log(b.zoom ?? 1) - Math.log(prev)) * k);
    prev = b.zoom ?? 1;
  }
  const outroAt = durationInFrames / FPS - OUTRO;
  const back = ease(Math.min(1, Math.max(0, (now - outroAt + 0.3) / OUTRO)));
  z = Math.exp(Math.log(z) * (1 - back));
  const land = spring({ frame: frame - Math.round((INTRO - 0.7) * FPS), fps: FPS, config: { damping: 22, mass: 0.9, stiffness: 120 } });
  const fade = interpolate(now, [outroAt + OUTRO * 0.35, outroAt + OUTRO], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const s = z * interpolate(land, [0, 1], [0.94, 1]);

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily }}>
      <AbsoluteFill style={{ opacity: Math.min(1, land * 1.3) * fade, transformOrigin: "50% 0%", transform: `scale(${s})` }}>
        <Sequence from={Math.round(INTRO * FPS)} layout="none">
          <OffthreadVideo
            src={staticFile("notch/screen.mp4")}
            startFrom={Math.round(NOTCH_START * FPS)}
            muted
            style={{ position: "absolute", left: -((SRC_W - CROP_W) / 2) * K, top: 0, width: SRC_W * K }}
          />
        </Sequence>
      </AbsoluteFill>
      <NotchTitle />
      {BEATS.map((b, i) => (
        <NotchCaption key={i} from={INTRO + b.t + 0.4} to={INTRO + (BEATS[i + 1]?.t ?? NOTCH_LEN)} beat={b} />
      ))}
      <AbsoluteFill style={{ background: C.bg, opacity: 1 - fade }} />
    </AbsoluteFill>
  );
};

function NotchTitle() {
  const frame = useCurrentFrame();
  const t = frame / FPS;
  if (t > INTRO) return null;
  const out = interpolate(t, [INTRO - 0.55, INTRO - 0.1], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: ease });
  const words = "It lives in the notch.".split(" ");
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", flexDirection: "column", gap: 26, opacity: 1 - out, transform: `translateY(${-60 * out}px)`, filter: `blur(${8 * out}px)` }}>
      <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
        <DayRing size={64} body={false} />
        <span style={{ color: C.muted, fontSize: 26, fontWeight: 500 }}>Notch</span>
      </div>
      <div style={{ color: C.ink, fontSize: 76, fontWeight: 600, letterSpacing: -2, display: "flex", gap: 20 }}>
        {words.map((w, i) => {
          const k = spring({ frame: frame - 6 - i * 3, fps: FPS, config: { damping: 18, mass: 0.6 } });
          return (
            <span key={i} style={{ opacity: k, transform: `translateY(${(1 - k) * 34}px)`, filter: `blur(${(1 - k) * 10}px)` }}>
              {w}
            </span>
          );
        })}
      </div>
    </AbsoluteFill>
  );
}

function NotchCaption({ from, to, beat }: { from: number; to: number; beat: Beat }) {
  const frame = useCurrentFrame();
  const t = frame / FPS;
  if (t < from || t > to + 0.4) return null;
  const k = spring({ frame: frame - Math.round(from * FPS), fps: FPS, config: { damping: 20, mass: 0.7 } });
  const o = interpolate(t, [to - 0.05, to + 0.3], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "center", paddingBottom: 64 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 16, padding: "18px 30px 18px 26px", borderRadius: 18, background: "rgba(14,15,17,0.82)", backdropFilter: "blur(20px)", boxShadow: "0 0 0 1px rgba(255,255,255,0.08), 0 18px 50px rgba(0,0,0,0.5)", opacity: Math.min(k * 1.2, 1) * o, transform: `translateY(${(1 - k) * 26}px) scale(${0.96 + 0.04 * k})`, filter: `blur(${(1 - k) * 6}px)` }}>
        <i style={{ width: 12, height: 12, borderRadius: 4, background: beat.dot ?? C.ink, transform: `scale(${k})` }} />
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ color: C.ink, fontSize: 38, fontWeight: 600, letterSpacing: -0.7 }}>{beat.caption}</span>
          {beat.sub && <span style={{ color: C.muted, fontSize: 24, fontWeight: 500 }}>{beat.sub}</span>}
        </div>
      </div>
    </AbsoluteFill>
  );
}
