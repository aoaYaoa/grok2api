"""
Grok2API 应用入口

FastAPI 应用初始化和路由注册
"""

from contextlib import asynccontextmanager
import argparse
import os
import platform
import sys
from pathlib import Path
from typing import Dict, List
import asyncio

from dotenv import load_dotenv
import httpx

BASE_DIR = Path(__file__).resolve().parent
APP_DIR = BASE_DIR / "app"

# Ensure the project root is on sys.path (helps when Vercel sets a different CWD)
if str(BASE_DIR) not in sys.path:
    sys.path.insert(0, str(BASE_DIR))

env_file = BASE_DIR / ".env"
if env_file.exists():
    load_dotenv(env_file)

from fastapi import FastAPI, HTTPException  # noqa: E402
from fastapi.middleware.cors import CORSMiddleware  # noqa: E402
from fastapi import Depends  # noqa: E402

from app.core.auth import verify_api_key  # noqa: E402
from app.core.config import get_config  # noqa: E402
from app.core.logger import logger, setup_logging  # noqa: E402
from app.core.exceptions import register_exception_handlers  # noqa: E402
from app.core.response_middleware import ResponseLoggerMiddleware  # noqa: E402
from app.api.v1.chat import router as chat_router  # noqa: E402
from app.api.v1.image import router as image_router  # noqa: E402
from app.api.v1.nsfw import router as nsfw_router  # noqa: E402
from app.api.v1.files import router as files_router  # noqa: E402
from app.api.v1.models import router as models_router  # noqa: E402
from app.api.v1.response import router as responses_router  # noqa: E402
from app.services.token import get_scheduler  # noqa: E402
from app.api.v1.admin_api import router as admin_router
from app.api.v1.public_api import router as public_router
from app.api.v1.video_api import router as video_router
from app.api.pages import router as pages_router
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse

# 初始化日志
setup_logging(
    level=os.getenv("LOG_LEVEL", "INFO"), json_console=False, file_logging=True
)


FFMPEG_VENDOR_DIR = APP_DIR / "static" / "vendor" / "ffmpeg"
FFMPEG_VENDOR_ASSETS: Dict[str, List[str]] = {
    "ffmpeg.js": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/ffmpeg@0.12.10/dist/umd/ffmpeg.js",
        "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/umd/ffmpeg.js",
    ],
    "814.ffmpeg.js": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/ffmpeg@0.12.10/dist/umd/814.ffmpeg.js",
        "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/umd/814.ffmpeg.js",
    ],
    "ffmpeg.js.map": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/ffmpeg@0.12.10/dist/umd/ffmpeg.js.map",
        "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/umd/ffmpeg.js.map",
    ],
    "814.ffmpeg.js.map": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/ffmpeg@0.12.10/dist/umd/814.ffmpeg.js.map",
        "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/umd/814.ffmpeg.js.map",
    ],
    "ffmpeg-util.js": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/util@0.12.1/dist/umd/index.js",
        "https://unpkg.com/@ffmpeg/util@0.12.1/dist/umd/index.js",
    ],
    "ffmpeg-core.js": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/core@0.12.6/dist/umd/ffmpeg-core.js",
        "https://unpkg.com/@ffmpeg/core@0.12.6/dist/umd/ffmpeg-core.js",
    ],
    "ffmpeg-core.wasm": [
        "https://cdn.jsdelivr.net/npm/@ffmpeg/core@0.12.6/dist/umd/ffmpeg-core.wasm",
        "https://unpkg.com/@ffmpeg/core@0.12.6/dist/umd/ffmpeg-core.wasm",
    ],
}


async def _download_first_success(
    client: httpx.AsyncClient, filename: str, urls: List[str], target_path: Path
) -> bool:
    for url in urls:
        try:
            resp = await client.get(url, follow_redirects=True)
            if resp.status_code == 200 and resp.content:
                target_path.write_bytes(resp.content)
                logger.info(
                    f"FFmpeg vendor downloaded: {filename} <- {url} ({len(resp.content)} bytes)"
                )
                return True
            logger.warning(
                f"FFmpeg vendor candidate failed: {filename} <- {url} status={resp.status_code}"
            )
        except Exception as e:
            logger.warning(f"FFmpeg vendor download error: {filename} <- {url}, error={e}")
    return False


async def ensure_ffmpeg_vendor_assets() -> None:
    """启动时预热 ffmpeg 前端依赖到本地静态目录，避免浏览器跨域/CORS 问题。"""
    FFMPEG_VENDOR_DIR.mkdir(parents=True, exist_ok=True)
    timeout = httpx.Timeout(connect=8.0, read=30.0, write=30.0, pool=8.0)
    async with httpx.AsyncClient(timeout=timeout) as client:
        tasks = []
        pending_names: list[str] = []
        for filename, urls in FFMPEG_VENDOR_ASSETS.items():
            target = FFMPEG_VENDOR_DIR / filename
            if target.exists() and target.stat().st_size > 0:
                logger.info(f"FFmpeg vendor exists: {filename}")
                continue
            pending_names.append(filename)
            tasks.append(_download_first_success(client, filename, urls, target))

        if not tasks:
            return
        results = await asyncio.gather(*tasks, return_exceptions=False)
        failed = [name for name, ok in zip(pending_names, results) if not ok]
        if failed:
            logger.warning(f"FFmpeg vendor partial ready, failed={failed}")


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    # 1. 注册服务默认配置
    from app.core.config import config, register_defaults
    from app.services.grok.defaults import get_grok_defaults

    register_defaults(get_grok_defaults())

    # 2. 加载配置
    await config.load()

    # 3. 启动服务显示
    logger.info("Starting Grok2API...")
    logger.info(f"Platform: {platform.system()} {platform.release()}")
    logger.info(f"Python: {sys.version.split()[0]}")
    await ensure_ffmpeg_vendor_assets()

    # 4. 启动 Token 刷新调度器
    refresh_enabled = get_config("token.auto_refresh", True)
    if refresh_enabled:
        basic_interval = get_config("token.refresh_interval_hours", 8)
        super_interval = get_config("token.super_refresh_interval_hours", 2)
        interval = min(basic_interval, super_interval)
        scheduler = get_scheduler(interval)
        scheduler.start()

    # 5. 启动 cf_clearance 自动刷新
    #    环境变量 FLARESOLVERR_URL 会作为初始值写入配置（兼容旧部署方式）
    _flaresolverr_env = os.getenv("FLARESOLVERR_URL", "")
    if _flaresolverr_env and not get_config("proxy.flaresolverr_url"):
        await config.update({
            "proxy": {
                "enabled": True,
                "flaresolverr_url": _flaresolverr_env,
                "refresh_interval": int(os.getenv("CF_REFRESH_INTERVAL", "600")),
                "timeout": int(os.getenv("CF_TIMEOUT", "60")),
            }
        })

    from app.services.cf_refresh import start as cf_refresh_start
    cf_refresh_start()

    logger.info("Application startup complete.")
    yield

    # 关闭
    logger.info("Shutting down Grok2API...")

    from app.services.cf_refresh import stop as cf_refresh_stop
    cf_refresh_stop()

    from app.core.storage import StorageFactory

    if StorageFactory._instance:
        await StorageFactory._instance.close()

    if refresh_enabled:
        scheduler = get_scheduler()
        scheduler.stop()


def create_app() -> FastAPI:
    """创建 FastAPI 应用"""
    app = FastAPI(
        title="Grok2API",
        lifespan=lifespan,
    )

    # CORS 配置
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # 请求日志和 ID 中间件
    app.add_middleware(ResponseLoggerMiddleware)

    # 注册异常处理器
    register_exception_handlers(app)

    # 注册路由
    app.include_router(
        chat_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    app.include_router(
        image_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    app.include_router(
        nsfw_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    app.include_router(
        models_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    app.include_router(
        responses_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    # 兼容部分客户端直接请求 /responses（不带 /v1）
    app.include_router(
        responses_router, dependencies=[Depends(verify_api_key)]
    )
    app.include_router(
        video_router, prefix="/v1", dependencies=[Depends(verify_api_key)]
    )
    app.include_router(files_router, prefix="/v1/files")

    # 静态文件服务
    static_dir = APP_DIR / "static"
    if static_dir.exists():
        app.mount("/static", StaticFiles(directory=static_dir), name="static")

    # 注册管理与公共路由
    app.include_router(admin_router, prefix="/v1/admin")
    app.include_router(public_router, prefix="/v1/public")
    app.include_router(pages_router)

    @app.get("/favicon.ico", include_in_schema=False)
    async def favicon():
        file_path = static_dir / "common/img/favicon/favicon.ico"
        if not file_path.exists():
            raise HTTPException(status_code=404, detail="Not Found")
        return FileResponse(file_path, media_type="image/x-icon")

    @app.get("/health", include_in_schema=False)
    async def health():
        return {"status": "ok"}

    return app


app = create_app()


if __name__ == "__main__":
    host = os.getenv("SERVER_HOST", "0.0.0.0")
    port = int(os.getenv("SERVER_PORT", "8000"))
    workers = int(os.getenv("SERVER_WORKERS", "1"))
    log_level = os.getenv("LOG_LEVEL", "INFO").lower()
    logger.error(
        "Direct startup via `python main.py` is disabled. "
        "Please run with Granian CLI to avoid Python wrapper issues."
    )
    logger.error(
        "Use: uv run granian --interface asgi "
        f"--host {host} --port {port} --workers {workers} "
        f"--log-level {log_level} main:app"
    )
    raise SystemExit(1)
