"""
文件服务 API 路由
"""

import aiofiles.os
from urllib.parse import quote, unquote
from pathlib import Path
from curl_cffi.requests import AsyncSession
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import FileResponse, Response

from app.core.config import get_config
from app.core.logger import logger
from app.core.storage import DATA_DIR
from app.services.reverse.utils.headers import build_sso_cookie
from app.services.token.service import TokenService

router = APIRouter(tags=["Files"])

# 缓存根目录
BASE_DIR = DATA_DIR / "tmp"
IMAGE_DIR = BASE_DIR / "image"
VIDEO_DIR = BASE_DIR / "video"
FILE_DIR = BASE_DIR / "file"


def _safe_asset_path(path: str) -> str:
    value = (path or "").strip()
    try:
        value = unquote(value)
    except Exception:
        pass
    value = value.strip().lstrip("/")
    if not value or "://" in value or value.startswith("\\") or ".." in value.split("/"):
        raise HTTPException(status_code=400, detail="Invalid asset path")
    return quote(value, safe="/:@-._~!$&'()*+,;=")


def _download_filename(path: str) -> str:
    try:
        decoded = unquote(path)
    except Exception:
        decoded = path
    name = decoded.rsplit("/", 1)[-1].strip() or "download"
    return name.replace("\\", "-").replace("/", "-")


def _asset_user_id(safe_path: str) -> str:
    parts = safe_path.split("/")
    if len(parts) >= 2 and parts[0] == "users":
        return parts[1]
    return ""


def _build_asset_headers(safe_path: str, token: str | None, range_header: str | None) -> dict[str, str]:
    headers = {
        "Accept": "*/*",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Cache-Control": "no-cache",
        "Origin": "https://grok.com",
        "Pragma": "no-cache",
        "Priority": "u=1, i",
        "Referer": "https://grok.com/",
        "Sec-Fetch-Dest": "empty",
        "Sec-Fetch-Mode": "cors",
        "Sec-Fetch-Site": "same-site",
        "User-Agent": str(
            get_config("proxy.user_agent")
            or "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
        ),
    }
    if token:
        cookie = build_sso_cookie(token)
        user_id = _asset_user_id(safe_path)
        if user_id and "x-userid=" not in cookie:
            cookie = f"{cookie}; x-userid={user_id}"
        headers["Cookie"] = cookie
    if range_header:
        headers["range"] = range_header
    return headers


async def _asset_token_candidates() -> list[str | None]:
    tokens: list[str | None] = [None]
    seen: set[str] = set()
    for pool_name in ("ssoBasic", "ssoSuper"):
        try:
            pool_tokens = await TokenService.list_tokens(pool_name)
        except Exception:
            continue
        for item in pool_tokens:
            token = (getattr(item, "token", "") or "").strip()
            if not token or token in seen:
                continue
            seen.add(token)
            tokens.append(token)
    return tokens


def _response_headers(upstream_headers: dict, filename: str) -> dict[str, str]:
    headers = {
        "Cache-Control": "public, max-age=31536000, immutable",
        "Content-Disposition": f"inline; filename*=UTF-8''{quote(filename)}",
    }
    for key in ("content-length", "content-range", "accept-ranges"):
        value = upstream_headers.get(key) or upstream_headers.get(key.title())
        if value:
            headers[key.title()] = str(value)
    return headers


def _guess_local_content_type(file_path: Path, fallback: str = "application/octet-stream") -> str:
    suffix = file_path.suffix.lower()
    return {
        ".jpg": "image/jpeg",
        ".jpeg": "image/jpeg",
        ".png": "image/png",
        ".webp": "image/webp",
        ".gif": "image/gif",
        ".mp4": "video/mp4",
        ".webm": "video/webm",
        ".mov": "video/quicktime",
        ".zip": "application/zip",
        ".pdf": "application/pdf",
        ".txt": "text/plain; charset=utf-8",
        ".json": "application/json",
    }.get(suffix, fallback)


def _normalize_cached_filename(filename: str) -> str:
    """
    规范化客户端传入文件名，兼容尾部误带反斜杠等情况。
    """
    value = (filename or "").strip()
    # 尝试解码一次，兼容 %5C 这类编码
    try:
        value = unquote(value)
    except Exception:
        pass
    value = value.strip().strip('"').strip("'").rstrip("\\/")
    # 将路径分隔符统一扁平化到缓存命名规则
    value = value.replace("\\", "-").replace("/", "-")
    return value


@router.get("/image/{filename:path}")
async def get_image(filename: str):
    """
    获取图片文件
    """
    filename = _normalize_cached_filename(filename)

    file_path = IMAGE_DIR / filename

    if await aiofiles.os.path.exists(file_path):
        if await aiofiles.os.path.isfile(file_path):
            content_type = "image/jpeg"
            if file_path.suffix.lower() == ".png":
                content_type = "image/png"
            elif file_path.suffix.lower() == ".webp":
                content_type = "image/webp"

            # 增加缓存头，支持高并发场景下的浏览器/CDN缓存
            return FileResponse(
                file_path,
                media_type=content_type,
                headers={"Cache-Control": "public, max-age=31536000, immutable"},
            )

    logger.warning(f"Image not found: {filename}")
    raise HTTPException(status_code=404, detail="Image not found")


@router.get("/video/{filename:path}")
async def get_video(filename: str):
    """
    获取视频文件
    """
    filename = _normalize_cached_filename(filename)

    file_path = VIDEO_DIR / filename

    if await aiofiles.os.path.exists(file_path):
        if await aiofiles.os.path.isfile(file_path):
            return FileResponse(
                file_path,
                media_type="video/mp4",
                headers={"Cache-Control": "public, max-age=31536000, immutable"},
            )

    logger.warning(f"Video not found: {filename}")
    raise HTTPException(status_code=404, detail="Video not found")


@router.get("/file/{filename:path}")
async def get_file(filename: str):
    """
    获取返回文件。
    """
    filename = _normalize_cached_filename(filename)

    file_path = FILE_DIR / filename

    if await aiofiles.os.path.exists(file_path):
        if await aiofiles.os.path.isfile(file_path):
            return FileResponse(
                file_path,
                media_type=_guess_local_content_type(file_path),
                filename=file_path.name,
                headers={"Cache-Control": "public, max-age=31536000, immutable"},
            )

    logger.warning(f"File not found: {filename}")
    raise HTTPException(status_code=404, detail="File not found")


@router.get("/asset/{path:path}")
async def get_asset(path: str, request: Request):
    """
    转发 Grok 返回的文件资源。
    """
    safe_path = _safe_asset_path(path)
    url = f"https://assets.grok.com/{safe_path}"
    proxy_url = get_config("proxy.asset_proxy_url") or get_config("proxy.base_proxy_url")
    proxies = {"http": proxy_url, "https": proxy_url} if proxy_url else None
    range_header = request.headers.get("range")

    session = AsyncSession()
    try:
        last_response = None
        browser = get_config("proxy.browser")
        for token in await _asset_token_candidates():
            headers = _build_asset_headers(safe_path, token, range_header)
            try:
                request_kwargs = {
                    "headers": headers,
                    "proxies": proxies,
                    "timeout": get_config("asset.download_timeout"),
                    "allow_redirects": True,
                }
                if browser:
                    request_kwargs["impersonate"] = browser
                response = await session.get(url, **request_kwargs)
            except Exception as exc:
                logger.warning(f"Asset proxy request failed: {safe_path} {exc}")
                continue
            if response.status_code < 400:
                break
            last_response = response
        else:
            status_code = getattr(last_response, "status_code", 502)
            logger.warning(f"Asset proxy failed: {status_code} {safe_path}")
            raise HTTPException(status_code=status_code, detail="Asset not found")

        content_type = response.headers.get("content-type", "application/octet-stream")
        return Response(
            content=response.content,
            status_code=response.status_code,
            media_type=content_type,
            headers=_response_headers(response.headers, _download_filename(safe_path)),
        )
    finally:
        await session.close()
