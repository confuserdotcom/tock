import { createContext, useContext } from "react";

/** Groups are the capability groups that are on; off hides their parts of
 * the dashboard. Both are on until /api/status says otherwise. */
export type Groups = { focus: boolean; billing: boolean };

export const ALL_ON: Groups = { focus: true, billing: true };

export const GroupsContext = createContext<Groups>(ALL_ON);

export const useGroups = () => useContext(GroupsContext);

/** groupsOf reads the groups from /api/status; missing fields are on. */
export function groupsOf(s: { focus?: boolean; billing?: boolean } | null | undefined): Groups {
  return { focus: s?.focus ?? true, billing: s?.billing ?? true };
}

export type WeekExport = { label: string; sub: string; path?: (monday: string) => string; q?: string; ext: string };

/** weekExports is the Week Export menu: with Billing off the hours carry no
 * rate and there is no Invoice entry. */
export function weekExports(billing: boolean): { items: WeekExport[]; invoice: boolean } {
  return {
    items: [
      { label: "Week report", sub: "One page with no scripts, to mail or keep", path: (m) => `/api/report?date=${m}`, ext: "html" },
      { label: "Entries as CSV", sub: "Each entry with its app, title and URL", q: "format=csv", ext: "csv" },
      { label: "Entries as JSON", sub: "The same, for scripts", q: "format=json", ext: "json" },
      { label: "Hours by project", sub: billing ? "Hours, rate and amount, as CSV" : "Hours per project, as CSV", q: "by=project", ext: "csv" },
    ],
    invoice: billing,
  };
}
