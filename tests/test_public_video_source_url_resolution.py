import sys
import types
import unittest
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
            raise RuntimeError('ws_connect should not be used in public video source url tests')

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

from app.api.v1.public_api import video


class PublicVideoSourceUrlResolutionTests(unittest.TestCase):
    def test_prefers_explicit_source_image_url_for_parent_post(self):
        result = video._resolve_parent_source_image_url(
            parent_post_id='37b7e1ef-2bba-4a9c-b47c-8d2580734617',
            source_image_url='http://localhost:18000/v1/files/image/users/demo/generated/37b7e1ef-2bba-4a9c-b47c-8d2580734617/image.jpg',
        )

        self.assertEqual(
            result,
            'http://localhost:18000/v1/files/image/users/demo/generated/37b7e1ef-2bba-4a9c-b47c-8d2580734617/image.jpg',
        )

    def test_falls_back_to_imagine_public_when_source_image_url_missing(self):
        result = video._resolve_parent_source_image_url(
            parent_post_id='37b7e1ef-2bba-4a9c-b47c-8d2580734617',
            source_image_url='',
        )

        self.assertEqual(
            result,
            'https://imagine-public.x.ai/imagine-public/images/37b7e1ef-2bba-4a9c-b47c-8d2580734617.jpg',
        )


if __name__ == '__main__':
    unittest.main()
