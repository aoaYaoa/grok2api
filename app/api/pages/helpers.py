from __future__ import annotations

import hashlib
import os
import subprocess
import tomllib
from functools import lru_cache
from pathlib import Path

from fastapi import HTTPException
from fastapi.responses import HTMLResponse

_PROJECT_ROOT = Path(__file__).resolve().parents[3]
_PYPROJECT_PATH = _PROJECT_ROOT / 'pyproject.toml'
_STATIC_DIR = _PROJECT_ROOT / 'app' / 'static'
_PLACEHOLDER = '__ASSET_VERSION__'
_FALLBACK_ASSET_VERSION = 'dev'
_FINGERPRINT_SUFFIXES = {'.css', '.html', '.js'}


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


@lru_cache(maxsize=1)
def _get_static_fingerprint() -> str:
    if not _STATIC_DIR.exists():
        return ''

    digest = hashlib.sha256()
    try:
        paths = sorted(
            path for path in _STATIC_DIR.rglob('*')
            if path.is_file() and path.suffix.lower() in _FINGERPRINT_SUFFIXES
        )
        for path in paths:
            digest.update(path.relative_to(_STATIC_DIR).as_posix().encode('utf-8'))
            with path.open('rb') as file_obj:
                for chunk in iter(lambda: file_obj.read(64 * 1024), b''):
                    digest.update(chunk)
    except OSError:
        return ''
    return digest.hexdigest()[:10]


def get_asset_version() -> str:
    raw = (os.getenv('APP_ASSET_VERSION') or '').strip()
    if raw:
        return raw

    version = _get_project_version() or _FALLBACK_ASSET_VERSION
    sha = _get_git_short_sha()
    if sha:
        return f'{version}+{sha}'
    fingerprint = _get_static_fingerprint()
    if fingerprint:
        return f'{version}+{fingerprint}'
    return version


def render_html_page(static_dir: Path, relative_path: str) -> HTMLResponse:
    file_path = static_dir / relative_path
    if not file_path.exists():
        raise HTTPException(status_code=404, detail='Page not found')

    content = file_path.read_text(encoding='utf-8')
    if _PLACEHOLDER in content:
        content = content.replace(_PLACEHOLDER, get_asset_version())
    return HTMLResponse(
        content=content,
        headers={'Cache-Control': 'no-store, max-age=0'},
    )
