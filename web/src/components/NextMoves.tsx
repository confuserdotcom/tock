import { ArrowRight } from "lucide-react";
import type { Move } from "@/lib/moves";
import { go } from "@/lib/route";
import { cn } from "@/lib/utils";

const TONE: Record<Move["tone"], string> = {
  plain: "bg-faint",
  good: "bg-good",
  warn: "bg-[hsl(38_70%_58%)]",
  bad: "bg-bad",
};

/** NextMoves: what is worth doing about today, each a click from where it's done. */
export function NextMoves({ moves }: { moves: Move[] }) {
  if (!moves.length) return null;
  return (
    <ul className="flex flex-wrap gap-2" aria-label="Next moves">
      {moves.slice(0, 4).map((m) => (
        <li key={m.id}>
          <button onClick={() => m.to && go(m.to === "inbox" ? { view: "inbox" } : { view: "library", sub: "goals" })}
            className="group flex h-9 items-center gap-2.5 rounded-full border bg-card pl-3 pr-3.5 text-[13px] transition-[background-color,border-color,transform] hover:-translate-y-px hover:border-foreground/20 hover:bg-accent active:translate-y-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <i className={cn("h-1.5 w-1.5 rounded-full", TONE[m.tone])} />
            <span className="num">{m.text}</span>
            <ArrowRight className="h-3.5 w-3.5 -translate-x-0.5 text-faint transition-transform group-hover:translate-x-0 group-hover:text-foreground" />
          </button>
        </li>
      ))}
    </ul>
  );
}
