import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import patch

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
            raise RuntimeError('ws_connect should not be used in imagine source url tests')

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

from app.api.v1.public_api import imagine


class ImagineSourceUrlResolutionTests(unittest.TestCase):
    def test_resolve_source_image_url_prefers_local_file_url_for_assets_path(self):
        with patch('app.api.v1.public_api.imagine.get_config', side_effect=lambda key, default=None: {
            'app.app_url': 'http://192.168.1.107:8000',
        }.get(key, default)):
            result = imagine._resolve_source_image_url(
                image_url='https://assets.grok.com/users/demo/generated/9e51c8d6-e799-41fd-86bb-e697d88c6937/image.jpg',
                parent_post_id='9e51c8d6-e799-41fd-86bb-e697d88c6937',
            )

        self.assertEqual(
            result,
            'http://192.168.1.107:8000/v1/files/image/users/demo/generated/9e51c8d6-e799-41fd-86bb-e697d88c6937/image.jpg',
        )


class ImagineParentSourceCanonicalizationTests(unittest.IsolatedAsyncioTestCase):
    async def test_canonicalize_parent_source_image_url_falls_back_to_imagine_public(self):
        parent_post_id = '9e51c8d6-e799-41fd-86bb-e697d88c6937'
        self.assertTrue(
            hasattr(imagine, '_canonicalize_parent_source_image_url'),
            'missing _canonicalize_parent_source_image_url helper',
        )
        result = await imagine._canonicalize_parent_source_image_url(
            'test-token',
            parent_post_id,
            '',
        )
        self.assertEqual(
            result,
            f'https://imagine-public.x.ai/imagine-public/images/{parent_post_id}.jpg',
        )


if __name__ == '__main__':
    unittest.main()
