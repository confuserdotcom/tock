import React from "react";
import { Composition, staticFile } from "remotion";
import { Clip, clipFrames, type ClipProps, type Events } from "./Clip";
import { CLIPS } from "./clips";
import { Notch, notchFrames } from "./Notch";
import { Launch, launchFrames, type LaunchProps } from "./Launch";
import { FPS, H, W } from "./theme";

const load = async (footage: string): Promise<Events> => (await fetch(staticFile(`${footage}/events.json`))).json();

export const Root: React.FC = () => (
  <>
    {CLIPS.map((def) => (
      <Composition
        key={def.id}
        id={def.id}
        component={Clip as React.FC<ClipProps>}
        width={W}
        height={H}
        fps={FPS}
        durationInFrames={600}
        defaultProps={{ def, ev: { duration: 10, viewport: { w: 1440, h: 900 }, events: [] } }}
        calculateMetadata={async ({ props }) => {
          const ev = await load(props.def.footage);
          return { durationInFrames: clipFrames(props.def, ev), props: { ...props, ev } };
        }}
      />
    ))}
    <Composition id="notch" component={Notch} width={W} height={H} fps={FPS} durationInFrames={notchFrames} />
    <Composition
      id="launch"
      component={Launch as React.FC<LaunchProps>}
      width={W}
      height={H}
      fps={FPS}
      durationInFrames={600}
      defaultProps={{ clips: [], notch: false } as LaunchProps}
      calculateMetadata={async () => {
        const clips = await Promise.all(CLIPS.map(async (def) => ({ def, ev: await load(def.footage) })));
        const notch = (await fetch(staticFile("notch/screen.mp4"), { method: "HEAD" })).ok;
        const props = { clips, notch };
        return { durationInFrames: launchFrames(props), props };
      }}
    />
  </>
);
