import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import patch

if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.WSMsgType = SimpleNamespace(TEXT="TEXT", BINARY="BINARY", CLOSE="CLOSE", CLOSED="CLOSED", ERROR="ERROR")
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

    aiohttp.TCPConnector = _TCPConnector
    aiohttp.ClientTimeout = _ClientTimeout
    aiohttp.ClientSession = _ClientSession
    sys.modules["aiohttp"] = aiohttp

if "aiohttp_socks" not in sys.modules:
    aiohttp_socks = types.ModuleType("aiohttp_socks")

    class _ProxyConnector:
        @staticmethod
        def from_url(*args, **kwargs):
            return object()

    aiohttp_socks.ProxyConnector = _ProxyConnector
    sys.modules["aiohttp_socks"] = aiohttp_socks

from app.services.reverse.app_chat import _resolve_request_timeout


class AppChatTimeoutTests(unittest.TestCase):
    def test_regular_request_uses_existing_read_timeout(self):
        def fake_get_config(key, default=None):
            values = {
                "chat.timeout": 60,
                "video.timeout": 90,
                "image.timeout": 45,
                "chat.connect_timeout": None,
            }
            return values.get(key, default)

        with patch("app.services.reverse.app_chat.get_config", side_effect=fake_get_config):
            connect_timeout, read_timeout = _resolve_request_timeout({})

        self.assertEqual(read_timeout, 90.0)
        self.assertEqual(connect_timeout, 12.0)

    def test_video_generation_request_extends_upstream_read_timeout_past_total_timeout(self):
        def fake_get_config(key, default=None):
            values = {
                "chat.timeout": 60,
                "video.timeout": 60,
                "image.timeout": 45,
                "video.total_timeout": 300,
                "chat.connect_timeout": None,
            }
            return values.get(key, default)

        with patch("app.services.reverse.app_chat.get_config", side_effect=fake_get_config):
            connect_timeout, read_timeout = _resolve_request_timeout({"videoGen": True})

        self.assertEqual(read_timeout, 330.0)
        self.assertEqual(connect_timeout, 12.0)


if __name__ == "__main__":
    unittest.main()
