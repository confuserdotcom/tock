// The tock dark palette (web/src/styles.css), for the stage around the footage.
import { loadFont } from "@remotion/google-fonts/Inter";

export const { fontFamily } = loadFont("normal", { weights: ["400", "500", "600", "700"], subsets: ["latin"] });

export const C = {
  bg: "#0b0b0c",
  card: "#16171a",
  border: "#26272b",
  ink: "#e6e6e8",
  muted: "#8b8d93",
  faint: "#5d5f66",
  deep: "#4fa89a",
  shallow: "#6f86b5",
  browse: "#a07fb0",
  neutral: "#7d8088",
  distract: "#c9625a",
};

export const KIND_ORDER = [C.deep, C.shallow, C.browse, C.neutral, C.distract];

export const FPS = 60;
export const W = 1920;
export const H = 1080;
