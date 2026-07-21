export interface WeeklyQuotaBreakdownItem {
  productCode: number;
  usagePercent: number;
}

export interface WeeklyQuotaProduct extends WeeklyQuotaBreakdownItem {
  remainingPercent: number;
}

export interface WebQuotaWindow {
  mode: string;
  remaining: number;
  resetAt?: string;
  breakdown?: WeeklyQuotaBreakdownItem[];
}

export interface WebQuotaAvailability {
  exhausted: boolean;
  resetAt?: string;
  imagineRemainingPercent?: number;
}

export function getWebQuotaAvailability(windows: WebQuotaWindow[] | undefined): WebQuotaAvailability;
export function normalizeWeeklyQuotaProducts(breakdown: WeeklyQuotaBreakdownItem[] | undefined): WeeklyQuotaProduct[];
