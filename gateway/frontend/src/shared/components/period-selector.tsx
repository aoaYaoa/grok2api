import { Button } from "@/components/ui/button";
import { cn } from "@/shared/lib/cn";
import { PERIOD_DAYS, toPeriodValue, type PeriodDays } from "@/shared/lib/period";

export function PeriodSelector({ value, onChange, ariaLabel, className }: { value: PeriodDays; onChange: (value: PeriodDays) => void; ariaLabel: string; className?: string }) {
  return (
    <div className={cn("inline-grid h-8 max-w-full grid-cols-4 items-center gap-0.5 overflow-hidden rounded-md border bg-muted p-0.5", className)} role="group" aria-label={ariaLabel}>
      {PERIOD_DAYS.map((days) => (
        <Button
          key={days}
          type="button"
          variant="ghost"
          size="sm"
          className={cn("h-7 min-w-0 rounded-sm px-2 text-xs font-normal shadow-none", value === days && "bg-background shadow-none ring-1 ring-inset ring-border hover:bg-background")}
          aria-pressed={value === days}
          onClick={() => onChange(days)}
        >
          {toPeriodValue(days)}
        </Button>
      ))}
    </div>
  );
}
