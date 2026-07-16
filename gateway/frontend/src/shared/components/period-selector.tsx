import { SegmentedControl } from "@/shared/components/segmented-control";
import { PERIOD_DAYS, toPeriodValue, type PeriodDays } from "@/shared/lib/period";

export function PeriodSelector({ value, onChange, ariaLabel, className }: { value: PeriodDays; onChange: (value: PeriodDays) => void; ariaLabel: string; className?: string }) {
  return <SegmentedControl value={value} options={PERIOD_DAYS.map((days) => ({ value: days, label: toPeriodValue(days) }))} onChange={onChange} ariaLabel={ariaLabel} className={className} />;
}
