import assert from "node:assert/strict";
import test from "node:test";

import { getWebQuotaAvailability, normalizeWeeklyQuotaProducts } from "../src/features/accounts/weekly-quota-products.mjs";

test("weekly quota products keep zero usage and expose remaining percentages", () => {
  assert.deepEqual(normalizeWeeklyQuotaProducts([
    { productCode: 5, usagePercent: 100 },
    { productCode: 4, usagePercent: 0 },
  ]), [
    { productCode: 4, usagePercent: 0, remainingPercent: 100 },
    { productCode: 5, usagePercent: 100, remainingPercent: 0 },
  ]);
});

test("web weekly quota with no remaining capacity is waiting for reset", () => {
  assert.deepEqual(getWebQuotaAvailability([
    {
      mode: "weekly",
      remaining: 0,
      total: 10000,
      usagePercent: 100,
      resetAt: "2026-07-21T20:05:00Z",
      breakdown: [
        { productCode: 4, usagePercent: 0 },
        { productCode: 5, usagePercent: 100 },
      ],
    },
  ]), {
    exhausted: true,
    resetAt: "2026-07-21T20:05:00Z",
    imagineRemainingPercent: 0,
  });
});

test("web weekly quota with remaining capacity stays routable", () => {
  assert.deepEqual(getWebQuotaAvailability([
    {
      mode: "weekly",
      remaining: 8500,
      total: 10000,
      usagePercent: 15,
      resetAt: "2026-07-21T20:05:00Z",
      breakdown: [{ productCode: 5, usagePercent: 15 }],
    },
  ]), {
    exhausted: false,
    resetAt: "2026-07-21T20:05:00Z",
    imagineRemainingPercent: 85,
  });
});

test("legacy web quota is exhausted only when every mode has no capacity", () => {
  assert.equal(getWebQuotaAvailability([
    { mode: "fast", remaining: 0, total: 30, usagePercent: 100 },
    { mode: "expert", remaining: 2, total: 10, usagePercent: 80 },
  ]).exhausted, false);

  assert.equal(getWebQuotaAvailability([
    { mode: "fast", remaining: 0, total: 30, usagePercent: 100 },
    { mode: "expert", remaining: 0, total: 10, usagePercent: 100 },
  ]).exhausted, true);
});
