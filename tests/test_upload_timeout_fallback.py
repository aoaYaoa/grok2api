import base64
import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

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
            raise RuntimeError('ws_connect should not be used in upload tests')

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

from app.services.grok.utils.upload import UploadService


class _DummyLock:
    async def __aenter__(self):
        return None

    async def __aexit__(self, exc_type, exc, tb):
        return False


class _FakeResponse:
    def __init__(self):
        self.status_code = 200
        self.headers = {"content-type": "image/jpeg"}
        self.content = b"test-image-bytes"


class _FakeSession:
    def __init__(self):
        self.calls = []

    async def get(self, url, timeout=None, proxies=None, stream=False):
        self.calls.append(
            {
                "url": url,
                "timeout": timeout,
                "proxies": proxies,
                "stream": stream,
            }
        )
        return _FakeResponse()


class UploadTimeoutFallbackTests(unittest.IsolatedAsyncioTestCase):
    async def test_parse_b64_uses_safe_default_timeout_when_config_is_none(self):
        service = UploadService()
        session = _FakeSession()
        expected_b64 = base64.b64encode(b"test-image-bytes").decode()

        def fake_get_config(key, default=None):
            if key == "asset.upload_timeout":
                return None
            if key == "proxy.base_proxy_url":
                return None
            if key == "app.app_url":
                return ""
            return default

        with patch("app.services.grok.utils.upload.get_config", side_effect=fake_get_config), \
             patch("app.services.grok.utils.upload._file_lock", return_value=_DummyLock()), \
             patch.object(UploadService, "create", AsyncMock(return_value=session)):
            name, b64, mime = await service.parse_b64("https://example.com/demo.jpg")

        self.assertEqual(name, "demo.jpg")
        self.assertEqual(mime, "image/jpeg")
        self.assertEqual(b64, expected_b64)
        self.assertEqual(len(session.calls), 1)
        self.assertEqual(session.calls[0]["timeout"], 60.0)
        self.assertEqual(session.calls[0]["stream"], True)

    async def test_parse_b64_reads_local_v1_files_url_without_app_url_match(self):
        service = UploadService()

        def fake_get_config(key, default=None):
            if key == "asset.upload_timeout":
                return None
            if key == "proxy.base_proxy_url":
                return None
            if key == "app.app_url":
                return ""
            return default

        with patch("app.services.grok.utils.upload.get_config", side_effect=fake_get_config), \
             patch.object(UploadService, "_read_local_file", AsyncMock(return_value=("image.jpg", "b64local", "image/jpeg"))) as read_local, \
             patch.object(UploadService, "create", AsyncMock(side_effect=AssertionError("network session should not be created"))):
            name, b64, mime = await service.parse_b64("http://192.168.1.107:8000/v1/files/image/users/demo/generated/test-id/image.jpg")

        self.assertEqual((name, b64, mime), ("image.jpg", "b64local", "image/jpeg"))
        read_local.assert_awaited_once_with("image", "users-demo-generated-test-id-image.jpg")

    async def test_read_local_file_uses_safe_default_timeout_when_config_is_none(self):
        import tempfile
        from pathlib import Path

        service = UploadService()

        def fake_get_config(key, default=None):
            if key == "asset.upload_timeout":
                return None
            return default

        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            image_dir = root / "tmp" / "image"
            image_dir.mkdir(parents=True, exist_ok=True)
            target = image_dir / "demo.jpg"
            target.write_bytes(b"local-image-bytes")

            with patch("app.services.grok.utils.upload.get_config", side_effect=fake_get_config),                  patch("app.services.grok.utils.upload.DATA_DIR", root),                  patch("app.services.grok.utils.upload._file_lock", return_value=_DummyLock()):
                name, b64, mime = await service._read_local_file("image", "demo.jpg")

        self.assertEqual(name, "demo.jpg")
        self.assertEqual(mime, "image/jpeg")
        self.assertEqual(base64.b64decode(b64), b"local-image-bytes")


if __name__ == "__main__":
    unittest.main()
