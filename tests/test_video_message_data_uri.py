import sys
import types
import unittest
from types import SimpleNamespace

# Stub optional websocket deps so isolated unit tests can import service modules
if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.WSMsgType = SimpleNamespace(
        TEXT="TEXT", BINARY="BINARY", CLOSE="CLOSE", CLOSED="CLOSED", ERROR="ERROR"
    )
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
            raise RuntimeError("ws_connect should not be used in video message tests")

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

from app.services.grok.services.video import VideoService


class VideoMessageDataUriTests(unittest.TestCase):
    def test_build_video_message_strips_data_uri_without_prompt(self):
        message = VideoService._build_video_message(
            prompt="",
            preset="normal",
            source_image_url="data:image/png;base64,AAA",
        )
        self.assertNotIn("data:image", message)
        self.assertEqual(message, "--mode=normal")

    def test_build_video_message_strips_data_uri_with_prompt(self):
        message = VideoService._build_video_message(
            prompt="a cat",
            preset="normal",
            source_image_url="data:image/png;base64,AAA",
        )
        self.assertNotIn("data:image", message)
        self.assertEqual(message, "a cat --mode=custom")


if __name__ == "__main__":
    unittest.main()
