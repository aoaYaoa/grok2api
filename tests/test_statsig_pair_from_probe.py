"""浏览器 probe 返回的 Statsig seed/hex 配对处理测试。"""

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from app.services.reverse.browser_bridge import (
    _extract_statsig_pair_from_probe,
    _should_persist_statsig_pair,
)


class StatsigPairFromProbeTest(unittest.TestCase):
    def test_extract_pair_from_probe_payload(self):
        data = {
            "x_statsig_id": "abc",
            "request_headers": {"x-statsig-id": "abc"},
            "statsig_seed": "seed-value",
            "statsig_hex": "hex-value",
        }
        seed, hx = _extract_statsig_pair_from_probe(data)
        self.assertEqual(seed, "seed-value")
        self.assertEqual(hx, "hex-value")

    def test_extract_pair_empty_when_incomplete(self):
        data = {"statsig_seed": "only-seed", "request_headers": {"x": "1"}}
        seed, hx = _extract_statsig_pair_from_probe(data)
        self.assertEqual(seed, "")
        self.assertEqual(hx, "")

    def test_should_persist_when_pair_changed(self):
        self.assertTrue(
            _should_persist_statsig_pair("old-seed", "old-hex", "new-seed", "new-hex")
        )

    def test_should_not_persist_when_unchanged(self):
        self.assertFalse(
            _should_persist_statsig_pair("same", "hex", "same", "hex")
        )


if __name__ == "__main__":
    unittest.main()
