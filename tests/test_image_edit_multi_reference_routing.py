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

from app.services.grok.services.image_edit import ImageEditService


class ImageEditMultiReferenceRoutingTests(unittest.IsolatedAsyncioTestCase):
    async def test_multi_reference_edit_skips_parent_post_creation(self):
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(consume=AsyncMock(return_value=True))
        service = ImageEditService()
        uploaded_urls = [
            'https://assets.grok.com/users/demo/generated/first/content',
            'https://assets.grok.com/users/demo/generated/second/content',
        ]

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(return_value='good-token')), \
             patch.object(ImageEditService, '_upload_images', AsyncMock(return_value=uploaded_urls)), \
             patch.object(ImageEditService, '_get_parent_post_id', AsyncMock(return_value='should-not-be-used')) as get_parent_post_id, \
             patch.object(ImageEditService, '_collect_images', AsyncMock(return_value=['https://assets.grok.com/users/demo/generated/out/image.jpg'])) as collect_images:
            result = await service.edit(
                token_mgr=token_mgr,
                token='good-token',
                model_info=model_info,
                prompt='merge two people',
                images=['img-a', 'img-b'],
                n=1,
                response_format='url',
                stream=False,
                return_all_images=True,
            )

        self.assertEqual(result.data, ['https://assets.grok.com/users/demo/generated/out/image.jpg'])
        get_parent_post_id.assert_not_awaited()
        config = collect_images.await_args.kwargs['model_config_override']['modelMap']['imageEditModelConfig']
        self.assertEqual(config['imageReferences'], uploaded_urls)
        self.assertNotIn('parentPostId', config)

    async def test_reference_items_multi_skips_parent_post_creation(self):
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(consume=AsyncMock(return_value=True))
        service = ImageEditService()
        prepared_refs = [
            {
                "source_url": "data:image/png;base64,AAA",
                "request_url": "https://assets.grok.com/users/demo/generated/first/content",
                "resolved_url": "https://assets.grok.com/users/demo/generated/first/content",
                "original_id": "",
                "resolved_id": "first",
                "mention_id": "first",
                "attachment_id": "first",
                "parent_post_id": "",
                "mention_alias": "Image 1",
            },
            {
                "source_url": "data:image/png;base64,BBB",
                "request_url": "https://assets.grok.com/users/demo/generated/second/content",
                "resolved_url": "https://assets.grok.com/users/demo/generated/second/content",
                "original_id": "",
                "resolved_id": "second",
                "mention_id": "second",
                "attachment_id": "second",
                "parent_post_id": "",
                "mention_alias": "Image 2",
            },
        ]

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(return_value='good-token')), \
             patch.object(ImageEditService, '_prepare_reference_items', AsyncMock(return_value=prepared_refs)), \
             patch('app.services.grok.services.image_edit.VideoService.create_image_post', AsyncMock(return_value='should-not-be-used')) as create_image_post, \
             patch.object(ImageEditService, '_collect_images', AsyncMock(return_value=['https://assets.grok.com/users/demo/generated/out/image.jpg'])) as collect_images:
            result = await service.edit_with_reference_items(
                token_mgr=token_mgr,
                token='good-token',
                model_info=model_info,
                prompt='合照',
                reference_items=[{"image_url": "a"}, {"image_url": "b"}],
                response_format='url',
                stream=False,
                return_all_images=True,
            )

        self.assertEqual(result.data, ['https://assets.grok.com/users/demo/generated/out/image.jpg'])
        create_image_post.assert_not_awaited()
        config = collect_images.await_args.kwargs['model_config_override']['modelMap']['imageEditModelConfig']
        self.assertEqual(
            config['imageReferences'],
            [
                "https://assets.grok.com/users/demo/generated/first/content",
                "https://assets.grok.com/users/demo/generated/second/content",
            ],
        )
        self.assertNotIn('parentPostId', config)


if __name__ == '__main__':
    unittest.main()
