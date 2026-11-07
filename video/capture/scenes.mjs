// node capture/scenes.mjs [name ...]   (the showcase server must be on :7799)
//
// Each scene is one clip: the README section's loop and a piece of the
// launch video. Marks name the moments the edit zooms to.
import { record, W, H } from "./lib.mjs";
import { spawn } from "node:child_process";

const BIN = new URL("../footage/showcase.test", import.meta.url).pathname;

/** fresh starts the showcase server on a new temp data dir; edits don't carry over. */
async function fresh() {
  const p = spawn(BIN, ["-test.run", "^TestShowcase$", "-test.timeout", "0"], { env: { ...process.env, TOCK_DEMO_PORT: "7799" }, stdio: "ignore" });
  for (let i = 0; i < 100; i++) {
    try {
      await fetch("http://127.0.0.1:7799/");
      return p;
    } catch {
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  throw new Error("showcase server did not start");
}

const view = async (r, hash) => {
  await r.page.evaluate((h) => (location.hash = h), hash);
  await r.wait(1200);
};
const wheel = async (r, dy, ms = 1200) => {
  const steps = Math.round(ms / 16);
  for (let i = 0; i < steps; i++) {
    await r.page.mouse.wheel(0, dy / steps);
    await r.wait(16);
  }
};

const scenes = {
  // The day at a glance: timeline, score, kinds of time, goals.
  today: {
    async run(r) {
      const p = r.page;
      await r.wait(800);
      await r.mark("timeline", p.getByText("Timeline", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.hover(p.getByLabel("Xcode 10:05 to 11:04"), "bar-code", 1100);
      await r.wait(1400);
      await r.hover(p.getByLabel("Figma 13:04 to 14:41"), "bar-design", 900);
      await r.wait(1400);
      await r.mark("score", p.getByText("Focus score", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.moveTo(260, 640, 900);
      await r.wait(1600);
      await r.mark("donut", p.getByText("Kinds of time", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.moveTo(1150, 640, 1000);
      await r.wait(1600);
      await wheel(r, 520, 1400);
      await r.mark("goals", p.getByText("Goals", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.wait(2200);
    },
  },

  // Fix the record from the timeline: label a bar, pick a range.
  edit: {
    async run(r) {
      const p = r.page;
      await r.wait(600);
      await r.mark("timeline", p.getByText("Timeline", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.click(p.getByLabel("Arc 14:41 to 14:53"), "bar-unlabelled", 1100);
      await r.wait(900);
      await r.mark("sheet", p.getByRole("dialog"));
      await r.wait(500);
      await p.getByPlaceholder("Category, or Category / project").click();
      await r.type("Resea", 110);
      await r.wait(500);
      await r.mark("pick", p.getByRole("option", { name: "Research" }));
      await p.keyboard.press("Enter");
      await r.wait(700);
      await r.mark("toast", p.locator("[data-sonner-toast]").first());
      await r.wait(2000);
      // A range across the lanes.
      const lane = await p.getByLabel("Arc 11:04 to 11:13").boundingBox();
      await r.mark("range", p.getByText("Timeline", { exact: true }).locator("xpath=ancestor::section[1]"));
      await r.drag(lane.x - 4, lane.y - 30, lane.x + 150, lane.y + 120, 1300);
      await r.wait(2400);
      await p.keyboard.press("Escape");
      await r.wait(600);
    },
  },

  // The inbox: one key labels a block and writes the rule.
  inbox: {
    async setup(r) {
      await view(r, "inbox");
    },
    async run(r) {
      const p = r.page;
      await r.wait(900);
      await r.mark("list", p.locator("main"));
      const first = p.getByRole("button", { name: /Label/ }).first();
      await r.click(first, "label", 1200);
      await r.wait(900);
      await r.mark("sheet", p.getByRole("dialog"));
      await r.type("Research", 100);
      await r.wait(500);
      await p.keyboard.press("Enter");
      await r.wait(700);
      await r.mark("toast", p.locator("[data-sonner-toast]").first());
      await r.wait(2400);
    },
  },

  // The week: hours per day, focus heat map, export.
  week: {
    async setup(r) {
      await view(r, "week");
    },
    async run(r) {
      const p = r.page;
      await r.wait(1200);
      await r.moveTo(500, 330, 1100);
      await r.wait(1500);
      await wheel(r, 560, 1500);
      await r.wait(1500);
      await wheel(r, -560, 1200);
      await r.click(p.getByRole("button", { name: "Export" }), "export", 1100);
      await r.wait(700);
      await r.mark("menu", p.getByRole("dialog"));
      await r.hover(p.getByRole("dialog").getByRole("button").nth(2), "pick", 800);
      await r.wait(2200);
      await p.keyboard.press("Escape");
      await r.wait(500);
    },
  },

  // The month: calendar by deep time, deep time and score per day.
  trends: {
    async setup(r) {
      await view(r, "trends/2026-10-31");
    },
    async run(r) {
      await r.wait(1500);
      await r.moveTo(380, 420, 1100);
      await r.wait(1600);
      await r.moveTo(1000, 520, 1100);
      await r.wait(1500);
      await wheel(r, 500, 1400);
      await r.wait(2000);
    },
  },

  // The library: names, rates, stale ones, look-alikes.
  library: {
    async setup(r) {
      await view(r, "library");
    },
    async run(r) {
      const p = r.page;
      await r.wait(1200);
      await r.mark("hint", p.getByText(/looks like/).first());
      await r.moveTo(700, 210, 1100);
      await r.wait(1500);
      await r.click(p.getByRole("row", { name: /Atlas/ }).first(), "row", 1100);
      await r.wait(900);
      await r.mark("sheet", p.getByRole("dialog"));
      const rate = p.getByRole("dialog").getByLabel(/Hourly rate/);
      await r.click(rate, "rate", 900);
      await p.keyboard.press("Meta+A");
      await r.type("110", 140);
      await r.wait(500);
      await r.click(p.getByRole("dialog").getByRole("button", { name: "Save" }), "save", 800);
      await r.wait(700);
      await r.mark("toast", p.locator("[data-sonner-toast]").first());
      await r.wait(2400);
    },
  },

  // ⌘K: jump to a view or a date.
  palette: {
    async run(r) {
      const p = r.page;
      await r.wait(800);
      await p.keyboard.press("Meta+k");
      await r.wait(700);
      await r.mark("palette", p.getByRole("dialog"));
      await r.type("tren", 140);
      await r.wait(900);
      await p.keyboard.press("Enter");
      await r.wait(1800);
      await p.keyboard.press("Meta+k");
      await r.wait(600);
      await r.type("2026-10-14", 110);
      await r.wait(900);
      await p.keyboard.press("Enter");
      await r.wait(2000);
    },
  },
};

const pick = process.argv.slice(2);
for (const [name, s] of Object.entries(scenes)) {
  if (pick.length && !pick.includes(name)) continue;
  for (const dark of process.env.LIGHT ? [true, false] : [true]) {
    const srv = await fresh();
    try {
      await record(dark ? name : `${name}-light`, { dark }, s);
    } catch (e) {
      console.error(`${name}${dark ? "" : "-light"}: ${e.message.split("\n")[0]}`);
      break;
    } finally {
      srv.kill();
      await new Promise((r) => setTimeout(r, 300));
    }
  }
}
