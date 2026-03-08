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
            raise RuntimeError('ws_connect should not be used in image edit upload retry tests')

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

from app.core.exceptions import AppException
from app.services.grok.services.image_edit import ImageEditService


class ImageEditUploadTokenRetryTests(unittest.IsolatedAsyncioTestCase):
    async def test_edit_retries_next_token_when_upload_network_error_occurs(self):
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(
            consume=AsyncMock(return_value=True),
            mark_rate_limited=AsyncMock(return_value=True),
        )
        service = ImageEditService()
        upload_error = AppException(
            message='图片上传失败：网络连接异常，请稍后重试',
            error_type='server_error',
            code='upload_network_error',
            status_code=502,
        )

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 2,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(side_effect=['bad-token', 'good-token'])), \
             patch.object(ImageEditService, '_upload_images', AsyncMock(side_effect=[upload_error, ['https://assets.grok.com/users/demo/uploaded/source.jpg']])), \
             patch.object(ImageEditService, '_get_parent_post_id', AsyncMock(return_value='uploaded-parent-post')), \
             patch.object(ImageEditService, '_collect_images', AsyncMock(return_value=['https://assets.grok.com/users/demo/generated/out/image.jpg'])):
            result = await service.edit(
                token_mgr=token_mgr,
                token='bad-token',
                model_info=model_info,
                prompt='背景融合',
                images=['http://localhost:18000/v1/files/image/users/demo/generated/source-id/image.jpg'],
                n=1,
                response_format='url',
                stream=False,
            )

        self.assertEqual(result.data, ['https://assets.grok.com/users/demo/generated/out/image.jpg'])
        token_mgr.consume.assert_awaited_once()
        consume_args = token_mgr.consume.await_args.args
        self.assertEqual(consume_args[0], 'good-token')


if __name__ == '__main__':
    unittest.main()
