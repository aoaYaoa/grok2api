export interface WeeklyQuotaBreakdownItem {
  productCode: number;
  usagePercent: number;
}

export interface WeeklyQuotaProduct extends WeeklyQuotaBreakdownItem {
  remainingPercent: number;
}

export function normalizeWeeklyQuotaProducts(breakdown: WeeklyQuotaBreakdownItem[] | undefined): WeeklyQuotaProduct[];
