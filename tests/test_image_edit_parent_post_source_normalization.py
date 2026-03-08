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


class ImageEditParentPostSourceNormalizationTests(unittest.IsolatedAsyncioTestCase):
    async def test_parent_post_edit_normalizes_private_source_to_imagine_public(self):
        parent_post_id = '97abdfd1-fe20-4af4-9d84-123456789abc'
        private_source = (
            'http://192.168.18.88:8000/v1/files/image/users/demo/'
            'generated/97abdfd1-fe20-4af4-9d84-123456789abc/image.jpg'
        )
        imagine_public = (
            f'https://imagine-public.x.ai/imagine-public/images/{parent_post_id}.jpg'
        )
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(consume=AsyncMock(return_value=True))
        service = ImageEditService()

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(return_value='good-token')), \
             patch('app.services.grok.services.image_edit.VideoService.create_image_post', AsyncMock(return_value='precreated-post-id')) as create_image_post, \
             patch.object(ImageEditService, '_collect_images', AsyncMock(return_value=['https://assets.grok.com/users/demo/generated/out/image.jpg'])) as collect_images:
            result = await service.edit_with_parent_post(
                token_mgr=token_mgr,
                token='good-token',
                model_info=model_info,
                prompt='extend scene',
                parent_post_id=parent_post_id,
                source_image_url=private_source,
                response_format='url',
                stream=False,
            )

        self.assertEqual(result.data, ['https://assets.grok.com/users/demo/generated/out/image.jpg'])
        create_image_post.assert_awaited_once_with('good-token', imagine_public)
        config = collect_images.await_args.kwargs['model_config_override']['modelMap']['imageEditModelConfig']
        self.assertEqual(config['imageReferences'], [imagine_public])
        self.assertEqual(config['parentPostId'], 'precreated-post-id')

    async def test_parent_post_edit_falls_back_to_upload_when_parent_chain_returns_no_images(self):
        parent_post_id = '28e4cd77-7480-474b-8b17-e71556facd5f'
        source_image_url = 'https://assets.grok.com/users/demo/generated/28e4cd77-7480-474b-8b17-e71556facd5f/image.jpg'
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(consume=AsyncMock(return_value=True))
        service = ImageEditService()
        fallback_result = SimpleNamespace(
            stream=False,
            data=['https://assets.grok.com/users/demo/generated/fallback/image.jpg'],
        )

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(return_value='good-token')), \
             patch('app.services.grok.services.image_edit.VideoService.create_image_post', AsyncMock(return_value='precreated-post-id')), \
             patch.object(ImageEditService, '_collect_images', AsyncMock(return_value=[])), \
             patch.object(ImageEditService, 'edit', AsyncMock(return_value=fallback_result)) as fallback_edit:
            result = await service.edit_with_parent_post(
                token_mgr=token_mgr,
                token='good-token',
                model_info=model_info,
                prompt='outpaint group photo',
                parent_post_id=parent_post_id,
                source_image_url=source_image_url,
                response_format='url',
                stream=False,
                return_all_images=True,
            )

        self.assertEqual(result.data, fallback_result.data)
        fallback_edit.assert_awaited_once()
        kwargs = fallback_edit.await_args.kwargs
        self.assertEqual(kwargs['images'], [source_image_url])
        self.assertEqual(kwargs['token'], 'good-token')
        self.assertEqual(kwargs['prompt'], 'outpaint group photo')
        self.assertEqual(kwargs['response_format'], 'url')
        self.assertEqual(kwargs['return_all_images'], True)

    async def test_parent_post_edit_falls_back_to_upload_when_parent_chain_raises_empty_result(self):
        parent_post_id = '63f8be21-72ae-4059-8f88-50975fbae71a'
        source_image_url = 'https://assets.grok.com/users/demo/generated/63f8be21-72ae-4059-8f88-50975fbae71a/image.jpg'
        model_info = SimpleNamespace(
            model_id='grok-image-edit-test',
            grok_model='grok-2-image',
            cost=SimpleNamespace(value='low'),
        )
        token_mgr = SimpleNamespace(consume=AsyncMock(return_value=True))
        service = ImageEditService()
        fallback_result = SimpleNamespace(
            stream=False,
            data=['https://assets.grok.com/users/demo/generated/fallback-empty-result/image.jpg'],
        )
        empty_result_error = UpstreamException(
            'Image edit upstream ended before any image URL was returned',
            details={
                'error': 'empty_result',
                'chat_connected': False,
                'image_count': 0,
            },
        )

        with patch('app.services.grok.services.image_edit.get_config', side_effect=lambda key, default=None: {
            'retry.max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.image_edit.pick_token', AsyncMock(return_value='good-token')), \
             patch('app.services.grok.services.image_edit.VideoService.create_image_post', AsyncMock(return_value='precreated-post-id')), \
             patch.object(ImageEditService, '_collect_images', AsyncMock(side_effect=empty_result_error)), \
             patch.object(ImageEditService, 'edit', AsyncMock(return_value=fallback_result)) as fallback_edit:
            result = await service.edit_with_parent_post(
                token_mgr=token_mgr,
                token='good-token',
                model_info=model_info,
                prompt='continue portrait scene',
                parent_post_id=parent_post_id,
                source_image_url=source_image_url,
                response_format='url',
                stream=False,
                return_all_images=True,
            )

        self.assertEqual(result.data, fallback_result.data)
        fallback_edit.assert_awaited_once()
        kwargs = fallback_edit.await_args.kwargs
        self.assertEqual(kwargs['images'], [source_image_url])
        self.assertEqual(kwargs['token'], 'good-token')
        self.assertEqual(kwargs['prompt'], 'continue portrait scene')
        self.assertEqual(kwargs['response_format'], 'url')
        self.assertEqual(kwargs['return_all_images'], True)


if __name__ == '__main__':
    unittest.main()
