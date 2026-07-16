import type { ReactNode } from "react";

import { cn } from "@/shared/lib/cn";

type SegmentedOption<T extends string | number> = {
  value: T;
  label: ReactNode;
};

export function SegmentedControl<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
}: {
  value: T;
  options: readonly SegmentedOption<T>[];
  onChange: (value: T) => void;
  ariaLabel: string;
  className?: string;
}) {
  return (
    <div className={cn("inline-grid h-9 max-w-full grid-flow-col auto-cols-fr items-stretch gap-0.5 overflow-hidden rounded-md border bg-muted p-[3px]", className)} role="group" aria-label={ariaLabel}>
      {options.map((option) => {
        const selected = value === option.value;
        return (
          <button
            key={option.value}
            type="button"
            className={cn(
              "inline-flex h-full min-h-0 min-w-0 items-center justify-center whitespace-nowrap rounded-[4px] border border-transparent px-3 text-xs font-normal leading-none text-muted-foreground outline-none transition-[background-color,border-color,color] hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50",
              selected && "border-border bg-background text-foreground",
            )}
            aria-pressed={selected}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
