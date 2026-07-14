const productOrder = new Map([[4, 0], [5, 1], [6, 2]]);

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
