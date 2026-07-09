"""
Token 数据模型

额度规则:
- Basic 新号默认 80 配额
- Super 新号默认 140 配额
- 重置后恢复默认值
- lowEffort 扣 1，highEffort 扣 4
"""

from enum import Enum
from typing import Optional, List, Dict, Any
from pydantic import BaseModel, Field, field_validator
from datetime import datetime


# 默认配额
BASIC__DEFAULT_QUOTA = 80
SUPER_DEFAULT_QUOTA = 140
HEAVY_DEFAULT_QUOTA = 400

# 失败阈值
FAIL_THRESHOLD = 5


class TokenStatus(str, Enum):
    """Token 状态"""

    ACTIVE = "active"
    DISABLED = "disabled"
    EXPIRED = "expired"
    COOLING = "cooling"


class EffortType(str, Enum):
    """请求消耗类型"""

    LOW = "low"  # 扣 1
    HIGH = "high"  # 扣 4


EFFORT_COST = {
    EffortType.LOW: 1,
    EffortType.HIGH: 4,
}


class QuotaWindow(BaseModel):
    """单模型额度窗口"""

    remaining: int = 0
    total: int = 0
    window_seconds: Optional[int] = None
    reset_at: Optional[int] = None
    synced_at: Optional[int] = None
    source: Optional[str] = None


class TokenQuotaSet(BaseModel):
    """按模型拆分的额度集合"""

    auto: QuotaWindow = Field(default_factory=QuotaWindow)
    fast: QuotaWindow = Field(default_factory=QuotaWindow)
    expert: QuotaWindow = Field(default_factory=QuotaWindow)
    heavy: Optional[QuotaWindow] = None
    grok_4_3: Optional[QuotaWindow] = None

    def get_mode(self, mode: str) -> Optional[QuotaWindow]:
        return getattr(self, mode, None)

    def mode_remaining(self, mode: str) -> int:
        window = self.get_mode(mode)
        return int(window.remaining) if window else 0

    def primary_remaining(self) -> int:
        return self.mode_remaining("auto")

    def total_remaining(self) -> int:
        return (
            self.mode_remaining("auto")
            + self.mode_remaining("fast")
            + self.mode_remaining("expert")
            + self.mode_remaining("heavy")
            + self.mode_remaining("grok_4_3")
        )

    def set_mode(self, mode: str, remaining: int, total: Optional[int] = None, *, window_seconds: Optional[int] = None, reset_at: Optional[int] = None, synced_at: Optional[int] = None, source: Optional[str] = None) -> None:
        payload = QuotaWindow(
            remaining=max(0, int(remaining)),
            total=max(0, int(total if total is not None else remaining)),
            window_seconds=window_seconds,
            reset_at=reset_at,
            synced_at=synced_at,
            source=source,
        )
        setattr(self, mode, payload)


def default_quota_set(default_quota: int, *, heavy_supported: bool = False, grok_4_3_supported: bool = False) -> TokenQuotaSet:
    """根据池类型构造默认额度集合"""
    quota = TokenQuotaSet(
        auto=QuotaWindow(remaining=default_quota, total=default_quota),
        fast=QuotaWindow(remaining=default_quota, total=default_quota),
        expert=QuotaWindow(remaining=default_quota, total=default_quota),
    )
    if heavy_supported:
        quota.heavy = QuotaWindow(remaining=20, total=20, window_seconds=7200)
    if grok_4_3_supported:
        quota.grok_4_3 = QuotaWindow(remaining=default_quota, total=default_quota)
    return quota


class TokenInfo(BaseModel):
    """Token 信息"""

    token: str
    status: TokenStatus = TokenStatus.ACTIVE
    quota: int | TokenQuotaSet = BASIC__DEFAULT_QUOTA

    # 统计
    created_at: int = Field(
        default_factory=lambda: int(datetime.now().timestamp() * 1000)
    )
    last_used_at: Optional[int] = None
    use_count: int = 0

    # 失败追踪
    fail_count: int = 0
    last_fail_at: Optional[int] = None
    last_fail_reason: Optional[str] = None

    # 冷却管理
    last_sync_at: Optional[int] = None  # 上次同步时间

    # 扩展
    tags: List[str] = Field(default_factory=list)
    note: str = ""
    last_asset_clear_at: Optional[int] = None

    @field_validator("quota", mode="before")
    @classmethod
    def _normalize_quota(cls, value):
        if isinstance(value, TokenQuotaSet):
            return value
        if isinstance(value, dict):
            if any(k in value for k in ("auto", "fast", "expert", "heavy", "grok_4_3")):
                return value
            if "remaining" in value or "total" in value:
                remaining = int(value.get("remaining", 0))
                total = int(value.get("total", remaining))
                return {
                    "auto": {"remaining": remaining, "total": total},
                    "fast": {"remaining": remaining, "total": total},
                    "expert": {"remaining": remaining, "total": total},
                }
        if value is None:
            value = BASIC__DEFAULT_QUOTA
        quota = max(0, int(value))
        return {
            "auto": {"remaining": quota, "total": quota},
            "fast": {"remaining": quota, "total": quota},
            "expert": {"remaining": quota, "total": quota},
        }

    def quota_set(self) -> TokenQuotaSet:
        if isinstance(self.quota, TokenQuotaSet):
            return self.quota
        if isinstance(self.quota, dict):
            self.quota = TokenQuotaSet.model_validate(self.quota)
            return self.quota
        self.quota = default_quota_set(int(self.quota))
        return self.quota

    def primary_quota(self) -> int:
        return self.quota_set().primary_remaining()

    def mode_quota(self, mode: str = "auto") -> int:
        return self.quota_set().mode_remaining(mode)

    def total_quota(self) -> int:
        return self.quota_set().total_remaining()

    def is_available(self) -> bool:
        """检查是否可用（状态正常且配额 > 0）"""
        return self.status == TokenStatus.ACTIVE and self.primary_quota() > 0

    def is_mode_available(self, mode: str = "auto") -> bool:
        """检查指定额度窗口是否可用。"""
        return self.status == TokenStatus.ACTIVE and self.mode_quota(mode) > 0

    def consume(self, effort: EffortType = EffortType.LOW) -> int:
        """
        消耗配额

        Args:
            effort: LOW 扣 1 配额并计 1 次，HIGH 扣 4 配额并计 4 次

        Returns:
            实际扣除的配额
        """
        cost = EFFORT_COST[effort]
        quota_set = self.quota_set()
        current = quota_set.primary_remaining()
        actual_cost = min(cost, current)

        self.last_used_at = int(datetime.now().timestamp() * 1000)
        self.use_count += actual_cost  # 使用 actual_cost 避免配额不足时过度计数
        quota_set.set_mode("auto", current - actual_cost, quota_set.auto.total)

        # 注意：不在这里清零 fail_count，只有 record_success() 才清零
        # 这样可以避免失败后调用 consume 导致失败计数被重置

        if self.primary_quota() == 0:
            self.status = TokenStatus.COOLING
        elif self.status == TokenStatus.COOLING:
            # 只从 COOLING 恢复，不从 EXPIRED 恢复
            self.status = TokenStatus.ACTIVE

        return actual_cost

    def update_quota(
        self,
        new_quota: int,
        *,
        mode: str = "auto",
        total: Optional[int] = None,
        window_seconds: Optional[int] = None,
        reset_at: Optional[int] = None,
        synced_at: Optional[int] = None,
        source: Optional[str] = None,
    ):
        """
        更新配额（用于 API 同步）

        Args:
            new_quota: 新的配额值
        """
        quota_set = self.quota_set()
        old_total = None
        current_window = quota_set.get_mode(mode)
        if current_window:
            old_total = current_window.total
        quota_set.set_mode(
            mode,
            new_quota,
            total if total is not None else old_total,
            window_seconds=window_seconds,
            reset_at=reset_at,
            synced_at=synced_at,
            source=source,
        )

        if self.primary_quota() == 0:
            self.status = TokenStatus.COOLING
        elif self.primary_quota() > 0 and self.status in [
            TokenStatus.COOLING,
            TokenStatus.EXPIRED,
        ]:
            self.status = TokenStatus.ACTIVE

    def reset(self, default_quota: Optional[int] = None):
        """重置配额到默认值"""
        quota = BASIC__DEFAULT_QUOTA if default_quota is None else default_quota
        quota = max(0, int(quota))
        has_heavy = self.quota_set().heavy is not None
        has_grok_4_3 = self.quota_set().grok_4_3 is not None
        self.quota = default_quota_set(
            quota,
            heavy_supported=has_heavy,
            grok_4_3_supported=has_grok_4_3,
        )
        self.status = TokenStatus.ACTIVE
        self.fail_count = 0
        self.last_fail_reason = None

    def record_fail(
        self,
        status_code: int = 401,
        reason: str = "",
        threshold: Optional[int] = None,
    ):
        """记录失败，达到阈值后自动标记为 expired"""
        # 仅 401 计入失败
        if status_code != 401:
            return

        self.fail_count += 1
        self.last_fail_at = int(datetime.now().timestamp() * 1000)
        self.last_fail_reason = reason

        limit = FAIL_THRESHOLD if threshold is None else threshold
        if self.fail_count >= limit:
            self.status = TokenStatus.EXPIRED

    def record_success(self, is_usage: bool = True):
        """记录成功，清空失败计数并根据配额更新状态"""
        self.fail_count = 0
        self.last_fail_at = None
        self.last_fail_reason = None

        if is_usage:
            self.use_count += 1
            self.last_used_at = int(datetime.now().timestamp() * 1000)

        if self.primary_quota() == 0:
            self.status = TokenStatus.COOLING
        else:
            self.status = TokenStatus.ACTIVE

    def need_refresh(self, interval_hours: int = 8) -> bool:
        """检查是否需要刷新配额"""
        if self.status != TokenStatus.COOLING:
            return False

        if self.last_sync_at is None:
            return True

        now = int(datetime.now().timestamp() * 1000)
        interval_ms = interval_hours * 3600 * 1000
        return (now - self.last_sync_at) >= interval_ms

    def mark_synced(self):
        """标记已同步"""
        self.last_sync_at = int(datetime.now().timestamp() * 1000)


class TokenPoolStats(BaseModel):
    """Token 池统计"""

    total: int = 0
    active: int = 0
    disabled: int = 0
    expired: int = 0
    cooling: int = 0
    total_quota: int = 0
    avg_quota: float = 0.0


__all__ = [
    "TokenStatus",
    "TokenInfo",
    "TokenPoolStats",
    "EffortType",
    "EFFORT_COST",
    "BASIC__DEFAULT_QUOTA",
    "SUPER_DEFAULT_QUOTA",
    "FAIL_THRESHOLD",
]
