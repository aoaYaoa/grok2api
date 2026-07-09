"""CloakBrowser bridge Node/Playwright 依赖自动检查与安装。"""
from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
import sys
from pathlib import Path

from app.core.config import get_config
from app.core.logger import logger

BASE_DIR = Path(__file__).resolve().parent
_INSTALL_LOCK = asyncio.Lock()
_INSTALL_DONE = False


def _node_binary() -> str:
    return str(get_config("cloakbrowser.node_binary", "node") or "node").strip() or "node"


def _npm_binary() -> str:
    configured = str(get_config("cloakbrowser.npm_binary", "") or "").strip()
    if configured:
        return configured
    npm = shutil.which("npm")
    if npm:
        return npm
    node = _node_binary()
    if os.name == "nt" and node.lower().endswith("node.exe"):
        candidate = str(Path(node).with_name("npm.cmd"))
        if Path(candidate).exists():
            return candidate
    return "npm"


def _auto_install_enabled() -> bool:
    return bool(get_config("cloakbrowser.auto_install_bridge_deps", True))


def _playwright_marker() -> Path:
    return BASE_DIR / "node_modules" / "playwright"


def _node_modules_ok() -> bool:
    return _playwright_marker().is_dir()


def _run_command(cmd: list[str], *, cwd: Path, timeout: float, env: dict | None = None) -> None:
    logger.info(f"CloakBrowser deps: running {' '.join(cmd)}")
    completed = subprocess.run(
        cmd,
        cwd=str(cwd),
        env=env,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    )
    if completed.returncode != 0:
        stderr = (completed.stderr or "").strip()
        stdout = (completed.stdout or "").strip()
        detail = stderr or stdout or f"exit code {completed.returncode}"
        raise RuntimeError(detail[:2000])


def _check_node_available() -> None:
    node = _node_binary()
    if not shutil.which(node) and not Path(node).is_file():
        raise RuntimeError(
            f"未找到 Node 可执行文件: {node}。请安装 Node.js 18+，或在 cloakbrowser.node_binary 中指定路径。"
        )


def _verify_playwright_launch() -> bool:
    node = _node_binary()
    script = (
        "const { chromium } = require('playwright');"
        "chromium.launch({ headless: true })"
        ".then((b) => b.close())"
        ".then(() => process.exit(0))"
        ".catch((e) => { console.error(e && e.message ? e.message : e); process.exit(2); });"
    )
    completed = subprocess.run(
        [node, "-e", script],
        cwd=str(BASE_DIR),
        capture_output=True,
        text=True,
        timeout=120,
        check=False,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    )
    if completed.returncode == 0:
        return True
    output = f"{completed.stderr}\n{completed.stdout}".lower()
    if "executable doesn't exist" in output or "browserType.launch" in output or "exit 2" in output:
        return False
    if completed.returncode == 2:
        return False
    detail = (completed.stderr or completed.stdout or "").strip()
    raise RuntimeError(f"Playwright 自检失败: {detail[:1000]}")




def _playwright_cli() -> list[str]:
    if os.name == "nt":
        local = BASE_DIR / "node_modules" / ".bin" / "playwright.cmd"
    else:
        local = BASE_DIR / "node_modules" / ".bin" / "playwright"
    if local.is_file():
        return [str(local)]
    return [_npm_binary(), "exec", "--yes", "playwright"]

def _install_npm_deps() -> None:
    if not (BASE_DIR / "package.json").is_file():
        raise RuntimeError(f"缺少 package.json: {BASE_DIR}")
    npm = _npm_binary()
    lock = BASE_DIR / "package-lock.json"
    if lock.is_file():
        cmd = [npm, "ci", "--omit=dev", "--no-audit", "--no-fund"]
    else:
        cmd = [npm, "install", "--omit=dev", "--no-audit", "--no-fund"]
    timeout = float(get_config("cloakbrowser.deps_install_timeout", 600) or 600)
    _run_command(cmd, cwd=BASE_DIR, timeout=max(timeout, 60.0))


def _install_playwright_chromium() -> None:
    npm = _npm_binary()
    timeout = float(get_config("cloakbrowser.deps_install_timeout", 600) or 600)
    env = os.environ.copy()
    env.setdefault("PLAYWRIGHT_BROWSERS_PATH", "0")
    _run_command(
        _playwright_cli() + ["install", "chromium"],
        cwd=BASE_DIR,
        timeout=max(timeout, 120.0),
        env=env,
    )
    if sys.platform.startswith("linux") and bool(get_config("cloakbrowser.auto_install_system_deps", False)):
        try:
            _run_command(
                _playwright_cli() + ["install-deps", "chromium"],
                cwd=BASE_DIR,
                timeout=max(timeout, 300.0),
                env=env,
            )
        except Exception as exc:
            logger.warning(f"CloakBrowser 系统依赖自动安装失败（通常需要 root）: {exc}")


def ensure_bridge_dependencies_sync() -> None:
    global _INSTALL_DONE
    if _INSTALL_DONE:
        return
    if not _auto_install_enabled():
        _check_node_available()
        if not _node_modules_ok():
            raise RuntimeError(
                "CloakBrowser bridge 依赖未安装。请在 app/services/browser_bridge 目录执行: "
                "npm install --omit=dev && npx playwright install chromium"
            )
        if not _verify_playwright_launch():
            raise RuntimeError(
                "Playwright Chromium 未安装。请执行: npx playwright install chromium"
            )
        _INSTALL_DONE = True
        return

    _check_node_available()
    if not _node_modules_ok():
        logger.info("CloakBrowser deps: node_modules 缺失，开始自动安装 npm 依赖")
        _install_npm_deps()
    if not _verify_playwright_launch():
        logger.info("CloakBrowser deps: Chromium 不可用，开始自动安装 Playwright 浏览器")
        _install_playwright_chromium()
        if not _verify_playwright_launch():
            raise RuntimeError("Playwright Chromium 安装后仍无法启动，请查看 logs/cloakbrowser_bridge.log")
    _INSTALL_DONE = True
    logger.info("CloakBrowser bridge 依赖检查完成")


async def ensure_bridge_dependencies() -> None:
    if not bool(get_config("cloakbrowser.enabled", False)):
        return
    async with _INSTALL_LOCK:
        await asyncio.to_thread(ensure_bridge_dependencies_sync)
