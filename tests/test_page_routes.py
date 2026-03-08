import os
import sys
import tempfile
import types
import unittest
from unittest.mock import patch
from pathlib import Path
from types import SimpleNamespace

if 'aiohttp' not in sys.modules:
    aiohttp = types.ModuleType('aiohttp')
    aiohttp.WSMsgType = SimpleNamespace(TEXT='TEXT', BINARY='BINARY', CLOSE='CLOSE', CLOSED='CLOSED', ERROR='ERROR')
    aiohttp.BaseConnector = object
    aiohttp.ClientWebSocketResponse = object
    aiohttp.ClientError = Exception

    class _TCPConnector:
        def __init__(self, *args, **kwargs):
            pass

    class _ClientTimeout:
        def __init__(self, *args, **kwargs):
            pass

    class _ClientSession:
        def __init__(self, *args, **kwargs):
            pass

        async def close(self):
            return None

        async def ws_connect(self, *args, **kwargs):
            raise RuntimeError('ws_connect should not be used in page route tests')

    aiohttp.TCPConnector = _TCPConnector
    aiohttp.ClientTimeout = _ClientTimeout
    aiohttp.ClientSession = _ClientSession
    sys.modules['aiohttp'] = aiohttp

if 'aiohttp_socks' not in sys.modules:
    aiohttp_socks = types.ModuleType('aiohttp_socks')

    class _ProxyConnector:
        @staticmethod
        def from_url(*args, **kwargs):
            return object()

    aiohttp_socks.ProxyConnector = _ProxyConnector
    sys.modules['aiohttp_socks'] = aiohttp_socks

from fastapi import HTTPException
from fastapi.responses import FileResponse, HTMLResponse

from app.api.pages import public as public_pages
from app.api.pages import admin as admin_pages
from app.api.pages import helpers as page_helpers
from app.api.pages.helpers import get_asset_version, render_html_page
from main import create_app


class PageRouteHelpersTests(unittest.TestCase):
    def test_public_page_response_raises_404_for_missing_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            original = public_pages.STATIC_DIR
            public_pages.STATIC_DIR = Path(tmp)
            try:
                with self.assertRaises(HTTPException) as ctx:
                    public_pages._public_page_response('public/pages/missing.html')
                self.assertEqual(ctx.exception.status_code, 404)
            finally:
                public_pages.STATIC_DIR = original

    def test_admin_page_response_raises_404_for_missing_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            original = admin_pages.STATIC_DIR
            admin_pages.STATIC_DIR = Path(tmp)
            try:
                with self.assertRaises(HTTPException) as ctx:
                    admin_pages._admin_page_response('admin/pages/missing.html')
                self.assertEqual(ctx.exception.status_code, 404)
            finally:
                admin_pages.STATIC_DIR = original

    def test_public_page_response_returns_html_response_when_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'public/pages/demo.html'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('ok?v=__ASSET_VERSION__')
            original = public_pages.STATIC_DIR
            public_pages.STATIC_DIR = root
            try:
                response = public_pages._public_page_response('public/pages/demo.html')
                self.assertIsInstance(response, HTMLResponse)
                body = response.body.decode()
                self.assertIn(f'ok?v={get_asset_version()}', body)
                self.assertNotIn('__ASSET_VERSION__', body)
            finally:
                public_pages.STATIC_DIR = original

    def test_render_html_page_raises_404_for_missing_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(HTTPException) as ctx:
                render_html_page(Path(tmp), 'public/pages/missing.html')
            self.assertEqual(ctx.exception.status_code, 404)

    def test_render_html_page_replaces_asset_version_placeholder(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'public/pages/demo.html'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('<script src="/static/demo.js?v=__ASSET_VERSION__"></script>')
            response = render_html_page(root, 'public/pages/demo.html')
            self.assertIsInstance(response, HTMLResponse)
            body = response.body.decode()
            self.assertIn(get_asset_version(), body)
            self.assertNotIn('__ASSET_VERSION__', body)

    def test_admin_page_response_returns_html_response_when_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'admin/pages/demo.html'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('admin?v=__ASSET_VERSION__')
            original = admin_pages.STATIC_DIR
            admin_pages.STATIC_DIR = root
            try:
                response = admin_pages._admin_page_response('admin/pages/demo.html')
                self.assertIsInstance(response, HTMLResponse)
                body = response.body.decode()
                self.assertIn(f'admin?v={get_asset_version()}', body)
                self.assertNotIn('__ASSET_VERSION__', body)
            finally:
                admin_pages.STATIC_DIR = original

    def test_render_html_page_prefers_env_asset_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'public/pages/demo.html'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('env?v=__ASSET_VERSION__')
            old = os.environ.get('APP_ASSET_VERSION')
            os.environ['APP_ASSET_VERSION'] = 'env-test-version'
            try:
                response = render_html_page(root, 'public/pages/demo.html')
                body = response.body.decode()
                self.assertIn('env?v=env-test-version', body)
            finally:
                if old is None:
                    os.environ.pop('APP_ASSET_VERSION', None)
                else:
                    os.environ['APP_ASSET_VERSION'] = old

    def test_create_app_registers_health_and_favicon_routes(self):
        app = create_app()
        paths = {route.path for route in app.router.routes}
        self.assertIn('/health', paths)
        self.assertIn('/favicon.ico', paths)

    def test_get_asset_version_defaults_to_project_version(self):
        with patch.dict(os.environ, {}, clear=True):
            with patch.object(page_helpers, '_get_git_short_sha', return_value=''):
                self.assertEqual(page_helpers.get_asset_version(), '0.3.0')

    def test_get_asset_version_uses_git_sha_when_available(self):
        with patch.dict(os.environ, {}, clear=True):
            with patch.object(page_helpers, '_get_git_short_sha', return_value='abc1234'):
                self.assertEqual(page_helpers.get_asset_version(), '0.3.0+abc1234')

class PublicEntryAssetRouteTests(unittest.IsolatedAsyncioTestCase):
    async def test_public_manifest_returns_file_response_when_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'public/manifest.webmanifest'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('{"name":"demo"}')
            original = public_pages.STATIC_DIR
            public_pages.STATIC_DIR = root
            try:
                with patch('app.api.pages.public.is_public_enabled', return_value=True):
                    response = await public_pages.public_manifest()
                self.assertIsInstance(response, FileResponse)
            finally:
                public_pages.STATIC_DIR = original

    async def test_public_service_worker_returns_404_when_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            original = public_pages.STATIC_DIR
            public_pages.STATIC_DIR = Path(tmp)
            try:
                with patch('app.api.pages.public.is_public_enabled', return_value=True):
                    with self.assertRaises(HTTPException) as ctx:
                        await public_pages.public_service_worker()
                self.assertEqual(ctx.exception.status_code, 404)
            finally:
                public_pages.STATIC_DIR = original


if __name__ == '__main__':
    unittest.main()
