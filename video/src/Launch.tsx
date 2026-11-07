// The launch cut: an opening card, every clip in order (each opens on its
// own title), the notch after Today, and a closing card with the install.
import React from "react";
import { AbsoluteFill, interpolate, Series, spring, useCurrentFrame } from "remotion";
import { Clip, clipFrames, type ClipDef, type Events } from "./Clip";
import { Notch, notchFrames } from "./Notch";
import { DayRing } from "./DayRing";
import { C, fontFamily, FPS } from "./theme";

export type LaunchProps = { clips: { def: ClipDef; ev: Events }[]; notch: boolean };
export const CARD = Math.round(3.2 * FPS);

export const launchFrames = (p: LaunchProps) =>
  CARD * 2 + p.clips.reduce((n, c) => n + clipFrames(c.def, c.ev), 0) + (p.notch ? notchFrames : 0);

export const Launch: React.FC<LaunchProps> = ({ clips, notch }) => (
  <AbsoluteFill style={{ background: C.bg }}>
    <Series>
      <Series.Sequence durationInFrames={CARD}>
        <Card title="tock" line="Know where your day went." sub="A time tracker you never start." />
      </Series.Sequence>
      {clips.flatMap((c, i) => {
        const seq = [
          <Series.Sequence key={c.def.id} durationInFrames={clipFrames(c.def, c.ev)}>
            <Clip def={c.def} ev={c.ev} />
          </Series.Sequence>,
        ];
        if (notch && i === 0)
          seq.push(
            <Series.Sequence key="notch" durationInFrames={notchFrames}>
              <Notch />
            </Series.Sequence>,
          );
        return seq;
      })}
      <Series.Sequence durationInFrames={CARD}>
        <Card title="tock" line="Local. Private. Yours." sub="brew install confuserdotcom/tock/tock" mono />
      </Series.Sequence>
    </Series>
  </AbsoluteFill>
);

function Card({ title, line, sub, mono }: { title: string; line: string; sub: string; mono?: boolean }) {
  const f = useCurrentFrame();
  const a = (d: number) => spring({ frame: f - d, fps: FPS, config: { damping: 18, mass: 0.7 } });
  const out = interpolate(f, [CARD - 18, CARD], [1, 0], { extrapolateLeft: "clamp" });
  const rise = (k: number) => ({ opacity: k, transform: `translateY(${(1 - k) * 30}px)`, filter: `blur(${(1 - k) * 10}px)` });
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", flexDirection: "column", gap: 30, fontFamily, opacity: out, background: `radial-gradient(90% 70% at 50% 45%, #17181b 0%, ${C.bg} 70%)` }}>
      <div style={{ display: "flex", alignItems: "center", gap: 28 }}>
        <DayRing size={150} />
        <span style={{ color: C.ink, fontSize: 120, fontWeight: 700, letterSpacing: -5, ...rise(a(16)) }}>{title}</span>
      </div>
      <span style={{ color: C.ink, fontSize: 54, fontWeight: 600, letterSpacing: -1.4, ...rise(a(30)) }}>{line}</span>
      <span style={{ color: C.muted, fontSize: 30, fontWeight: 500, fontFamily: mono ? "ui-monospace, Menlo, monospace" : fontFamily, ...rise(a(42)) }}>{sub}</span>
    </AbsoluteFill>
  );
}
