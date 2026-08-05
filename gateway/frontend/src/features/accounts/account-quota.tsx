import type { TFunction } from "i18next";
import { Info } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AccountDTO, BillingDTO, QuotaDTO } from "@/features/accounts/accounts-api";
import { getWebQuotaAvailability, normalizeWeeklyQuotaProducts } from "@/features/accounts/weekly-quota-products.mjs";
import { cn } from "@/shared/lib/cn";
import { formatDateTime, formatNumber } from "@/shared/lib/format";

export function AccountQuota({ quota, billing, locale }: { quota: QuotaDTO; billing?: BillingDTO; locale: string }) {
  const { t } = useTranslation();
  if (quota.type === "unknown") {
    return <span className="text-xs text-muted-foreground">{t("accountType.pending")}</span>;
  }
  if (quota.type !== "free") {
    return <BuildQuota quota={quota} billing={billing} locale={locale} />;
  }

  const percent = Math.min(100, Math.max(0, quota.usagePercent));
  const used = formatNumber(quota.used, locale, 0);
  const limit = formatNumber(quota.limit, locale, 0);
  const isEstimated = !quota.limitKnown;
  const statusDescription = quota.status === "waitingReset" && quota.nextProbeAt
    ? t("accounts.waitingResetUntil", { time: formatDateTime(quota.nextProbeAt, locale) })
    : quota.status === "probing"
      ? t("accounts.probingQuota")
      : quota.confirmed
        ? t("accounts.upstreamConfirmed")
        : null;
  const usage = quota.limit > 0
    ? isEstimated ? t("accounts.freeEstimatedUsage", { used, limit }) : `${used} / ${limit} tokens`
    : t("accounts.freeObservedUsage", { used });

  return (
    <div className="w-full min-w-0 space-y-1.5">
      <div className="flex items-start justify-between gap-3 text-[11px] font-normal">
        <div className="inline-flex min-w-0 items-center gap-1 text-muted-foreground">
          <span>{usage}</span>
          {isEstimated ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button type="button" className="inline-flex shrink-0 text-muted-foreground transition-colors hover:text-foreground" aria-label={t("accounts.freeEstimatedDescription")}>
                  <Info className="size-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>{t("accounts.freeEstimatedDescription")}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        <span className="shrink-0 text-muted-foreground">{isEstimated ? "≈" : ""}{formatNumber(quota.usagePercent, locale, 1)}%</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${percent}%` }} /></div>
      {statusDescription ? <div className="text-[11px] text-muted-foreground">{statusDescription}</div> : null}
    </div>
  );
}

function BuildQuota({ quota, billing, locale }: { quota: QuotaDTO; billing?: BillingDTO; locale: string }) {
  const { t } = useTranslation();
  const percentageQuota = quota.unit === "percent";
  const hasWeekly = percentageQuota || billing?.usagePeriodType === "USAGE_PERIOD_TYPE_WEEKLY";
  const hasMonthly = !percentageQuota && quota.limit > 0;
  if (!hasWeekly && !hasMonthly) return <span className="text-xs text-muted-foreground">{t("accounts.paidQuotaUsage")}</span>;

  const weeklyPercent = Math.max(0, Math.min(100, percentageQuota ? quota.usagePercent : (billing?.creditUsagePercent ?? 0)));
  const monthlyPercent = Math.max(0, Math.min(100, quota.usagePercent));
  const weeklyPeriodEnd = quota.periodEnd ?? billing?.usagePeriodEnd;
  const statusDescription = quota.status === "waitingReset" && quota.nextProbeAt
    ? t("accounts.paidWaitingResetUntil", { time: formatDateTime(quota.nextProbeAt, locale) })
    : quota.status === "probing" ? t("accounts.paidProbingQuota") : null;

  return (
    <div className="w-full min-w-0 space-y-1.5">
      <div className={cn("grid w-full min-w-0 divide-x divide-border/70", hasWeekly && hasMonthly ? "grid-cols-2" : "grid-cols-1")}>
        {hasWeekly ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <button type="button" className="min-w-0 px-2 text-left font-normal first:pl-0 last:pr-0">
                <div className="flex items-center justify-between gap-1 text-[11px]"><span className="truncate text-muted-foreground">{t("accounts.weeklyQuota")}</span><span className="shrink-0 tabular-nums">{formatNumber(weeklyPercent, locale, 1)}%</span></div>
                <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${weeklyPercent}%` }} /></div>
              </button>
            </TooltipTrigger>
            <TooltipContent>
              <div>{t("accounts.weeklyLimit", { percent: formatNumber(100 - weeklyPercent, locale, 1) })}</div>
              <div className="text-muted-foreground">{weeklyPeriodEnd ? t("accounts.quotaResetAt", { time: formatDateTime(weeklyPeriodEnd, locale) }) : t("accounts.quotaResetUnknown")}</div>
            </TooltipContent>
          </Tooltip>
        ) : null}
        {hasMonthly ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <button type="button" className="min-w-0 px-2 text-left font-normal first:pl-0 last:pr-0">
                <div className="flex items-center justify-between gap-1 text-[11px]"><span className="truncate text-muted-foreground">{t("accounts.monthlyQuota")}</span><span className="shrink-0 tabular-nums">{formatNumber(quota.used, locale, 2)}/{formatNumber(quota.limit, locale, 2)}</span></div>
                <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${monthlyPercent}%` }} /></div>
              </button>
            </TooltipTrigger>
            <TooltipContent>
              <div>{t("accounts.paidQuotaDetails", { remaining: formatNumber(quota.remaining, locale, 2) })}</div>
              <div className="text-muted-foreground">{billing?.billingPeriodEnd ? t("accounts.quotaResetAt", { time: formatDateTime(billing.billingPeriodEnd, locale) }) : t("accounts.quotaResetUnknown")}</div>
            </TooltipContent>
          </Tooltip>
        ) : null}
      </div>
      {statusDescription ? <div className="text-[11px] text-muted-foreground">{statusDescription}</div> : null}
    </div>
  );
}

const visibleWebQuotaModes = ["auto", "fast", "expert", "heavy"] as const;

export function ConsoleQuota({ windows, locale }: { windows: NonNullable<AccountDTO["quotaWindows"]>; locale: string }) {
  const { t } = useTranslation();
  if (windows.length === 0) return <span className="text-xs text-muted-foreground">{t("accounts.quotaNotSynced")}</span>;
  const windowsByMode = new Map(windows.map((window) => [window.mode, window]));
  const modes = [
    { mode: "console", label: t("creativeConsole.modes.chat") },
    { mode: "console_image", label: t("creativeConsole.modes.image") },
    { mode: "console_video", label: t("creativeConsole.modes.video") },
  ] as const;
  return (
    <div className="grid w-full min-w-0 grid-cols-3 divide-x divide-border/70">
      {modes.map(({ mode, label }) => {
        const window = windowsByMode.get(mode);
        if (!window) {
          return <div key={mode} className="min-w-0 px-2 first:pl-0 last:pr-0"><div className="flex items-center justify-between gap-1 text-[11px]"><span className="truncate text-muted-foreground">{label}</span><span className="text-muted-foreground">-</span></div><div className="mt-1.5 h-1.5 rounded-full bg-muted" /></div>;
        }
        return <WebQuotaMode key={mode} mode={label} window={window} locale={locale} compact recoveryProbe={mode === "console" && window.remaining === 0} />;
      })}
    </div>
  );
}

export function WebQuota({ windows, locale, tier }: { windows: NonNullable<AccountDTO["quotaWindows"]>; locale: string; tier?: AccountDTO["webTier"] }) {
  const { t } = useTranslation();
  if (windows.length === 0) return <span className="text-xs text-muted-foreground">{t("accounts.quotaNotSynced")}</span>;
  const windowsByMode = new Map(windows.map((window) => [window.mode, window]));
  const weekly = windowsByMode.get("weekly");
  if (weekly) return <WeeklyWebQuota window={weekly} locale={locale} t={t} />;

  const fast = windowsByMode.get("fast");
  if (tier === "basic" && fast) {
    return (
      <WebChatQuotaSummary>
        <WebQuotaMode mode="Fast" window={fast} locale={locale} />
      </WebChatQuotaSummary>
    );
  }
  const mediaWeeklyQuotaUnavailable = tier === "super" || tier === "heavy";
  return (
    <WebChatQuotaSummary mediaWeeklyQuotaUnavailable={mediaWeeklyQuotaUnavailable}>
      <div className="grid w-full min-w-0 grid-cols-4 divide-x divide-border/70">
        {visibleWebQuotaModes.map((mode) => {
          const window = windowsByMode.get(mode);
          if (!window) {
            return <div key={mode} className="min-w-0 px-2 first:pl-0 last:pr-0"><div className="flex items-center justify-between gap-1 text-[11px]"><span className="truncate capitalize text-muted-foreground">{mode}</span><span className="text-muted-foreground">-</span></div><div className="mt-1.5 h-1.5 rounded-full bg-muted" /></div>;
          }
          return <WebQuotaMode key={mode} mode={formatWebQuotaMode(mode)} window={window} locale={locale} compact />;
        })}
      </div>
    </WebChatQuotaSummary>
  );
}

type WebQuotaWindow = NonNullable<AccountDTO["quotaWindows"]>[number];

function WebChatQuotaSummary({ children, mediaWeeklyQuotaUnavailable = false }: { children: ReactNode; mediaWeeklyQuotaUnavailable?: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="w-full min-w-0 space-y-1.5">
      {children}
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] leading-4">
        <span className="text-muted-foreground">{t("accounts.officialChatQuotaWindow")}</span>
        {mediaWeeklyQuotaUnavailable ? <span className="font-medium text-amber-700 dark:text-amber-400">{t("accounts.mediaWeeklyQuotaUnavailable")}</span> : null}
      </div>
    </div>
  );
}

function WeeklyWebQuota({ window, locale, t }: { window: WebQuotaWindow; locale: string; t: TFunction }) {
  const usedPercent = Math.max(0, Math.min(100, window.usagePercent));
  const remainingPercent = Math.max(0, 100 - usedPercent);
  const products = normalizeWeeklyQuotaProducts(window.breakdown);
  const visibleProducts = products.slice(0, 3);
  const exhausted = getWebQuotaAvailability([window]).exhausted;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" className="block w-full min-w-0 text-left">
          {visibleProducts.length > 0 ? (
            <div className={cn("grid w-full min-w-0 divide-x divide-border/70", visibleProducts.length === 1 ? "grid-cols-1" : visibleProducts.length === 2 ? "grid-cols-2" : "grid-cols-3")}>
              {visibleProducts.map((item) => {
                const itemExhausted = item.remainingPercent <= 0;
                return <div key={item.productCode} className="min-w-0 px-2 first:pl-0 last:pr-0"><div className="flex items-center justify-between gap-1 text-[11px]"><span className={cn("truncate", itemExhausted ? "font-medium text-amber-700 dark:text-amber-300" : "text-muted-foreground")}>{quotaProductLabel(item.productCode, t)}</span><span className={cn("shrink-0 tabular-nums", itemExhausted && "font-medium text-amber-700 dark:text-amber-300")}>{t("accounts.quotaRemainingShort", { percent: formatNumber(item.remainingPercent, locale, 1) })}</span></div><div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"><div className={cn("h-full", quotaProductColor(item.productCode))} style={{ width: `${item.remainingPercent}%` }} /></div></div>;
              })}
            </div>
          ) : (
            <><div className="flex items-center justify-between gap-2 text-[11px]"><span className="truncate text-muted-foreground">{t("accounts.weeklyQuota")}</span><span className="shrink-0 tabular-nums">{t("accounts.quotaRemainingShort", { percent: formatNumber(remainingPercent, locale, 1) })}</span></div><div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${remainingPercent}%` }} /></div></>
          )}
          {exhausted ? <div className="mt-1.5 truncate text-[10px] font-medium text-amber-700 dark:text-amber-300">{window.resetAt ? t("accounts.webQuotaWaitingResetUntil", { time: formatDateTime(window.resetAt, locale) }) : t("accounts.webQuotaWaitingReset")}</div> : null}
        </button>
      </TooltipTrigger>
      <TooltipContent>
        <div>{t("accounts.webWeeklyQuotaUsage", { remaining: formatNumber(remainingPercent, locale, 1) })}</div>
        <div className="text-muted-foreground">{window.resetAt ? t("accounts.quotaResetAt", { time: formatDateTime(window.resetAt, locale) }) : t("accounts.quotaResetUnknown")}</div>
        {window.syncedAt ? <div className="text-muted-foreground">{t("accounts.quotaSyncedAt", { time: formatDateTime(window.syncedAt, locale) })}</div> : null}
        {products.length > 0 ? <div className="mt-2 grid gap-1 border-t pt-2">{products.map((item) => <div key={item.productCode} className="flex items-center justify-between gap-4"><span className="flex items-center gap-1.5"><span className={cn("size-2 rounded-full", quotaProductColor(item.productCode))} />{quotaProductLabel(item.productCode, t)}</span><span className="tabular-nums">{formatNumber(item.remainingPercent, locale, 1)}%</span></div>)}</div> : null}
      </TooltipContent>
    </Tooltip>
  );
}

function WebQuotaMode({ mode, window, locale, compact = false, recoveryProbe = false }: { mode: string; window: WebQuotaWindow; locale: string; compact?: boolean; recoveryProbe?: boolean }) {
  const { t } = useTranslation();
  const percent = window.total > 0 ? Math.max(0, Math.min(100, window.remaining / window.total * 100)) : 0;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" className={cn("block w-full min-w-0 text-left", compact && "px-2 first:pl-0 last:pr-0")}>
          <div className="flex items-center justify-between gap-1 text-[11px]"><span className="truncate text-muted-foreground">{mode}</span><span className="shrink-0 tabular-nums">{formatNumber(window.remaining, locale, 0)}/{formatNumber(window.total, locale, 0)}</span></div>
          <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${percent}%` }} /></div>
        </button>
      </TooltipTrigger>
      <TooltipContent><div>{t("accounts.webModeQuotaRemaining", { mode, remaining: formatNumber(window.remaining, locale, 0) })}</div><div className="text-muted-foreground">{window.resetAt ? recoveryProbe ? t("console.recoveryProbeAt", { time: formatDateTime(window.resetAt, locale) }) : t("accounts.quotaResetAt", { time: formatDateTime(window.resetAt, locale) }) : t("accounts.quotaResetUnknown")}</div>{window.syncedAt ? <div className="text-muted-foreground">{t("accounts.quotaSyncedAt", { time: formatDateTime(window.syncedAt, locale) })}</div> : null}</TooltipContent>
    </Tooltip>
  );
}

function formatWebQuotaMode(mode: string): string {
  return mode ? mode.charAt(0).toUpperCase() + mode.slice(1) : mode;
}

function quotaProductLabel(code: number, t: TFunction): string {
  const keys: Record<number, string> = { 0: "thirdParty", 1: "api", 2: "build", 3: "plugins", 4: "chat", 5: "imagine", 6: "voice" };
  const key = keys[code];
  return key ? t(`quotaProducts.${key}`) : t("quotaProducts.unknown", { code });
}

function quotaProductColor(code: number): string {
  const colors: Record<number, string> = { 0: "bg-quota-product-0", 1: "bg-quota-product-1", 2: "bg-quota-product-2", 3: "bg-quota-product-3", 4: "bg-quota-product-4", 5: "bg-quota-product-5", 6: "bg-quota-product-6" };
  return colors[code] ?? "bg-muted-foreground";
}
