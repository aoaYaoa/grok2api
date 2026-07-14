import assert from "node:assert/strict";
import test from "node:test";

import { normalizeWeeklyQuotaProducts } from "../src/features/accounts/weekly-quota-products.mjs";

test("weekly quota products keep zero usage and expose remaining percentages", () => {
  assert.deepEqual(normalizeWeeklyQuotaProducts([
    { productCode: 5, usagePercent: 100 },
    { productCode: 4, usagePercent: 0 },
  ]), [
    { productCode: 4, usagePercent: 0, remainingPercent: 100 },
    { productCode: 5, usagePercent: 100, remainingPercent: 0 },
  ]);
});
