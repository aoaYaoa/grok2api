import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

if 'aiohttp' not in sys.modules:
    aiohttp = types.ModuleType('aiohttp')
    aiohttp.BaseConnector = object
    aiohttp.TCPConnector = lambda *args, **kwargs: object()
    aiohttp.ClientSession = object
    aiohttp.ClientWebSocketResponse = object
    aiohttp.ClientTimeout = lambda *args, **kwargs: object()
    sys.modules['aiohttp'] = aiohttp

if 'aiohttp_socks' not in sys.modules:
    aiohttp_socks = types.ModuleType('aiohttp_socks')
    class _ProxyConnector:
        @staticmethod
        def from_url(*args, **kwargs):
            return object()
    aiohttp_socks.ProxyConnector = _ProxyConnector
    sys.modules['aiohttp_socks'] = aiohttp_socks

from app.core.exceptions import AppException, UpstreamException
from app.services.grok.services.video import VideoService, VideoStreamProcessor, _classify_video_error
import app.services.grok.utils.asset_token_map as asset_token_map


class EmptyAsyncIterator:
    def __aiter__(self):
        return self

    async def __anext__(self):
        raise StopAsyncIteration


class FakeTokenManager:
    def __init__(self):
        self.record_fail = AsyncMock(return_value=True)
        self.consume = AsyncMock(return_value=True)
        self.reload_if_stale = AsyncMock(return_value=None)
        self.mark_rate_limited = AsyncMock(return_value=None)
        self._tokens = [
            SimpleNamespace(token='bad-token-1'),
            SimpleNamespace(token='bad-token-2'),
        ]

    def get_pool_name_for_token(self, token):
        return 'pool-a'

    def get_token_for_video(self, **kwargs):
        exclude = set(kwargs.get('exclude') or set())
        for info in self._tokens:
            if info.token not in exclude:
                return info
        return None


class VideoExtensionRuntimeErrorsTests(unittest.IsolatedAsyncioTestCase):
    async def test_stream_processor_raises_on_empty_video_stream(self):
        processor = VideoStreamProcessor(
            'grok-imagine-1.0-video',
            token='good-token',
            show_think=False,
            upscale_on_finish=False,
        )

        async def consume_all():
            async for _ in processor.process(EmptyAsyncIterator()):
                pass

        with self.assertRaises(AppException) as ctx:
            with patch(
                'app.services.grok.services.video.get_config',
                side_effect=lambda key, default=None: {
                    'video.stream_timeout': 60,
                    'video.total_timeout': 300,
                }.get(key, default),
            ):
                await consume_all()

        exc = ctx.exception
        self.assertEqual(exc.code, 'video_empty_stream')
        self.assertEqual(exc.status_code, 502)

    async def test_extension_auth_failures_exhausted_return_token_unbound_error(self):
        manager = FakeTokenManager()
        auth_error = UpstreamException(
            'Failed to look up session ID. [WKE=unauthenticated:invalid-credentials]',
            status_code=401,
            details={'body': 'unauthenticated:invalid-credentials'},
        )

        with patch('app.services.grok.services.video.get_token_manager', AsyncMock(return_value=manager)), \
             patch('app.services.grok.services.video.get_config', side_effect=lambda key, default=None: {
                 'retry.max_retry': 2,
                 'video.extension_token_retry': 2,
                 'video.auto_upscale': False,
             }.get(key, default)), \
             patch.object(asset_token_map.AssetTokenMap, 'get_instance', AsyncMock(return_value=SimpleNamespace(get_token=AsyncMock(return_value=None)))), \
             patch('app.services.grok.services.model.ModelService.pool_candidates_for_model', return_value=[]), \
             patch('app.services.grok.services.chat.MessageExtractor.extract', return_value=('video prompt', [], [])), \
             patch.object(VideoService, 'generate_extend_video', AsyncMock(side_effect=[auth_error, auth_error])):
            with self.assertRaises(AppException) as ctx:
                await VideoService.completions(
                    'grok-imagine-1.0-video',
                    messages=[{'role': 'user', 'content': 'video prompt'}],
                    stream=False,
                    reasoning_effort='none',
                    aspect_ratio='16:9',
                    video_length=10,
                    resolution='720p',
                    preset='normal',
                    extend_post_id='12345678-1234-1234-1234-123456789abc',
                    video_extension_start_time=2.5,
                    stitch_with_extend=True,
                )

        exc = ctx.exception
        self.assertEqual(exc.code, 'video_extension_token_unbound')
        self.assertEqual(exc.status_code, 502)
        self.assertIn('当前 token 池中没有可访问该视频的账号', exc.message)
        self.assertEqual(manager.record_fail.await_count, 2)

    def test_classify_empty_video_stream_as_specific_error(self):
        exc = UpstreamException(
            'Video stream finished without any playable result',
            status_code=502,
            details={'type': 'empty_video_stream'},
        )

        message, code, status = _classify_video_error(exc)

        self.assertEqual(code, 'video_empty_stream')
        self.assertEqual(status, 502)
        self.assertIn('上游未返回视频结果', message)


if __name__ == '__main__':
    unittest.main()
