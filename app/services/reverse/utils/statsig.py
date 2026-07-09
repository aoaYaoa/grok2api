"""
Statsig ID generator for reverse interfaces.
"""

import base64
import random
import string
from urllib.parse import urlparse

from app.core.logger import logger
from app.core.config import get_config
from app.services.reverse.utils import statsig_pure


class StatsigGenerator:
    """Statsig ID generator for reverse interfaces."""

    @staticmethod
    def _rand(length: int, alphanumeric: bool = False) -> str:
        """Generate random string."""
        chars = (
            string.ascii_lowercase + string.digits
            if alphanumeric
            else string.ascii_lowercase
        )
        return "".join(random.choices(chars, k=length))

    @staticmethod
    def _legacy_fallback() -> str:
        if random.choice([True, False]):
            rand = StatsigGenerator._rand(5, alphanumeric=True)
            message = (
                f"x1:TypeError: Cannot read properties of null "
                f"(reading 'children[\\'{rand}\\']')"
            )
        else:
            rand = StatsigGenerator._rand(10)
            message = (
                f"x1:TypeError: Cannot read properties of undefined (reading '{rand}')"
            )
        return base64.b64encode(message.encode()).decode()

    @staticmethod
    def gen_id(
        token: str | None = None,
        *,
        pathname: str | None = None,
        method: str | None = None,
        request_url: str | None = None,
    ) -> str:
        """
        Generate Statsig ID.

        优先级：
        1. cloakbrowser.manual_statsig_id（手动覆盖）
        2. aurora 纯算法 statsig_pure.generate（默认）
        3. 旧版 TypeError 假值兜底
        """
        manual = str(get_config("cloakbrowser.manual_statsig_id", "") or "").strip()
        if manual:
            logger.debug("Using manual Statsig ID from config")
            return manual

        fixed = str(get_config("proxy.statsig_id", "") or "").strip()
        if fixed:
            logger.debug("Using fixed proxy.statsig_id from config")
            return fixed

        use_pure = bool(get_config("proxy.statsig_pure_enabled", True))
        if use_pure:
            try:
                statsig_pure.apply_pair_from_config()
                path = pathname or ""
                if not path and request_url:
                    parsed = urlparse(request_url)
                    path = parsed.path or ""
                if not path:
                    path = "/rest/app-chat/conversations/new"
                meth = (method or "POST").upper()
                value = statsig_pure.generate(path, meth)
                ok, _ = statsig_pure.verify_structure(value)
                if ok and len(value) >= 90:
                    logger.debug(
                        f"Generated pure Statsig ID for {meth} {path} (len={len(value)})"
                    )
                    return value
            except Exception as exc:
                logger.warning(f"Pure Statsig generation failed, fallback: {exc}")

        # 兼容旧开关：仅在显式关闭纯算法且开启动态时走旧逻辑
        if get_config("app.dynamic_statsig"):
            logger.debug("Generating legacy dynamic Statsig ID")
            return StatsigGenerator._legacy_fallback()

        logger.debug("Generating legacy static Statsig ID")
        return "ZTpUeXBlRXJyb3I6IENhbm5vdCByZWFkIHByb3BlcnRpZXMgb2YgdW5kZWZpbmVkIChyZWFkaW5nICdjaGlsZE5vZGVzJyk="


__all__ = ["StatsigGenerator"]
