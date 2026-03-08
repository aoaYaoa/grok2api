from __future__ import annotations

import os
import subprocess
import tomllib
from pathlib import Path

from fastapi import HTTPException
from fastapi.responses import HTMLResponse

_PROJECT_ROOT = Path(__file__).resolve().parents[3]
_PYPROJECT_PATH = _PROJECT_ROOT / 'pyproject.toml'
_PLACEHOLDER = '__ASSET_VERSION__'
_FALLBACK_ASSET_VERSION = 'dev'


def _get_project_version() -> str:
    try:
        data = tomllib.loads(_PYPROJECT_PATH.read_text(encoding='utf-8'))
    except Exception:
        return ''
    project = data.get('project') or {}
    version = project.get('version')
    return str(version or '').strip()


def _get_git_short_sha() -> str:
    try:
        result = subprocess.run(
            ['git', '-C', str(_PROJECT_ROOT), 'rev-parse', '--short', 'HEAD'],
            check=True,
            capture_output=True,
            text=True,
        )
    except Exception:
        return ''
    return (result.stdout or '').strip()


def get_asset_version() -> str:
    raw = (os.getenv('APP_ASSET_VERSION') or '').strip()
    if raw:
        return raw

    version = _get_project_version() or _FALLBACK_ASSET_VERSION
    sha = _get_git_short_sha()
    if sha:
        return f'{version}+{sha}'
    return version


def render_html_page(static_dir: Path, relative_path: str) -> HTMLResponse:
    file_path = static_dir / relative_path
    if not file_path.exists():
        raise HTTPException(status_code=404, detail='Page not found')

    content = file_path.read_text(encoding='utf-8')
    if _PLACEHOLDER in content:
        content = content.replace(_PLACEHOLDER, get_asset_version())
    return HTMLResponse(content=content)
