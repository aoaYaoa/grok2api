"""
纯 Python 复现 grok.com 的 x-statsig-id 生成（源自 aurora-develop/grok2api）。

算法要点：
- number = floor(now_unix) - 1682924400
- input = METHOD + "!" + PATH + "!" + str(number) + "obfiowerehiring" + HEX
- sha = SHA256(input)
- 70 字节 payload，base64 无 padding（94 字符）
"""

from __future__ import annotations

import base64
import hashlib
import os
import secrets
import threading
import time
from typing import Optional, Tuple

STATSIG_EPOCH = 1682924400
STATSIG_SALT = "obfiowerehiring"
STATSIG_MARK = 0x03

DEFAULT_SEED_B64 = "+yDQu9CyfFekeONYvuXYqIGtrRCE0LBIp1nhdPwaearzhgv5DxHCzznCYxNyIXYY"
DEFAULT_HEX = "388bf10d70a3d70a3d70808cccccccccccd08cccccccccccd0d70a3d70a3d70800"

_lock = threading.RLock()
_cur_seed: bytes = b""
_cur_hex: str = DEFAULT_HEX


def _decode_seed(seed_b64: str) -> bytes:
    raw = (seed_b64 or "").strip()
    if not raw:
        raise ValueError("empty seed")
    for decoder in (base64.b64decode, base64.urlsafe_b64decode):
        try:
            return decoder(raw + "==")
        except Exception:
            pass
    return base64.b64decode(raw, validate=False)


def _init_default_seed() -> bytes:
    seed = _decode_seed(DEFAULT_SEED_B64)
    if len(seed) != 48:
        raise ValueError("default seed must be 48 bytes")
    return seed


_cur_seed = _init_default_seed()


def set_pair(seed_b64: str, hex_value: str) -> None:
    """运行时覆盖 (seed, HEX) 对。"""
    seed = _decode_seed(seed_b64)
    if len(seed) != 48:
        raise ValueError("statsig seed must decode to 48 bytes")
    hx = (hex_value or "").strip()
    if not hx:
        raise ValueError("statsig hex is empty")
    global _cur_seed, _cur_hex
    with _lock:
        _cur_seed, _cur_hex = seed, hx


def apply_pair_from_config() -> None:
    from app.core.config import get_config

    seed = str(get_config("proxy.statsig_seed", "") or "").strip()
    hx = str(get_config("proxy.statsig_hex", "") or "").strip()
    if not seed or not hx:
        return
    try:
        set_pair(seed, hx)
    except Exception:
        pass


def generate(
    pathname: str,
    method: str,
    now_unix: Optional[int] = None,
) -> str:
    """生成 x-statsig-id。"""
    if not pathname:
        pathname = "/rest/app-chat/conversations/new"
    if not method:
        method = "POST"
    ts = int(now_unix if now_unix is not None else time.time())
    with _lock:
        seed = bytes(_cur_seed)
        hx = _cur_hex
    return _build(seed, hx, pathname, method, ts)


def _build(seed: bytes, hex_value: str, pathname: str, method: str, now_unix: int) -> str:
    if len(seed) != 48:
        raise ValueError("seed must be 48 bytes")
    number = (now_unix - STATSIG_EPOCH) & 0xFFFFFFFF
    payload = f"{method}!{pathname}!{number}{STATSIG_SALT}{hex_value}"
    digest = hashlib.sha256(payload.encode("utf-8")).digest()

    key = secrets.randbits(8)
    out = bytearray(70)
    out[0] = key
    for i in range(48):
        out[1 + i] = seed[i] ^ key
    out[49] = (number & 0xFF) ^ key
    out[50] = ((number >> 8) & 0xFF) ^ key
    out[51] = ((number >> 16) & 0xFF) ^ key
    out[52] = ((number >> 24) & 0xFF) ^ key
    for i in range(16):
        out[53 + i] = digest[i] ^ key
    out[69] = STATSIG_MARK ^ key
    return base64.b64encode(bytes(out)).decode("ascii").rstrip("=")


def verify_structure(value: str) -> Tuple[bool, str]:
    """校验 statsig 结构是否自洽（不请求上游）。"""
    try:
        pad = "=" * (-len(value) % 4)
        raw = base64.b64decode(value + pad, validate=False)
    except Exception as exc:
        return False, f"decode failed: {exc}"
    if len(raw) != 70:
        return False, f"bad length {len(raw)}"
    key = raw[0]
    if (raw[69] ^ key) != STATSIG_MARK:
        return False, "bad tail marker"
    return True, "ok"


__all__ = ["generate", "set_pair", "apply_pair_from_config", "verify_structure", "DEFAULT_SEED_B64", "DEFAULT_HEX"]
