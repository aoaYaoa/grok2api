import sys
import types
import unittest
from types import SimpleNamespace

if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.WSMsgType = SimpleNamespace(
        TEXT="TEXT",
        BINARY="BINARY",
        CLOSE="CLOSE",
        CLOSED="CLOSED",
        ERROR="ERROR",
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
            raise RuntimeError("ws_connect should not be used in media_post tests")

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


class MediaPostImportTests(unittest.TestCase):
    def test_media_post_reverse_import_and_metadata_dir(self):
        from app.services.reverse.media_post import MediaPostReverse

        metadata_dir = MediaPostReverse._metadata_dir()
        self.assertTrue(metadata_dir.name == "media-meta")
        self.assertTrue(metadata_dir.exists())


if __name__ == "__main__":
    unittest.main()
