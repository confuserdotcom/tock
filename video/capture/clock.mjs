// Page time for recording. The page's Date starts at the showcase day
// (Thu 2026-11-05 17:20 +02:00), and every JS clock (Date, performance.now,
// timers, requestAnimationFrame) runs slow times slower, so a slow screencast
// still catches every frame of a transition once sped back up.
export const SHOWCASE_NOW = Date.parse("2026-11-05T17:20:00+02:00");
export const clockScript = (slow) => `(() => {
  const S = ${slow}, R = Date, rnow = R.now.bind(R), t0 = rnow();
  const pnow = performance.now.bind(performance), p0 = pnow();
  const now = () => ${SHOWCASE_NOW} + (rnow() - t0) / S;
  class D extends R {
    constructor(...a) { if (a.length === 0) super(now()); else super(...a); }
    static now() { return now(); }
  }
  globalThis.Date = D;
  performance.now = () => p0 + (pnow() - p0) / S;
  const st = setTimeout, si = setInterval, raf = requestAnimationFrame;
  globalThis.setTimeout = (f, d = 0, ...a) => st(f, d * S, ...a);
  globalThis.setInterval = (f, d = 0, ...a) => si(f, d * S, ...a);
  globalThis.requestAnimationFrame = (cb) => raf((ts) => cb(p0 + (ts - p0) / S));
})();`;
