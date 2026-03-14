import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch
import orjson

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


class FakeSummaryProcessor:
    def __init__(self, model, token, response_format='url', progress_cb=None):
        self.progress_cb = progress_cb
        self.last_payload_keys = ['result']
        self.last_result_keys = ['response']
        self.last_response_keys = ['modelResponse']
        self.last_model_response_keys = ['imageEditUris']
        self.last_result_title = 'blocked'
        self.last_payload_summary = {}
        self.last_result_summary = {}
        self.last_response_summary = {}
        self.last_model_response_summary = {
            'imageEditUris': {
                'count': 1,
                'sample': 'https://assets.grok.com/users/demo/generated/abc/content'
            }
        }
        self.last_model_response_stream_errors = {
            'count': 1,
            'sample': 'policy_violation'
        }
        self.last_model_response_tool_responses = {
            'count': 1,
            'sample_keys': ['type', 'message']
        }

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

    async def test_collect_images_includes_model_response_summary(self):
        service = ImageEditService()
        model_info = SimpleNamespace(model_id='grok-image-edit-test', grok_model='imagine-image-edit')

        with patch('app.services.grok.services.image_edit.GrokChatService.chat', AsyncMock(return_value=object())), \
             patch('app.services.grok.services.image_edit.ImageCollectProcessor', FakeSummaryProcessor):
            with self.assertRaises(UpstreamException) as ctx:
                await service._collect_images(
                    token='good-token',
                    prompt='merge two people',
                    model_info=model_info,
                    response_format='url',
                    tool_overrides={'imageGen': True},
                    model_config_override={'modelMap': {}},
                )

        summary = ctx.exception.details.get('model_response_summary') or {}
        self.assertEqual(summary.get('imageEditUris', {}).get('count'), 1)
        self.assertEqual(ctx.exception.details.get('result_title'), 'blocked')
        self.assertEqual(
            (ctx.exception.details.get('model_response_stream_errors') or {}).get('count'),
            1,
        )
        self.assertEqual(
            (ctx.exception.details.get('model_response_tool_responses') or {}).get('count'),
            1,
        )


class ImageEditCollectResponseRootTests(unittest.IsolatedAsyncioTestCase):
    async def test_collect_images_from_response_root_urls(self):
        from app.services.grok.services.image_edit import ImageCollectProcessor
        processor = ImageCollectProcessor('grok-image-edit-test', response_format='url')

        async def fake_process_url(path, media_type='image'):
            return path

        processor.process_url = fake_process_url  # type: ignore[assignment]

        async def stream():
            payload = {
                'result': {
                    'response': {
                        'imageUrls': ['https://example.com/resp-root.jpg']
                    }
                }
            }
            yield orjson.dumps(payload)

        images = await processor.process(stream())
        self.assertEqual(images, ['https://example.com/resp-root.jpg'])

    async def test_collect_images_from_result_root_urls(self):
        from app.services.grok.services.image_edit import ImageCollectProcessor
        processor = ImageCollectProcessor('grok-image-edit-test', response_format='url')

        async def fake_process_url(path, media_type='image'):
            return path

        processor.process_url = fake_process_url  # type: ignore[assignment]

        async def stream():
            payload = {
                'result': {
                    'imageUrls': ['https://example.com/result-root.jpg']
                }
            }
            yield orjson.dumps(payload)

        images = await processor.process(stream())
        self.assertEqual(images, ['https://example.com/result-root.jpg'])

    async def test_collect_tracks_payload_keys_when_no_images(self):
        from app.services.grok.services.image_edit import ImageCollectProcessor
        processor = ImageCollectProcessor('grok-image-edit-test', response_format='url')

        async def stream():
            payload = {
                'result': {
                    'response': {
                        'modelResponse': {
                            'status': 'ok'
                        }
                    }
                },
                'metadata': {
                    'foo': 'bar'
                }
            }
            yield orjson.dumps(payload)

        images = await processor.process(stream())
        self.assertEqual(images, [])
        self.assertEqual(processor.last_payload_keys, ['metadata', 'result'])
        self.assertEqual(processor.last_result_keys, ['response'])
        self.assertEqual(processor.last_response_keys, ['modelResponse'])
        self.assertEqual(processor.last_model_response_keys, ['status'])

    async def test_collect_tracks_image_field_summary_when_no_images(self):
        from app.services.grok.services.image_edit import ImageCollectProcessor
        processor = ImageCollectProcessor('grok-image-edit-test', response_format='url')

        async def stream():
            payload = {
                'result': {
                    'response': {
                        'modelResponse': {
                            'imageEditUris': [
                                'https://assets.grok.com/users/demo/generated/abc/content'
                            ],
                            'fileUris': [
                                'https://assets.grok.com/users/demo/generated/def/content'
                            ],
                            'generatedImageUrls': []
                        }
                    }
                }
            }
            yield orjson.dumps(payload)

        images = await processor.process(stream())
        self.assertEqual(images, [])
        summary = processor.last_model_response_summary
        self.assertEqual(summary['imageEditUris']['count'], 1)
        self.assertEqual(summary['fileUris']['count'], 1)
        self.assertEqual(summary['generatedImageUrls']['count'], 0)

    async def test_collect_preserves_first_nonempty_model_response(self):
        from app.services.grok.services.image_edit import ImageCollectProcessor
        processor = ImageCollectProcessor('grok-image-edit-test', response_format='url')

        async def stream():
            first = {
                'result': {
                    'response': {
                        'modelResponse': {
                            'imageEditUris': [
                                'https://assets.grok.com/users/demo/generated/abc/content'
                            ],
                            'streamErrors': ['policy_violation'],
                        }
                    }
                }
            }
            yield orjson.dumps(first)
            yield orjson.dumps({'result': {'title': 'blocked'}})

        images = await processor.process(stream())
        self.assertEqual(images, [])
        self.assertEqual(processor.last_model_response_keys, ['imageEditUris', 'streamErrors'])
        self.assertEqual(processor.last_model_response_summary['imageEditUris']['count'], 1)
        self.assertEqual(processor.last_model_response_stream_errors['count'], 1)
        self.assertEqual(processor.last_result_title, 'blocked')


if __name__ == '__main__':
    unittest.main()
