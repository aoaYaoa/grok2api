import json
import sys
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch


if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.WSMsgType = SimpleNamespace(
        TEXT="TEXT", BINARY="BINARY", CLOSE="CLOSE", CLOSED="CLOSED", ERROR="ERROR"
    )
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
            raise RuntimeError("ws_connect should not be used in videos compat tests")

    aiohttp.TCPConnector = _TCPConnector
    aiohttp.ClientTimeout = _ClientTimeout
    aiohttp.ClientSession = _ClientSession
    sys.modules["aiohttp"] = aiohttp

if "aiohttp_socks" not in sys.modules:
    aiohttp_socks = types.ModuleType("aiohttp_socks")

    class _ProxyConnector:
        @staticmethod
        def from_url(*args, **kwargs):
            return object()

    aiohttp_socks.ProxyConnector = _ProxyConnector
    sys.modules["aiohttp_socks"] = aiohttp_socks


from app.api.v1 import video_api


class VideosCompatApiTests(unittest.IsolatedAsyncioTestCase):
    async def test_create_video_maps_html_client_payload_and_normalizes_html_content(self):
        fake_response = {
            "created": 123,
            "choices": [
                {
                    "message": {
                        "content": '<video controls><source src="https://cdn.example.com/out.mp4" type="video/mp4"></video>'
                    }
                }
            ],
        }

        with patch.object(video_api.VideoService, "completions", AsyncMock(return_value=fake_response)) as completions:
            response = await video_api.create_video(
                video_api.VideoGenerationRequest(
                    prompt="demo video",
                    size="1792x1024",
                    seconds=18,
                    quality="high",
                    image_reference={"image_url": "https://example.com/ref.png"},
                )
            )

        payload = json.loads(response.body)
        self.assertEqual(payload["created"], 123)
        self.assertEqual(payload["data"], [{"url": "https://cdn.example.com/out.mp4"}])

        self.assertEqual(completions.await_args.kwargs["model"], "grok-imagine-1.0-video")
        self.assertFalse(completions.await_args.kwargs["stream"])
        self.assertEqual(completions.await_args.kwargs["aspect_ratio"], "3:2")
        self.assertEqual(completions.await_args.kwargs["video_length"], 18)
        self.assertEqual(completions.await_args.kwargs["resolution"], "720p")
        self.assertEqual(completions.await_args.kwargs["single_image_mode"], "reference")
        self.assertEqual(
            completions.await_args.kwargs["messages"],
            [
                {
                    "role": "user",
                    "content": [
                        {"type": "text", "text": "demo video"},
                        {
                            "type": "image_url",
                            "image_url": {"url": "https://example.com/ref.png"},
                        },
                    ],
                }
            ],
        )

    async def test_create_video_normalizes_markdown_video_content(self):
        fake_response = {
            "choices": [
                {
                    "message": {
                        "content": "[video](https://cdn.example.com/markdown.mp4)"
                    }
                }
            ],
        }

        with patch.object(video_api.VideoService, "completions", AsyncMock(return_value=fake_response)):
            response = await video_api.create_video(
                video_api.VideoGenerationRequest(
                    prompt="markdown video",
                    size="1280x720",
                    seconds=6,
                    quality="standard",
                )
            )

        payload = json.loads(response.body)
        self.assertEqual(payload["data"], [{"url": "https://cdn.example.com/markdown.mp4"}])

    async def test_create_video_normalizes_plain_url_content(self):
        fake_response = {
            "choices": [
                {
                    "message": {
                        "content": "https://cdn.example.com/plain.mp4"
                    }
                }
            ],
        }

        with patch.object(video_api.VideoService, "completions", AsyncMock(return_value=fake_response)):
            response = await video_api.create_video(
                video_api.VideoGenerationRequest(prompt="plain url video")
            )

        payload = json.loads(response.body)
        self.assertEqual(payload["data"], [{"url": "https://cdn.example.com/plain.mp4"}])


if __name__ == "__main__":
    unittest.main()
