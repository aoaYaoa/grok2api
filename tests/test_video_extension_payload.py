import sys
import types
import unittest
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

from app.services.grok.services.video import VideoService, _build_extension_config


class VideoExtensionPayloadTests(unittest.IsolatedAsyncioTestCase):
    def test_build_extension_config_uses_post_id_for_stitch_field_when_enabled(self):
        payload = _build_extension_config(
            parent_post_id='b3403055a4dc42079cb3b85455338e65',
            extend_post_id='b3403055a4dc42079cb3b85455338e65',
            original_post_id='b3403055a4dc42079cb3b85455338e65',
            original_prompt='继续向前推进镜头',
            aspect_ratio='16:9',
            resolution_name='720p',
            video_length=10,
            start_time=12.5,
            stitch_with_extend=True,
        )

        cfg = payload['modelMap']['videoGenModelConfig']
        self.assertEqual(cfg['stitchWithExtendPostId'], 'b3403055a4dc42079cb3b85455338e65')

    def test_build_extension_config_omits_stitch_field_when_disabled(self):
        payload = _build_extension_config(
            parent_post_id='b3403055a4dc42079cb3b85455338e65',
            extend_post_id='b3403055a4dc42079cb3b85455338e65',
            original_post_id='b3403055a4dc42079cb3b85455338e65',
            original_prompt='继续向前推进镜头',
            aspect_ratio='16:9',
            resolution_name='720p',
            video_length=10,
            start_time=12.5,
            stitch_with_extend=False,
        )

        cfg = payload['modelMap']['videoGenModelConfig']
        self.assertNotIn('stitchWithExtendPostId', cfg)

    async def test_generate_extend_video_uses_post_id_for_stitch_field_when_enabled(self):
        captured = {}

        async def fake_request(session, token, **kwargs):
            captured.update(kwargs)

            async def _stream():
                yield b''

            return _stream()

        service = VideoService()

        with patch('app.services.grok.services.video.get_config', side_effect=lambda key, default=None: {
            'video.concurrent': 1,
            'video.moderated_max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.video.AppChatReverse.request', side_effect=fake_request):
            stream = await service.generate_extend_video(
                token='good-token',
                prompt='继续向前推进镜头',
                extend_post_id='b3403055a4dc42079cb3b85455338e65',
                original_post_id='b3403055a4dc42079cb3b85455338e65',
                file_attachment_id='b3403055a4dc42079cb3b85455338e65',
                video_extension_start_time=12.5,
                aspect_ratio='16:9',
                video_length=10,
                resolution='720p',
                preset='normal',
                stitch_with_extend=True,
            )
            async for _ in stream:
                pass

        cfg = captured['model_config_override']['modelMap']['videoGenModelConfig']
        self.assertEqual(cfg['stitchWithExtendPostId'], 'b3403055a4dc42079cb3b85455338e65')

    async def test_generate_extend_video_omits_stitch_field_when_disabled(self):
        captured = {}

        async def fake_request(session, token, **kwargs):
            captured.update(kwargs)

            async def _stream():
                yield b''

            return _stream()

        service = VideoService()

        with patch('app.services.grok.services.video.get_config', side_effect=lambda key, default=None: {
            'video.concurrent': 1,
            'video.moderated_max_retry': 1,
        }.get(key, default)), \
             patch('app.services.grok.services.video.AppChatReverse.request', side_effect=fake_request):
            stream = await service.generate_extend_video(
                token='good-token',
                prompt='继续向前推进镜头',
                extend_post_id='b3403055a4dc42079cb3b85455338e65',
                original_post_id='b3403055a4dc42079cb3b85455338e65',
                file_attachment_id='b3403055a4dc42079cb3b85455338e65',
                video_extension_start_time=12.5,
                aspect_ratio='16:9',
                video_length=10,
                resolution='720p',
                preset='normal',
                stitch_with_extend=False,
            )
            async for _ in stream:
                pass

        cfg = captured['model_config_override']['modelMap']['videoGenModelConfig']
        self.assertNotIn('stitchWithExtendPostId', cfg)


if __name__ == '__main__':
    unittest.main()
