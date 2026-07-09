"""
Reverse interface: rate limits.
"""

import orjson
from typing import Any
from curl_cffi.requests import AsyncSession

from app.core.logger import logger
from app.core.config import get_config
from app.core.exceptions import UpstreamException
from app.services.reverse.utils.headers import build_headers
from app.services.reverse.utils.retry import retry_on_status

RATE_LIMITS_API = "https://grok.com/rest/rate-limits"

MODE_NAME_BY_KEY = {
    "auto": "auto",
    "fast": "fast",
    "expert": "expert",
    "heavy": "heavy",
    "grok_4_3": "grok-420-computer-use-sa",
}


class RateLimitsReverse:
    """/rest/rate-limits reverse interface."""

    @staticmethod
    async def request(
        session: AsyncSession,
        token: str,
        *,
        model_name: str = "auto",
    ) -> Any:
        """Fetch rate limits from Grok.

        Args:
            session: AsyncSession, the session to use for the request.
            token: str, the SSO token.

        Returns:
            Any: The response from the request.
        """
        try:
            # Get proxies
            base_proxy = get_config("proxy.base_proxy_url")
            proxies = {"http": base_proxy, "https": base_proxy} if base_proxy else None

            # Build headers
            headers = build_headers(
                cookie_token=token,
                content_type="application/json",
                origin="https://grok.com",
                referer="https://grok.com/",
            )

            # Build payload
            payload = {
                "requestKind": "DEFAULT",
                "modelName": model_name,
            }

            # Curl Config
            timeout = get_config("usage.timeout")
            browser = get_config("proxy.browser")

            async def _do_request():
                response = await session.post(
                    RATE_LIMITS_API,
                    headers=headers,
                    data=orjson.dumps(payload),
                    timeout=timeout,
                    proxies=proxies,
                    impersonate=browser,
                )

                if response.status_code != 200:
                    logger.error(
                        f"RateLimitsReverse: Request failed, {response.status_code}",
                        extra={"error_type": "UpstreamException"},
                    )
                    raise UpstreamException(
                        message=f"RateLimitsReverse: Request failed, {response.status_code}",
                        details={"status": response.status_code},
                    )

                return response

            return await retry_on_status(_do_request)

        except Exception as e:
            # Handle upstream exception
            if isinstance(e, UpstreamException):
                status = None
                if e.details and "status" in e.details:
                    status = e.details["status"]
                else:
                    status = getattr(e, "status_code", None)
                raise

            # Handle other non-upstream exceptions
            logger.error(
                f"RateLimitsReverse: Request failed, {str(e)}",
                extra={"error_type": type(e).__name__},
            )
            raise UpstreamException(
                message=f"RateLimitsReverse: Request failed, {str(e)}",
                details={"status": 502, "error": str(e)},
            )


async def fetch_quota_windows(
    session: AsyncSession,
    token: str,
    *,
    mode_keys: tuple[str, ...],
) -> dict[str, dict]:
    """逐模型获取额度窗口，兼容旧版 TokenManager。"""
    windows: dict[str, dict] = {}
    for mode_key in mode_keys:
        model_name = MODE_NAME_BY_KEY.get(mode_key)
        if not model_name:
            continue
        response = await RateLimitsReverse.request(
            session,
            token,
            model_name=model_name,
        )
        data = response.json()
        remaining = data.get("remainingTokens")
        if remaining is None:
            remaining = data.get("remainingQueries")
        total = data.get("totalTokens")
        if total is None:
            total = data.get("totalQueries")
        windows[mode_key] = {
            "remaining": int(remaining or 0),
            "total": int(total if total is not None else remaining or 0),
            "window_seconds": data.get("windowSizeSeconds"),
        }
    return windows


__all__ = ["RateLimitsReverse", "fetch_quota_windows", "MODE_NAME_BY_KEY"]
