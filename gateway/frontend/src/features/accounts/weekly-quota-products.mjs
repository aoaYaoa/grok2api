const productOrder = new Map([[4, 0], [5, 1], [6, 2]]);

export function getWebQuotaAvailability(windows) {
  const values = Array.isArray(windows) ? windows : [];
  const weekly = values.filter((window) => window?.mode === "weekly");
  const relevant = weekly.length > 0 ? weekly : values;
  const exhausted = relevant.length > 0 && !relevant.some((window) => Number(window?.remaining) > 0);
  const resetAt = (weekly[0] ?? values[0])?.resetAt;
  const imagine = normalizeWeeklyQuotaProducts(weekly[0]?.breakdown).find((item) => item.productCode === 5);
  return {
    exhausted,
    resetAt,
    imagineRemainingPercent: imagine?.remainingPercent,
  };
}

export function normalizeWeeklyQuotaProducts(breakdown) {
  if (!Array.isArray(breakdown)) return [];
  return breakdown
    .filter((item) => Number.isFinite(item?.productCode) && Number.isFinite(item?.usagePercent))
    .map((item) => {
      const usagePercent = Math.max(0, Math.min(100, item.usagePercent));
      return {
        productCode: item.productCode,
        usagePercent,
        remainingPercent: 100 - usagePercent,
      };
    })
    .sort((left, right) => (productOrder.get(left.productCode) ?? 100 + left.productCode) - (productOrder.get(right.productCode) ?? 100 + right.productCode));
}
