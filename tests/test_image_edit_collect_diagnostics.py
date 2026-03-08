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
            raise RuntimeError('ws_connect should not be used in image_edit tests')

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

from app.core.exceptions import UpstreamException
from app.services.grok.services.image_edit import ImageEditService


class FakeProcessor:
    def __init__(self, model, token, response_format='url', progress_cb=None):
        self.progress_cb = progress_cb

    async def process(self, response):
        if self.progress_cb:
            await self.progress_cb('chat_connected', {'progress': 60, 'message': '模型连接成功，正在生成图片'})
        return []


class ImageEditCollectDiagnosticsTests(unittest.IsolatedAsyncioTestCase):
    async def test_collect_images_reports_connected_empty_state(self):
        service = ImageEditService()
        model_info = SimpleNamespace(model_id='grok-image-edit-test', grok_model='imagine-image-edit')

        with patch('app.services.grok.services.image_edit.GrokChatService.chat', AsyncMock(return_value=object())), \
             patch('app.services.grok.services.image_edit.ImageCollectProcessor', FakeProcessor):
            with self.assertRaises(UpstreamException) as ctx:
                await service._collect_images(
                    token='good-token',
                    prompt='merge two people',
                    model_info=model_info,
                    response_format='url',
                    tool_overrides={'imageGen': True},
                    model_config_override={'modelMap': {}},
                )

        self.assertIn('connected', ctx.exception.message.lower())
        self.assertEqual(ctx.exception.details.get('error'), 'empty_result')
        self.assertEqual(ctx.exception.details.get('chat_connected'), True)
        self.assertEqual(ctx.exception.details.get('image_count'), 0)


if __name__ == '__main__':
    unittest.main()
