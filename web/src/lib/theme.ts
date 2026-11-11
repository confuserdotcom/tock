// The theme: dark, light, or whatever the system says. Kept per browser.
export type Theme = "dark" | "light" | "system";
const KEY = "tock-theme";

export function getTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY);
    return t === "dark" || t === "light" ? t : "system";
  } catch {
    return "system";
  }
}

/** setTheme applies t and keeps it; system follows prefers-color-scheme. */
export function setTheme(t: Theme) {
  const el = document.documentElement;
  if (t === "system") delete el.dataset.theme;
  else el.dataset.theme = t;
  try {
    if (t === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, t);
  } catch {
    /* storage blocked: it lasts until the page reloads */
  }
  window.dispatchEvent(new Event("tock:theme"));
}
