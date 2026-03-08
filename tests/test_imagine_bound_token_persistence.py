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
            raise RuntimeError('ws_connect should not be used in imagine token tests')

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


class ImagineBoundTokenPersistenceTests(unittest.IsolatedAsyncioTestCase):
    async def test_get_bound_image_token_falls_back_to_persisted_asset_map(self):
        parent_post_id = '2ac1d8a3-4999-4157-8306-538418629d0b'
        fake_map = SimpleNamespace(get_token=AsyncMock(return_value='persisted-token'))

        with patch.object(imagine, '_IMAGINE_IMAGE_TOKENS', {}), \
             patch('app.services.grok.utils.asset_token_map.AssetTokenMap.get_instance', AsyncMock(return_value=fake_map)):
            token = await imagine._get_bound_image_token(parent_post_id)

        self.assertEqual(token, 'persisted-token')
        fake_map.get_token.assert_awaited_once_with(parent_post_id)


if __name__ == '__main__':
    unittest.main()
