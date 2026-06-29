import sys
import time
import types
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.WSMsgType = SimpleNamespace(
        TEXT="TEXT",
        BINARY="BINARY",
        CLOSE="CLOSE",
        CLOSED="CLOSED",
        ERROR="ERROR",
    )
    aiohttp.BaseConnector = object
    aiohttp.ClientWebSocketResponse = object
    aiohttp.ClientError = Exception
    aiohttp.TCPConnector = lambda *args, **kwargs: object()
    aiohttp.ClientTimeout = lambda *args, **kwargs: object()
    aiohttp.ClientSession = object
    sys.modules["aiohttp"] = aiohttp

if "aiohttp_socks" not in sys.modules:
    aiohttp_socks = types.ModuleType("aiohttp_socks")

    class _ProxyConnector:
        @staticmethod
        def from_url(*args, **kwargs):
            return object()

    aiohttp_socks.ProxyConnector = _ProxyConnector
    sys.modules["aiohttp_socks"] = aiohttp_socks

import asyncio

from app.api.v1.public_api import video


class FakeRequest:
    async def is_disconnected(self):
        return False


def _decode_chunks(chunks):
    result = []
    for chunk in chunks:
        if isinstance(chunk, bytes):
            result.append(chunk.decode("utf-8", errors="replace"))
        else:
            result.append(str(chunk))
    return "".join(result)


class PublicVideoParentPostSseTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        async with video._VIDEO_SESSIONS_LOCK:
            video._VIDEO_SESSIONS.clear()
        if hasattr(video, "_PARENT_POST_SSE_LOCKS"):
            video._PARENT_POST_SSE_LOCKS.clear()

    async def _register_session(self, task_id: str, parent_post_id: str):
        async with video._VIDEO_SESSIONS_LOCK:
            video._VIDEO_SESSIONS[task_id] = {
                "created_at": time.time(),
                "prompt": "",
                "aspect_ratio": "16:9",
                "video_length": 6,
                "resolution_name": "480p",
                "preset": "spicy",
                "image_url": None,
                "parent_post_id": parent_post_id,
                "source_image_url": f"https://imagine-public.x.ai/imagine-public/images/{parent_post_id}.jpg",
                "reference_items": [],
                "nsfw": True,
                "reasoning_effort": None,
                "single_image_mode": "frame",
                "is_video_extension": False,
            }

    async def test_parent_post_sse_uses_collected_completion_mode(self):
        task_id = "task-parent-post-mode"
        parent_post_id = "12345678-1234-1234-1234-123456789abc"
        await self._register_session(task_id, parent_post_id)

        fake_result = {
            "id": "resp_parent_video",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "<video>ok</video>"},
                    "finish_reason": "stop",
                }
            ],
        }

        with patch(
            "app.api.v1.public_api.video.imagine_public_api._get_bound_image_token",
            AsyncMock(return_value=None),
        ), patch(
            "app.api.v1.public_api.video.VideoService.completions",
            AsyncMock(return_value=fake_result),
        ) as completions:
            response = await video.public_video_sse(FakeRequest(), task_id=task_id)
            chunks = [chunk async for chunk in response.body_iterator]

        self.assertFalse(completions.await_args.kwargs["stream"])
        joined = _decode_chunks(chunks)
        self.assertIn('"id":"resp_parent_video"', joined)
        self.assertIn("data: [DONE]", joined)

    async def test_parent_post_sse_serializes_same_parent_requests(self):
        parent_post_id = "87654321-4321-4321-4321-cba987654321"
        await self._register_session("task-parent-1", parent_post_id)
        await self._register_session("task-parent-2", parent_post_id)

        running = 0
        max_running = 0

        async def fake_completions(*args, **kwargs):
            nonlocal running, max_running
            running += 1
            max_running = max(max_running, running)
            await asyncio.sleep(0.05)
            running -= 1
            return {
                "id": kwargs["parent_post_id"],
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": "<video>ok</video>"},
                        "finish_reason": "stop",
                    }
                ],
            }

        with patch(
            "app.api.v1.public_api.video.imagine_public_api._get_bound_image_token",
            AsyncMock(return_value=None),
        ), patch(
            "app.api.v1.public_api.video.VideoService.completions",
            AsyncMock(side_effect=fake_completions),
        ):
            response1 = await video.public_video_sse(FakeRequest(), task_id="task-parent-1")
            response2 = await video.public_video_sse(FakeRequest(), task_id="task-parent-2")
            await asyncio.gather(
                asyncio.create_task(response1.body_iterator.__anext__()),
                asyncio.create_task(response2.body_iterator.__anext__()),
            )

        self.assertEqual(max_running, 1)


if __name__ == "__main__":
    unittest.main()
