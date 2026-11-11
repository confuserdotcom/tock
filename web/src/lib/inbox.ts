// Pure helpers for the Inbox.
import type { Item } from "./api";

/** guessText says why the inbox guesses a label, inline under the item:
 *  "Study: same site, 80 % of the evidence". "" without a reason. */
export function guessText(s: Item["suggestion"]): string {
  if (!s?.reason) return "";
  const share = s.share && s.share < 1 ? `, ${Math.round(s.share * 100)} % of the evidence` : "";
  return `${s.label}: ${s.reason}${share}`;
}
