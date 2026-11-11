import { cn } from "@/lib/utils";

/** Switch is an on/off button (role switch): ink when on, as DESIGN.md
 *  keeps colour for kinds of time. */
export function Switch({ id, checked, onChange, disabled, label, className }: {
  id?: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean; label: string; className?: string;
}) {
  return (
    <button
      id={id}
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40",
        checked ? "border-foreground bg-foreground" : "border-input bg-secondary",
        className,
      )}
    >
      <span className={cn("block h-3.5 w-3.5 rounded-full shadow-sm transition-transform", checked ? "translate-x-[18px] bg-background" : "translate-x-[2px] bg-muted-foreground")} />
    </button>
  );
}
