"""Shared header builders for reverse interfaces."""

import json
import re
import uuid
import orjson
from urllib.parse import urlparse
from typing import Dict, Optional

from app.core.logger import logger
from app.core.config import get_config
from app.services.reverse.browser_bridge import get_browser_session
from app.services.reverse.utils.statsig import StatsigGenerator

_HEADER_CHAR_REPLACEMENTS = str.maketrans(
    {
        "\u2010": "-",
        "\u2011": "-",
        "\u2012": "-",
        "\u2013": "-",
        "\u2014": "-",
        "\u2212": "-",
        "\u2018": "'",
        "\u2019": "'",
        "\u201c": '"',
        "\u201d": '"',
        "\u00a0": " ",
        "\u2007": " ",
        "\u202f": " ",
        "\u200b": "",
        "\u200c": "",
        "\u200d": "",
        "\ufeff": "",
    }
)


def _configured_session_cookie_header() -> str:
    raw = str(get_config("cloakbrowser.session_cookies_json", "") or "").strip()
    if not raw:
        return ""
    try:
        cookies = json.loads(raw)
    except Exception:
        return ""
    if not isinstance(cookies, list):
        return ""
    parts: list[str] = []
    seen: set[str] = set()
    for item in cookies:
        if not isinstance(item, dict):
            continue
        name = str(item.get("name") or "").strip()
        value = str(item.get("value") or "")
        domain = str(item.get("domain") or "").strip().lower()
        if not name or not value:
            continue
        if domain and "grok.com" not in domain:
            continue
        if name in seen:
            continue
        seen.add(name)
        parts.append(f"{name}={value}")
    return "; ".join(parts)


def _sanitize_header_value(
    value: Optional[str],
    *,
    field_name: str,
    remove_all_spaces: bool = False,
) -> str:
    """Normalize header values and make sure they are latin-1 safe."""
    raw = "" if value is None else str(value)
    normalized = raw.translate(_HEADER_CHAR_REPLACEMENTS)
    if remove_all_spaces:
        normalized = re.sub(r"\s+", "", normalized)
    else:
        normalized = normalized.strip()

    normalized = normalized.encode("latin-1", errors="ignore").decode("latin-1")

    if normalized != raw:
        logger.warning(
            f"Sanitized header field '{field_name}' (len {len(raw)} -> {len(normalized)})"
        )
    return normalized


def build_sso_cookie(sso_token: str) -> str:
    sso_token = sso_token[4:] if sso_token.startswith("sso=") else sso_token
    sso_token = _sanitize_header_value(
        sso_token, field_name="sso_token", remove_all_spaces=True
    )

    cookie = f"sso={sso_token}; sso-rw={sso_token}"

    cf_cookies = _sanitize_header_value(
        get_config("proxy.cf_cookies") or "", field_name="proxy.cf_cookies"
    )
    cf_clearance = _sanitize_header_value(
        get_config("proxy.cf_clearance") or "",
        field_name="proxy.cf_clearance",
        remove_all_spaces=True,
    )
    if cf_clearance and cf_cookies:
        if re.search(r"(?:^|;\s*)cf_clearance=", cf_cookies):
            cf_cookies = re.sub(
                r"(^|;\s*)cf_clearance=[^;]*",
                r"\1cf_clearance=" + cf_clearance,
                cf_cookies,
                count=1,
            )
        else:
            cf_cookies = cf_cookies.rstrip("; ")
            cf_cookies = f"{cf_cookies}; cf_clearance={cf_clearance}"
    elif cf_clearance:
        cf_cookies = f"cf_clearance={cf_clearance}"
    if cf_cookies:
        if cookie and not cookie.endswith(";"):
            cookie += "; "
        cookie += cf_cookies

    return cookie


def _extract_major_version(browser: Optional[str], user_agent: Optional[str]) -> Optional[str]:
    if browser:
        match = re.search(r"(\d{2,3})", browser)
        if match:
            return match.group(1)
    if user_agent:
        for pattern in [r"Edg/(\d+)", r"Chrome/(\d+)", r"Chromium/(\d+)"]:
            match = re.search(pattern, user_agent)
            if match:
                return match.group(1)
    return None


def _detect_platform(user_agent: str) -> Optional[str]:
    ua = user_agent.lower()
    if "windows" in ua:
        return "Windows"
    if "mac os x" in ua or "macintosh" in ua:
        return "macOS"
    if "android" in ua:
        return "Android"
    if "iphone" in ua or "ipad" in ua:
        return "iOS"
    if "linux" in ua:
        return "Linux"
    return None


def _detect_arch(user_agent: str) -> Optional[str]:
    ua = user_agent.lower()
    if "aarch64" in ua or "arm" in ua:
        return "arm"
    if "x86_64" in ua or "x64" in ua or "win64" in ua or "intel" in ua:
        return "x86"
    return None


def _build_client_hints(browser: Optional[str], user_agent: Optional[str]) -> Dict[str, str]:
    browser = (browser or "").strip().lower()
    user_agent = user_agent or ""
    ua = user_agent.lower()

    is_edge = "edge" in browser or "edg" in ua
    is_brave = "brave" in browser
    is_chromium = any(key in browser for key in ["chrome", "chromium", "edge", "brave"]) or (
        "chrome" in ua or "chromium" in ua or "edg" in ua
    )
    is_firefox = "firefox" in ua or "firefox" in browser
    is_safari = ("safari" in ua and "chrome" not in ua and "chromium" not in ua and "edg" not in ua) or "safari" in browser

    if not is_chromium or is_firefox or is_safari:
        return {}

    version = _extract_major_version(browser, user_agent)
    if not version:
        return {}

    if is_edge:
        brand = "Microsoft Edge"
    elif "chromium" in browser:
        brand = "Chromium"
    elif is_brave:
        brand = "Brave"
    else:
        brand = "Google Chrome"

    sec_ch_ua = (
        f"\"{brand}\";v=\"{version}\", "
        f"\"Chromium\";v=\"{version}\", "
        "\"Not(A:Brand\";v=\"24\""
    )

    platform = _detect_platform(user_agent)
    arch = _detect_arch(user_agent)
    mobile = "?1" if ("mobile" in ua or platform in ("Android", "iOS")) else "?0"

    hints = {
        "Sec-Ch-Ua": sec_ch_ua,
        "Sec-Ch-Ua-Mobile": mobile,
    }
    if platform:
        hints["Sec-Ch-Ua-Platform"] = f"\"{platform}\""
    if arch:
        hints["Sec-Ch-Ua-Arch"] = arch
        hints["Sec-Ch-Ua-Bitness"] = "64"
    hints["Sec-Ch-Ua-Model"] = "" if mobile == "?0" else ""
    return hints


def build_ws_headers(token: Optional[str] = None, origin: Optional[str] = None, extra: Optional[Dict[str, str]] = None) -> Dict[str, str]:
    user_agent = _sanitize_header_value(
        get_config("proxy.user_agent"), field_name="proxy.user_agent"
    )
    if not user_agent:
        user_agent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
    safe_origin = _sanitize_header_value(origin or "https://grok.com", field_name="origin")
    headers = {
        "Origin": safe_origin,
        "User-Agent": user_agent,
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Cache-Control": "no-cache",
        "Pragma": "no-cache",
    }

    client_hints = _build_client_hints(get_config("proxy.browser"), user_agent)
    if client_hints:
        headers.update(client_hints)

    configured_cookie_header = _configured_session_cookie_header()
    if configured_cookie_header:
        headers["Cookie"] = configured_cookie_header
    elif token:
        headers["Cookie"] = build_sso_cookie(token)

    if extra:
        headers.update(extra)

    return headers


def build_headers(
    cookie_token: str,
    content_type: Optional[str] = None,
    origin: Optional[str] = None,
    referer: Optional[str] = None,
    request_url: Optional[str] = None,
    method: Optional[str] = None,
) -> Dict[str, str]:
    browser_bridge_enabled = bool(get_config("cloakbrowser.enabled", False))
    sync_browser_session = bool(get_config("cloakbrowser.sync_session", False))
    use_browser_statsig = bool(
        get_config("proxy.statsig_use_browser_capture", False)
    )
    session = {}
    if browser_bridge_enabled and sync_browser_session:
        try:
            session = get_browser_session(cookie_token)
        except Exception as exc:
            logger.warning(f"Browser session header sync skipped: {exc}")
            configured_cookie_header = _configured_session_cookie_header()
            if configured_cookie_header:
                session = {"cookie_header": configured_cookie_header}
    user_agent = _sanitize_header_value(
        session.get("user_agent") or get_config("proxy.user_agent"),
        field_name="proxy.user_agent",
    )
    if not user_agent:
        user_agent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
    safe_origin = _sanitize_header_value(origin or "https://grok.com", field_name="origin")
    safe_referer = _sanitize_header_value(
        referer or "https://grok.com/", field_name="referer"
    )
    headers = {
        "Accept-Encoding": "gzip, deflate, br, zstd",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Baggage": "sentry-environment=production,sentry-release=d6add6fb0460641fd482d767a335ef72b9b6abb8,sentry-public_key=b311e0f2690c81f25e2c4cf6d4f7ce1c",
        "Origin": safe_origin,
        "Priority": "u=1, i",
        "Referer": safe_referer,
        "Sec-Fetch-Mode": "cors",
        "User-Agent": user_agent,
    }

    client_hints = _build_client_hints(get_config("proxy.browser"), user_agent)
    if client_hints:
        headers.update(client_hints)

    captured_headers = session.get("request_headers") if isinstance(session, dict) else None
    if isinstance(captured_headers, dict):
        for key, value in captured_headers.items():
            clean_key = str(key or "").strip()
            lower_key = clean_key.lower()
            if not clean_key or lower_key in {"cookie", "content-type", "content-length"}:
                continue
            if lower_key in {"x-xai-request-id", "x-statsig-id"}:
                continue
            headers[clean_key] = _sanitize_header_value(
                value, field_name=f"browser.request_headers.{clean_key}"
            )

    session_cookie_header = _sanitize_header_value(
        session.get("cookie_header") or "", field_name="browser.cookie_header"
    )
    configured_cookie_header = _configured_session_cookie_header()
    has_captured_session_headers = bool(
        session.get("x_statsig_id")
        or (isinstance(captured_headers, dict) and captured_headers)
    )
    if configured_cookie_header and (
        not session_cookie_header or not has_captured_session_headers
    ):
        cookie_header = configured_cookie_header
        cookie_source = "configured"
    elif session_cookie_header:
        cookie_header = session_cookie_header
        cookie_source = "browser"
    else:
        cookie_header = build_sso_cookie(cookie_token)
        cookie_source = "fallback"
    headers["Cookie"] = cookie_header

    if content_type and content_type == "application/json":
        headers["Content-Type"] = "application/json"
        headers["Accept"] = "*/*"
        headers["Sec-Fetch-Dest"] = "empty"
    elif content_type in ["image/jpeg", "image/png", "video/mp4", "video/webm"]:
        headers["Content-Type"] = content_type
        headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
        headers["Sec-Fetch-Dest"] = "document"
    else:
        headers["Content-Type"] = "application/json"
        headers["Accept"] = "*/*"
        headers["Sec-Fetch-Dest"] = "empty"

    origin_domain = urlparse(headers.get("Origin", "")).hostname
    referer_domain = urlparse(headers.get("Referer", "")).hostname
    if origin_domain and referer_domain and origin_domain == referer_domain:
        headers["Sec-Fetch-Site"] = "same-origin"
    else:
        headers["Sec-Fetch-Site"] = "same-site"

    manual_statsig = str(get_config("cloakbrowser.manual_statsig_id", "") or "").strip()
    captured_statsig = ""
    if use_browser_statsig:
        captured_statsig = str(
            session.get("x_statsig_id") or headers.get("x-statsig-id") or ""
        ).strip()

    req_method = (method or "POST").upper()
    req_url = request_url or "/rest/app-chat/conversations/new"
    headers["x-statsig-id"] = (
        manual_statsig
        or captured_statsig
        or StatsigGenerator.gen_id(
            cookie_token,
            request_url=req_url,
            method=req_method,
        )
    )
    using_manual_statsig = bool(manual_statsig)
    headers["x-xai-request-id"] = str(uuid.uuid4())

    safe_headers = dict(headers)
    if "Cookie" in safe_headers:
        safe_headers["Cookie"] = "<redacted>"
    for key in list(safe_headers.keys()):
        if str(key).lower() == "x-statsig-id":
            safe_headers[key] = f"<redacted len={len(headers.get('x-statsig-id') or '')}>"
    safe_headers["SessionSource"] = cookie_source
    safe_headers["SessionCookieLen"] = len(cookie_header or "")
    safe_headers["SessionHasStatsig"] = bool(session.get("x_statsig_id"))
    safe_headers["ManualStatsig"] = using_manual_statsig
    safe_headers["BrowserBridgeEnabled"] = browser_bridge_enabled
    safe_headers["BrowserSyncSession"] = sync_browser_session
    safe_headers["StatsigUseBrowserCapture"] = use_browser_statsig
    safe_headers["CapturedHeaderKeys"] = (
        sorted([str(key) for key in captured_headers.keys()])
        if isinstance(captured_headers, dict)
        else []
    )
    logger.info(f"Built headers: {orjson.dumps(safe_headers).decode()}")

    return headers


__all__ = ["build_headers", "build_sso_cookie", "build_ws_headers"]
