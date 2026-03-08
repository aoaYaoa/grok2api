import sys
import types
import unittest
import uuid
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

import orjson

# Stub optional websocket deps so isolated unit tests can import API modules
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
            raise RuntimeError('ws_connect should not be used in video_30s tests')

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

from app.api.v1.chat import ChatCompletionRequest, MessageItem, VideoConfig, validate_request
from app.api.v1.public_api.video import VideoStartRequest, public_video_start
import app.services.grok.services.video as video_module


class Video30SupportTests(unittest.IsolatedAsyncioTestCase):
    async def test_public_video_start_accepts_30_seconds(self):
        with patch('app.api.v1.public_api.video._new_session', AsyncMock(return_value='task-30')) as new_session:
            result = await public_video_start(VideoStartRequest(prompt='demo video', video_length=30))

        self.assertEqual(result['task_id'], 'task-30')
        self.assertEqual(result['task_ids'], ['task-30'])
        self.assertEqual(new_session.await_args.args[2], 30)

    def test_chat_video_config_accepts_30_seconds(self):
        request = ChatCompletionRequest(
            model='grok-imagine-1.0-video',
            messages=[MessageItem(role='user', content='make a longer video')],
            video_config=VideoConfig(video_length=30),
        )
        fake_model = SimpleNamespace(is_video=True, is_image=False, is_image_edit=False)

        with patch('app.api.v1.chat.ModelService.valid', return_value=True), \
             patch('app.api.v1.chat.ModelService.get', return_value=fake_model):
            validate_request(request)

        self.assertEqual(request.video_config.video_length, 30)

    def test_round_plan_supports_30_seconds_basic_pool(self):
        build_round_plan = getattr(video_module, '_build_round_plan', None)
        self.assertTrue(callable(build_round_plan), 'expected _build_round_plan helper to exist')

        plan = build_round_plan(30, is_super=False)
        self.assertEqual(len(plan), 5)
        self.assertEqual([item.video_length for item in plan], [6, 6, 6, 6, 6])
        self.assertEqual([item.is_extension for item in plan], [False, True, True, True, True])
        self.assertEqual([item.extension_start_time for item in plan], [None, 6.0, 12.0, 18.0, 24.0])

    async def test_completions_chains_five_rounds_for_30_seconds_on_basic_pool(self):
        class FakeTokenManager:
            def __init__(self):
                self.reload_if_stale = AsyncMock(return_value=None)
                self.consume = AsyncMock(return_value=True)

            def get_token_for_video(self, **kwargs):
                return SimpleNamespace(token='basic-token')

            def get_pool_name_for_token(self, token):
                return 'ssoBasic'

        request_calls = []

        async def fake_request(session, token, **kwargs):
            request_calls.append(kwargs)
            post_id = str(uuid.UUID(int=len(request_calls)))
            payload = {
                'result': {
                    'response': {
                        'responseId': f'resp-{len(request_calls)}',
                        'streamingVideoGenerationResponse': {
                            'progress': 100,
                            'videoPostId': post_id,
                            'videoUrl': f'https://assets.grok.com/users/demo/generated/{post_id}/generated_video.mp4',
                            'thumbnailImageUrl': '/thumbs/final.jpg',
                        },
                    }
                }
            }

            async def _stream():
                yield orjson.dumps(payload)

            return _stream()

        async def fake_render_video(self, video_url, token, thumbnail_url=''):
            return f'rendered:{video_url}'

        fake_model = SimpleNamespace(cost=SimpleNamespace(value='low'))

        with patch('app.services.grok.services.video.get_token_manager', AsyncMock(return_value=FakeTokenManager())), \
             patch('app.services.grok.services.video.get_config', side_effect=lambda key, default=None: {
                 'app.stream': False,
                 'app.thinking': False,
                 'retry.max_retry': 1,
                 'video.auto_upscale': False,
                 'video.concurrent': 1,
                 'video.stream_timeout': 60,
                 'proxy.browser': '',
             }.get(key, default)), \
             patch('app.services.grok.services.video.ModelService.pool_candidates_for_model', return_value=[]), \
             patch('app.services.grok.services.video.ModelService.get', return_value=fake_model), \
             patch('app.services.grok.services.chat.MessageExtractor.extract', return_value=('demo video', [], [])), \
             patch('app.services.grok.services.video.AppChatReverse.request', side_effect=fake_request), \
             patch('app.services.grok.services.video.MediaPostReverse.request', AsyncMock(return_value=SimpleNamespace(json=lambda: {'post': {'id': str(uuid.UUID(int=999))}}))), \
             patch('app.services.grok.utils.download.DownloadService.render_video', fake_render_video):
            result = await video_module.VideoService.completions(
                'grok-imagine-1.0-video',
                messages=[{'role': 'user', 'content': 'demo video'}],
                stream=False,
                reasoning_effort='none',
                aspect_ratio='16:9',
                video_length=30,
                resolution='480p',
                preset='normal',
            )

        self.assertEqual(len(request_calls), 5)
        self.assertEqual(result['choices'][0]['message']['content'], f"rendered:https://assets.grok.com/users/demo/generated/{uuid.UUID(int=5)}/generated_video.mp4")


if __name__ == '__main__':
    unittest.main()
