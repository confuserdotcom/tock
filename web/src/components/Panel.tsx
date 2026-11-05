import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** Panel is one bordered section with a small caps label and an aside. */
export function Panel({ label, aside, className, children }: { label: string; aside?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <section className={cn("min-w-0 rounded-lg border bg-card p-5", className)}>
      <div className="mb-3 flex items-baseline justify-between gap-3">
        <h2 className="lbl">{label}</h2>
        {aside && <div className="text-xs text-muted-foreground">{aside}</div>}
      </div>
      {children}
    </section>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>;
}

/** Change shows a delta coloured by meaning: lowerIsGood flips it. */
export function Change({ v, text, lowerIsGood }: { v: number; text: string; lowerIsGood?: boolean }) {
  if (v === 0) return <span className="text-muted-foreground">{text}</span>;
  const good = lowerIsGood ? v < 0 : v > 0;
  return <span className={good ? "text-good" : "text-bad"}>{text}</span>;
}
