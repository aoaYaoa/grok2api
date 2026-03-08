import sys
import types
import unittest
from types import SimpleNamespace

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

from app.services.reverse.app_chat import AppChatReverse


class AppChatReasoningTests(unittest.TestCase):
    def test_build_payload_enables_reasoning_for_thinking_mode(self):
        payload = AppChatReverse.build_payload(
            message='hello',
            model='grok-4',
            mode='THINKING',
        )

        self.assertTrue(payload['isReasoning'])

    def test_build_payload_enables_reasoning_for_grok_420_mode(self):
        payload = AppChatReverse.build_payload(
            message='hello',
            model='grok-4',
            mode='GROK_420',
        )

        self.assertTrue(payload['isReasoning'])


if __name__ == '__main__':
    unittest.main()
