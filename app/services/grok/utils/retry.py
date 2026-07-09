"""
Retry helpers for token switching.
"""

from typing import Optional, Set

from app.core.config import get_config
from app.core.exceptions import UpstreamException
from app.services.grok.services.model import ModelService


async def pick_token(
    token_mgr,
    model_id: str,
    tried: Set[str],
    preferred: Optional[str] = None,
) -> Optional[str]:
    quota_mode = ModelService.quota_mode_for_model(model_id)
    if preferred and preferred not in tried:
        pool_name = token_mgr.get_pool_name_for_token(preferred)
        if not pool_name:
            return preferred
        pool = token_mgr.pools.get(pool_name)
        token_info = pool.get(preferred) if pool else None
        if token_info and token_info.is_mode_available(quota_mode):
            return preferred

    token = None
    for pool_name in ModelService.pool_candidates_for_model(model_id):
        token = token_mgr.get_token(pool_name, exclude=tried, quota_mode=quota_mode)
        if token:
            break

    if not token and not tried:
        result = await token_mgr.refresh_cooling_tokens()
        if result.get("recovered", 0) > 0:
            for pool_name in ModelService.pool_candidates_for_model(model_id):
                token = token_mgr.get_token(pool_name, quota_mode=quota_mode)
                if token:
                    break

    if (
        not token
        and not tried
        and bool(get_config("token.refresh_unavailable_once", False))
    ):
        pool_candidates = ModelService.pool_candidates_for_model(model_id)
        result = await token_mgr.refresh_unavailable_tokens(pool_candidates)
        if result.get("recovered", 0) > 0:
            for pool_name in pool_candidates:
                token = token_mgr.get_token(pool_name, quota_mode=quota_mode)
                if token:
                    break

    return token


def rate_limited(error: Exception) -> bool:
    if not isinstance(error, UpstreamException):
        return False
    status = error.details.get("status") if error.details else None
    code = error.details.get("error_code") if error.details else None
    return status == 429 or code == "rate_limit_exceeded"


__all__ = ["pick_token", "rate_limited"]
