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

from app.api.v1.public_api.video import _with_sse_keepalive


async def delayed_chunks():
    await asyncio.sleep(0.05)
    yield "data: first\n\n"
    await asyncio.sleep(0.05)
    yield "data: second\n\n"


class KeepaliveCloseHangingIterator:
    def __aiter__(self):
        return self

    async def __anext__(self):
        raise RuntimeError("upstream timeout")

    async def aclose(self):
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            await asyncio.sleep(1.0)
            raise


class PublicVideoKeepaliveTests(unittest.IsolatedAsyncioTestCase):
    async def test_propagates_upstream_error_even_when_close_hangs(self):
        async def consume_all():
            async for _ in _with_sse_keepalive(KeepaliveCloseHangingIterator(), interval_seconds=0.01):
                pass

        with self.assertRaises(RuntimeError) as ctx:
            await asyncio.wait_for(consume_all(), timeout=0.4)

        self.assertEqual(str(ctx.exception), "upstream timeout")

    async def test_keepalive_is_emitted_while_waiting_for_upstream_chunks(self):
        chunks = []
        async for chunk in _with_sse_keepalive(delayed_chunks(), interval_seconds=0.01):
            chunks.append(chunk)

        self.assertGreaterEqual(chunks.count(": keepalive\n\n"), 2)
        self.assertIn("data: first\n\n", chunks)
        self.assertIn("data: second\n\n", chunks)
        self.assertLess(chunks.index(": keepalive\n\n"), chunks.index("data: first\n\n"))


if __name__ == "__main__":
    unittest.main()
