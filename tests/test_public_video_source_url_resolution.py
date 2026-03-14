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
            raise RuntimeError('ws_connect should not be used in public video source url tests')

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

from app.api.v1.public_api import video
from app.api.v1.public_api.video import VideoStartRequest


class PublicVideoSourceUrlResolutionTests(unittest.TestCase):
    def test_prefers_explicit_source_image_url_for_parent_post(self):
        result = video._resolve_parent_source_image_url(
            parent_post_id='37b7e1ef-2bba-4a9c-b47c-8d2580734617',
            source_image_url='http://localhost:18000/v1/files/image/users/demo/generated/37b7e1ef-2bba-4a9c-b47c-8d2580734617/image.jpg',
        )

        self.assertEqual(
            result,
            'http://localhost:18000/v1/files/image/users/demo/generated/37b7e1ef-2bba-4a9c-b47c-8d2580734617/image.jpg',
        )

    def test_falls_back_to_imagine_public_when_source_image_url_missing(self):
        result = video._resolve_parent_source_image_url(
            parent_post_id='37b7e1ef-2bba-4a9c-b47c-8d2580734617',
            source_image_url='',
        )

        self.assertEqual(
            result,
            'https://imagine-public.x.ai/imagine-public/images/37b7e1ef-2bba-4a9c-b47c-8d2580734617.jpg',
        )


class PublicVideoReferenceNormalizationTests(unittest.IsolatedAsyncioTestCase):
    async def test_reference_item_parent_post_with_image_url_is_accepted(self):
        payload = VideoStartRequest(
            prompt="",
            image_url=None,
            parent_post_id=None,
            source_image_url=None,
            reference_items=[
                {
                    "parent_post_id": "137130a0-ef3f-43c7-b177-d6ca2b4d4a5d",
                    "image_url": "https://imagine-public.x.ai/imagine-public/images/137130a0-ef3f-43c7-b177-d6ca2b4d4a5d.jpg",
                    "source_image_url": "https://imagine-public.x.ai/imagine-public/images/137130a0-ef3f-43c7-b177-d6ca2b4d4a5d.jpg",
                    "mention_alias": "Image 1",
                }
            ],
            reasoning_effort="low",
            aspect_ratio="3:2",
            video_length=6,
            resolution_name="480p",
            preset="normal",
            concurrent=1,
        )

        with patch("app.api.v1.public_api.video._new_session", AsyncMock(return_value="task-1")) as new_session:
            result = await video.public_video_start(payload)

        self.assertEqual(result.get("task_id"), "task-1")
        new_session.assert_awaited_once()


if __name__ == '__main__':
    unittest.main()
