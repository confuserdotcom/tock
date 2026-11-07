// The tock icon's day ring, drawn on: the ring splits into deep, shallow
// and distracted time, then the hand settles. Same geometry as
// packaging/bar/icon/icon.svg.
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { C } from "./theme";

const R = 250;
const CIRC = 2 * Math.PI * R;

// Arc i runs from a to b degrees, clockwise from 12 o'clock.
const ARCS: { a: number; b: number; color: string }[] = [
  { a: 0, b: 109, color: C.deep },
  { a: 133, b: 205, color: C.shallow },
  { a: 225, b: 287, color: C.distract },
];

export function DayRing({ size = 160, delay = 0, body = true }: { size?: number; delay?: number; body?: boolean }) {
  const f = useCurrentFrame() - delay;
  const { fps } = useVideoConfig();
  const pop = spring({ frame: f, fps, config: { damping: 16, mass: 0.7 } });
  const hand = spring({ frame: f - 14, fps, config: { damping: 11, mass: 0.6 } });
  return (
    <svg width={size} height={size} viewBox="0 0 1024 1024" style={{ transform: `scale(${0.86 + 0.14 * pop})`, opacity: Math.min(1, pop * 1.4), overflow: "visible" }}>
      <defs>
        <linearGradient id="dr-body" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#26282d" />
          <stop offset="1" stopColor="#0e0f11" />
        </linearGradient>
      </defs>
      {body && <rect x="100" y="100" width="824" height="824" rx="185" fill="url(#dr-body)" stroke="#ffffff14" strokeWidth="4" />}
      <circle cx="512" cy="512" r={R} fill="none" stroke="#2c2e34" strokeWidth="64" />
      {ARCS.map((arc, i) => {
        const len = ((arc.b - arc.a) / 360) * CIRC;
        const k = interpolate(f, [6 + i * 7, 26 + i * 7], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: (t) => 1 - Math.pow(1 - t, 3) });
        return (
          <circle
            key={i}
            cx="512"
            cy="512"
            r={R}
            fill="none"
            stroke={arc.color}
            strokeWidth="64"
            strokeLinecap="round"
            strokeDasharray={`${len * k} ${CIRC}`}
            transform={`rotate(${arc.a - 90} 512 512)`}
            opacity={k > 0.01 ? 1 : 0}
          />
        );
      })}
      <g transform={`rotate(${interpolate(hand, [0, 1], [-140, 0])} 512 512)`} opacity={Math.min(1, Math.max(0, hand * 2))}>
        <line x1="512" y1="512" x2="512" y2="350" stroke={C.ink} strokeWidth="40" strokeLinecap="round" />
      </g>
      <circle cx="512" cy="512" r={40 * Math.min(1, pop)} fill={C.ink} />
    </svg>
  );
}
