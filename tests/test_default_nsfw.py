import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

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
            raise RuntimeError('ws_connect should not be used in default_nsfw tests')

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

from app.api.v1.image import ImageGenerationRequest, create_image
from app.api.v1.public_api.video import VideoStartRequest, public_video_start
from app.api.v1.nsfw import NSFWRequest
from app.services.token.manager import TokenManager
from app.services.token.models import TokenInfo
from app.services.token.pool import TokenPool


class DefaultNsfwTests(unittest.IsolatedAsyncioTestCase):
    async def test_images_generation_defaults_nsfw_true_and_allows_false(self):
        fake_model = SimpleNamespace(model_id='grok-imagine-1.0', is_image=True)
        fake_result = SimpleNamespace(stream=False, data=['img'], usage_override=None)

        with patch('app.api.v1.image._get_token', AsyncMock(return_value=(SimpleNamespace(), 'token'))), \
             patch('app.api.v1.image.ModelService.get', return_value=fake_model), \
             patch('app.api.v1.image.resolve_response_format', return_value='url'), \
             patch('app.api.v1.image.response_field_name', return_value='url'), \
             patch('app.api.v1.image.validate_generation_request', return_value=None), \
             patch('app.api.v1.image.ImageGenerationService.generate', AsyncMock(return_value=fake_result)) as generate:
            await create_image(ImageGenerationRequest(prompt='demo'))
            self.assertTrue(generate.await_args.kwargs['enable_nsfw'])

            await create_image(ImageGenerationRequest(prompt='demo', nsfw=False))
            self.assertFalse(generate.await_args.kwargs['enable_nsfw'])

    async def test_public_video_start_defaults_nsfw_true_and_allows_false(self):
        with patch('app.api.v1.public_api.video._new_session', AsyncMock(return_value='task-1')) as new_session:
            await public_video_start(VideoStartRequest(prompt='demo video'))
            self.assertTrue(new_session.await_args.args[8])

            await public_video_start(VideoStartRequest(prompt='demo video', nsfw=False))
            self.assertFalse(new_session.await_args.args[8])

    def test_nsfw_request_accepts_30_seconds(self):
        request = NSFWRequest(image_prompt='demo', video_length=30)
        self.assertEqual(request.video_length, 30)

    def test_video_token_routing_prefers_nsfw_tag_when_requested(self):
        manager = TokenManager.__new__(TokenManager)
        basic = TokenPool('ssoBasic')
        basic.add(TokenInfo(token='plain', quota=80))
        basic.add(TokenInfo(token='tagged', quota=80, tags=['nsfw']))
        manager.pools = {'ssoBasic': basic}

        picked = manager.get_token_for_video(resolution='480p', video_length=6, pool_candidates=['ssoBasic'], preferred_tags=['nsfw'])
        self.assertIsNotNone(picked)
        self.assertEqual(picked.token, 'tagged')


if __name__ == '__main__':
    unittest.main()
