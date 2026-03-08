import sys
import types

if "aiohttp" not in sys.modules:
    aiohttp = types.ModuleType("aiohttp")
    aiohttp.BaseConnector = object
    aiohttp.TCPConnector = lambda *args, **kwargs: object()
    aiohttp.ClientSession = object
    aiohttp.ClientWebSocketResponse = object
    aiohttp.ClientTimeout = lambda *args, **kwargs: object()
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
import unittest
from unittest.mock import patch

import orjson

from app.core.exceptions import UpstreamException, AppException
from app.services.grok.services.video import VideoStreamProcessor, _collect_round_result, _classify_video_error


async def progress_stream_forever():
    progress = 0
    while True:
        progress += 1
        payload = {
            "result": {
                "response": {
                    "responseId": "resp_timeout_case",
                    "streamingVideoGenerationResponse": {
                        "progress": progress,
                    },
                }
            }
        }
        yield orjson.dumps(payload)
        await asyncio.sleep(0.01)


class CancellationDelayedIterator:
    def __aiter__(self):
        return self

    async def __anext__(self):
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            await asyncio.sleep(0.2)
            raise
        raise StopAsyncIteration


class CloseHangingIterator:
    def __aiter__(self):
        return self

    async def __anext__(self):
        await asyncio.Future()
        raise StopAsyncIteration

    async def aclose(self):
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            await asyncio.sleep(0.2)
            raise


class VideoTotalTimeoutTests(unittest.IsolatedAsyncioTestCase):
    async def test_collect_round_result_fails_after_total_timeout_even_with_progress_updates(self):
        with self.assertRaises(UpstreamException) as ctx:
            await asyncio.wait_for(
                _collect_round_result(
                    progress_stream_forever(),
                    model="grok-imagine-1.0-video",
                    source="collect-round-1",
                    total_timeout=0.05,
                ),
                timeout=0.3,
            )

        exc = ctx.exception
        self.assertEqual(exc.details["type"], "video_total_timeout")
        self.assertEqual(exc.details["source"], "collect-round-1")
        self.assertEqual(exc.details["response_id"], "resp_timeout_case")
        self.assertGreater(exc.details["last_progress"], 0)


    async def test_stream_processor_times_out_when_upstream_cancellation_is_slow(self):
        processor = VideoStreamProcessor(
            "grok-imagine-1.0-video",
            token="good-token",
            show_think=False,
            upscale_on_finish=False,
        )

        async def consume_all():
            async for _ in processor.process(CancellationDelayedIterator()):
                pass

        with self.assertRaises(AppException) as ctx:
            with patch(
                "app.services.grok.services.video.get_config",
                side_effect=lambda key, default=None: {
                    "video.stream_timeout": 60,
                    "video.total_timeout": 0.05,
                }.get(key, default),
            ):
                await asyncio.wait_for(consume_all(), timeout=0.8)

        exc = ctx.exception
        self.assertEqual(exc.code, "video_total_timeout")
        self.assertEqual(exc.status_code, 504)


    async def test_stream_processor_times_out_even_when_aclose_hangs_after_cancellation(self):
        processor = VideoStreamProcessor(
            "grok-imagine-1.0-video",
            token="good-token",
            show_think=False,
            upscale_on_finish=False,
        )

        async def consume_all():
            async for _ in processor.process(CloseHangingIterator()):
                pass

        with self.assertRaises(AppException) as ctx:
            with patch(
                "app.services.grok.services.video.get_config",
                side_effect=lambda key, default=None: {
                    "video.stream_timeout": 60,
                    "video.total_timeout": 0.05,
                }.get(key, default),
            ):
                await asyncio.wait_for(consume_all(), timeout=0.8)

        exc = ctx.exception
        self.assertEqual(exc.code, "video_total_timeout")
        self.assertEqual(exc.status_code, 504)


    async def test_stream_processor_fails_after_total_timeout_even_when_progress_continues(self):
        processor = VideoStreamProcessor(
            "grok-imagine-1.0-video",
            token="good-token",
            show_think=False,
            upscale_on_finish=False,
        )

        async def consume_all():
            async for _ in processor.process(progress_stream_forever()):
                pass

        with self.assertRaises(AppException) as ctx:
            with patch(
                "app.services.grok.services.video.get_config",
                side_effect=lambda key, default=None: {
                    "video.stream_timeout": 60,
                    "video.total_timeout": 0.05,
                }.get(key, default),
            ):
                await asyncio.wait_for(consume_all(), timeout=0.3)

        exc = ctx.exception
        self.assertEqual(exc.code, "video_total_timeout")
        self.assertEqual(exc.status_code, 504)
        self.assertEqual(exc.message, "视频生成超时（5分钟），请稍后重试")

    def test_classify_total_timeout_as_explicit_timeout_message(self):
        exc = UpstreamException(
            "Video stream total timeout after 300s",
            status_code=504,
            details={"type": "video_total_timeout"},
        )

        message, code, status = _classify_video_error(exc)

        self.assertEqual(message, "视频生成超时（5分钟），请稍后重试")
        self.assertEqual(code, "video_total_timeout")
        self.assertEqual(status, 504)


if __name__ == "__main__":
    unittest.main()
