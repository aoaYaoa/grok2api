"""
Grok video generation service.
"""

import asyncio
import inspect
import math
import re
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, AsyncGenerator, AsyncIterable, Dict, List, Optional, Tuple

import orjson
from curl_cffi.requests import AsyncSession
from curl_cffi.requests.errors import RequestsError

from app.core.config import get_config
from app.core.exceptions import (
    UpstreamException,
    AppException,
    ValidationException,
    ErrorType,
    StreamIdleTimeoutError,
)
from app.core.logger import logger
from app.services.grok.services.model import ModelService
from app.services.grok.utils.download import DownloadService
from app.services.grok.utils.process import (
    BaseProcessor,
    _with_idle_timeout,
    _normalize_line,
    _is_http2_error,
)
from app.services.grok.utils.retry import rate_limited
from app.services.grok.utils.stream import wrap_stream_with_usage
from app.services.reverse.app_chat import AppChatReverse
from app.services.reverse.assets_list import AssetsListReverse
from app.services.reverse.media_post import MediaPostReverse
from app.services.reverse.video_upscale import VideoUpscaleReverse
from app.services.token import EffortType, get_token_manager
from app.services.token.manager import BASIC_POOL_NAME
from app.services.grok.utils.upload import UploadService

_VIDEO_SEMAPHORE = None
_VIDEO_SEM_VALUE = 0
_APP_CHAT_MODEL = "grok-3"
_POST_ID_URL_PATTERN = r"/generated/([0-9a-fA-F-]{32,36})/"


@dataclass(frozen=True)
class VideoRoundPlan:
    round_index: int
    total_rounds: int
    is_extension: bool
    video_length: int
    extension_start_time: Optional[float] = None


@dataclass
class VideoRoundResult:
    response_id: str = ""
    post_id: Optional[str] = None
    post_id_rank: int = 999
    video_url: str = ""
    thumbnail_url: str = ""
    last_progress: Any = None
    saw_video_event: bool = False
    stream_errors: List[Any] = field(default_factory=list)


def _pick_str(value: Any) -> str:
    if isinstance(value, str):
        return value.strip()
    return ""


def _extract_post_id_from_video_url(video_url: str) -> Optional[str]:
    if not isinstance(video_url, str) or not video_url:
        return None
    match = re.search(_POST_ID_URL_PATTERN, video_url)
    if match:
        return match.group(1)
    return None


def _extract_video_id(video_url: str) -> str:
    if not video_url:
        return ""
    match = re.search(_POST_ID_URL_PATTERN, video_url)
    if match:
        return match.group(1)
    match = re.search(r"/([0-9a-fA-F-]{32,36})/generated_video", video_url)
    if match:
        return match.group(1)
    return ""


def _build_base_config(
    parent_post_id: str,
    aspect_ratio: str,
    resolution_name: str,
    video_length: int,
) -> Dict[str, Any]:
    return {
        "modelMap": {
            "videoGenModelConfig": {
                "aspectRatio": aspect_ratio,
                "parentPostId": parent_post_id,
                "resolutionName": resolution_name,
                "videoLength": video_length,
                "isVideoEdit": False,
            }
        }
    }


def _build_extension_config(
    *,
    parent_post_id: str,
    extend_post_id: str,
    original_post_id: str,
    original_prompt: str,
    aspect_ratio: str,
    resolution_name: str,
    video_length: int,
    start_time: float,
    stitch_with_extend: bool = True,
) -> Dict[str, Any]:
    video_gen_config = {
        "isVideoExtension": True,
        "videoExtensionStartTime": float(start_time),
        "extendPostId": extend_post_id,
        "originalPostId": original_post_id,
        "originalRefType": "ORIGINAL_REF_TYPE_VIDEO_EXTENSION",
        "mode": "custom",
        "aspectRatio": aspect_ratio,
        "videoLength": video_length,
        "resolutionName": resolution_name,
        "parentPostId": parent_post_id,
        "isVideoEdit": False,
    }
    if stitch_with_extend:
        video_gen_config["stitchWithExtendPostId"] = extend_post_id

    payload = {
        "modelMap": {
            "videoGenModelConfig": video_gen_config
        }
    }
    if original_prompt:
        payload["modelMap"]["videoGenModelConfig"]["originalPrompt"] = original_prompt
    return payload


def _choose_round_length(target_length: int, *, is_super: bool) -> int:
    if not is_super:
        return 6
    return 10 if target_length >= 10 else 6


def _build_round_plan(target_length: int, *, is_super: bool) -> List[VideoRoundPlan]:
    x = _choose_round_length(target_length, is_super=is_super)
    ext_rounds = int(math.ceil(max(target_length - x, 0) / x))
    total_rounds = 1 + ext_rounds

    plan: List[VideoRoundPlan] = [
        VideoRoundPlan(
            round_index=1,
            total_rounds=total_rounds,
            is_extension=False,
            video_length=x,
            extension_start_time=None,
        )
    ]

    for i in range(1, ext_rounds + 1):
        round_target = min(target_length, x * (i + 1))
        start_time = float(round_target - x)
        plan.append(
            VideoRoundPlan(
                round_index=i + 1,
                total_rounds=total_rounds,
                is_extension=True,
                video_length=x,
                extension_start_time=start_time,
            )
        )

    return plan


def _build_round_config(
    plan: VideoRoundPlan,
    *,
    seed_post_id: str,
    last_post_id: str,
    original_post_id: Optional[str],
    prompt: str,
    aspect_ratio: str,
    resolution_name: str,
    stitch_with_extend: bool = True,
) -> Dict[str, Any]:
    if not plan.is_extension:
        return _build_base_config(
            seed_post_id,
            aspect_ratio,
            resolution_name,
            plan.video_length,
        )

    if not original_post_id:
        raise UpstreamException(
            message="Video extension missing original_post_id",
            status_code=502,
            details={"type": "missing_post_id", "round": plan.round_index},
        )

    return _build_extension_config(
        parent_post_id=last_post_id,
        extend_post_id=last_post_id,
        original_post_id=original_post_id,
        original_prompt=prompt,
        aspect_ratio=aspect_ratio,
        resolution_name=resolution_name,
        video_length=plan.video_length,
        start_time=float(plan.extension_start_time or 0.0),
        stitch_with_extend=stitch_with_extend,
    )


def _append_unique_errors(bucket: List[Any], raw_errors: Any):
    if raw_errors is None:
        return

    items = raw_errors if isinstance(raw_errors, list) else [raw_errors]
    for item in items:
        if item is None:
            continue
        text = item if isinstance(item, str) else str(item)
        if text and text not in bucket:
            bucket.append(text)


def _resolve_video_total_timeout() -> float:
    raw = get_config("video.total_timeout", 300)
    try:
        value = float(raw or 0)
    except (TypeError, ValueError):
        value = 300.0
    return max(0.0, value)


def _remaining_before_deadline(deadline: Optional[float]) -> Optional[float]:
    if deadline is None:
        return None
    return deadline - time.monotonic()


def _drain_task_result(task: asyncio.Task[Any]) -> None:
    try:
        task.result()
    except (asyncio.CancelledError, StopAsyncIteration):
        pass
    except Exception:
        pass


async def _best_effort_close_async_iterable(iterable: Any, *, timeout: float = 0.2) -> None:
    aclose = getattr(iterable, "aclose", None)
    if not callable(aclose):
        return
    try:
        result = aclose()
        if not inspect.isawaitable(result):
            return
        close_task = asyncio.create_task(result)
        close_task.add_done_callback(_drain_task_result)
        done, _ = await asyncio.wait(
            {close_task},
            timeout=max(0.0, timeout),
            return_when=asyncio.FIRST_COMPLETED,
        )
        if close_task not in done:
            close_task.cancel()
    except Exception:
        pass


async def _next_with_enforced_timeout(iterator: Any, *, timeout_seconds: float) -> Any:
    if timeout_seconds <= 0:
        return await iterator.__anext__()

    next_task = asyncio.create_task(iterator.__anext__())
    next_task.add_done_callback(_drain_task_result)
    done, _ = await asyncio.wait(
        {next_task},
        timeout=timeout_seconds,
        return_when=asyncio.FIRST_COMPLETED,
    )
    if next_task in done:
        return next_task.result()

    next_task.cancel()
    await _best_effort_close_async_iterable(iterator)
    raise asyncio.TimeoutError()


def _build_video_total_timeout_exception(
    *,
    timeout_seconds: float,
    source: str,
    result: Optional[VideoRoundResult] = None,
) -> UpstreamException:
    payload = result or VideoRoundResult()
    return UpstreamException(
        message=f"Video stream total timeout after {timeout_seconds}s",
        status_code=504,
        details={
            "type": "video_total_timeout",
            "source": source,
            "timeout_seconds": timeout_seconds,
            "response_id": payload.response_id,
            "last_progress": payload.last_progress,
            "stream_errors": list(payload.stream_errors or []),
        },
    )


def _extract_post_id_candidates(resp: Dict[str, Any]) -> List[Tuple[int, str]]:
    candidates: List[Tuple[int, str]] = []

    model_resp = resp.get("modelResponse")
    if isinstance(model_resp, dict):
        file_attachments = model_resp.get("fileAttachments")
        if isinstance(file_attachments, list) and file_attachments:
            first = _pick_str(file_attachments[0])
            if first:
                candidates.append((1, first))

    video_resp = resp.get("streamingVideoGenerationResponse")
    if isinstance(video_resp, dict):
        value = _pick_str(video_resp.get("videoPostId"))
        if value:
            candidates.append((2, value))
        value = _pick_str(video_resp.get("postId"))
        if value:
            candidates.append((3, value))

    post = resp.get("post")
    if isinstance(post, dict):
        value = _pick_str(post.get("id"))
        if value:
            candidates.append((4, value))

    for key in ("postId", "post_id", "parentPostId", "originalPostId"):
        value = _pick_str(resp.get(key))
        if value:
            candidates.append((5, value))

    return candidates


def _apply_post_id_candidates(result: VideoRoundResult, candidates: List[Tuple[int, str]]):
    for rank, value in candidates:
        if rank < result.post_id_rank:
            result.post_id_rank = rank
            result.post_id = value


async def _close_stream_resource(obj: Any):
    if obj is None:
        return

    aclose = getattr(obj, "aclose", None)
    if callable(aclose):
        try:
            await aclose()
        except Exception:
            pass

    close = getattr(obj, "close", None)
    if callable(close):
        try:
            close()
        except Exception:
            pass


async def _iter_round_events(
    response: AsyncIterable[bytes],
    *,
    model: str,
    source: str,
    total_timeout: Optional[float] = None,
) -> AsyncGenerator[Tuple[str, Any], None]:
    result = VideoRoundResult()
    idle_timeout = float(get_config("video.stream_timeout") or 60)
    resolved_total_timeout = (
        _resolve_video_total_timeout()
        if total_timeout is None
        else max(0.0, float(total_timeout or 0))
    )
    started_at = time.monotonic()
    iterator = None

    try:
        iterator = response.__aiter__()
        while True:
            timeout_seconds = idle_timeout
            timeout_reason = "idle"
            remaining_total = None

            if resolved_total_timeout > 0:
                elapsed_total = time.monotonic() - started_at
                remaining_total = resolved_total_timeout - elapsed_total
                if remaining_total <= 0:
                    logger.warning(
                        "Video round total timeout reached before next event",
                        extra={
                            "model": model,
                            "source": source,
                            "timeout_seconds": resolved_total_timeout,
                            "last_progress": result.last_progress,
                            "stream_errors": result.stream_errors,
                        },
                    )
                    raise _build_video_total_timeout_exception(
                        timeout_seconds=resolved_total_timeout,
                        source=source,
                        result=result,
                    )
                if timeout_seconds <= 0 or remaining_total < timeout_seconds:
                    timeout_seconds = remaining_total
                    timeout_reason = "total"

            try:
                if timeout_seconds > 0:
                    raw_line = await asyncio.wait_for(iterator.__anext__(), timeout=timeout_seconds)
                else:
                    raw_line = await iterator.__anext__()
            except asyncio.TimeoutError:
                if timeout_reason == "total":
                    logger.warning(
                        "Video round total timeout while waiting for event",
                        extra={
                            "model": model,
                            "source": source,
                            "timeout_seconds": resolved_total_timeout,
                            "last_progress": result.last_progress,
                            "stream_errors": result.stream_errors,
                        },
                    )
                    raise _build_video_total_timeout_exception(
                        timeout_seconds=resolved_total_timeout,
                        source=source,
                        result=result,
                    )
                raise StreamIdleTimeoutError(timeout_seconds)
            except StopAsyncIteration:
                break

            line = _normalize_line(raw_line)
            if not line:
                continue

            try:
                payload = orjson.loads(line)
            except orjson.JSONDecodeError:
                continue

            root = payload.get("result") if isinstance(payload, dict) else None
            resp = root.get("response") if isinstance(root, dict) else None
            if not isinstance(resp, dict):
                continue

            response_id = _pick_str(resp.get("responseId"))
            if response_id:
                result.response_id = response_id

            _append_unique_errors(result.stream_errors, resp.get("streamErrors"))

            model_resp = resp.get("modelResponse")
            if isinstance(model_resp, dict):
                rid = _pick_str(model_resp.get("responseId"))
                if rid:
                    result.response_id = rid
                _append_unique_errors(result.stream_errors, model_resp.get("streamErrors"))

            _apply_post_id_candidates(result, _extract_post_id_candidates(resp))

            video_resp = resp.get("streamingVideoGenerationResponse")
            progress = None
            if isinstance(video_resp, dict):
                result.saw_video_event = True
                progress = video_resp.get("progress")
                result.last_progress = progress

                url = _pick_str(video_resp.get("videoUrl"))
                if url:
                    result.video_url = url

                thumbnail = _pick_str(video_resp.get("thumbnailImageUrl"))
                if thumbnail:
                    result.thumbnail_url = thumbnail

            if not result.post_id and result.video_url:
                result.post_id = _extract_post_id_from_video_url(result.video_url)
                if result.post_id:
                    result.post_id_rank = 6

            if progress is not None:
                yield "progress", progress

        if not result.post_id and result.video_url:
            result.post_id = _extract_post_id_from_video_url(result.video_url)
            if result.post_id:
                result.post_id_rank = 6

        yield "done", result
    except StreamIdleTimeoutError as e:
        raise UpstreamException(
            message=f"Video stream idle timeout after {e.idle_seconds}s",
            status_code=504,
            details={
                "type": "stream_idle_timeout",
                "source": source,
                "idle_seconds": e.idle_seconds,
                "error": str(e),
            },
        )
    except RequestsError as e:
        if _is_http2_error(e):
            raise UpstreamException(
                message="Upstream connection closed unexpectedly",
                status_code=502,
                details={
                    "type": "http2_stream_error",
                    "source": source,
                    "error": str(e),
                },
            )
        raise UpstreamException(
            message=f"Upstream request failed: {e}",
            status_code=502,
            details={
                "type": "upstream_request_failed",
                "source": source,
                "error": str(e),
            },
        )
    finally:
        await _close_stream_resource(iterator)
        await _close_stream_resource(response)


async def _collect_round_result(
    response: AsyncIterable[bytes],
    *,
    model: str,
    source: str,
    total_timeout: Optional[float] = None,
) -> VideoRoundResult:
    result = VideoRoundResult()
    async for event_type, payload in _iter_round_events(
        response,
        model=model,
        source=source,
        total_timeout=total_timeout,
    ):
        if event_type == "done":
            result = payload
    return result


def _round_error_details(
    result: VideoRoundResult,
    *,
    err_type: str,
    round_index: int,
    total_rounds: int,
) -> Dict[str, Any]:
    return {
        "type": err_type,
        "round": round_index,
        "total_rounds": total_rounds,
        "response_id": result.response_id,
        "last_progress": result.last_progress,
        "stream_errors": result.stream_errors,
    }


def _ensure_round_result(
    result: VideoRoundResult,
    *,
    round_index: int,
    total_rounds: int,
    final_round: bool,
):
    if not result.post_id:
        err_type = "moderated_or_stream_errors" if result.stream_errors else "missing_post_id"
        raise UpstreamException(
            message=f"Video round {round_index}/{total_rounds} missing post_id",
            status_code=502,
            details=_round_error_details(
                result,
                err_type=err_type,
                round_index=round_index,
                total_rounds=total_rounds,
            ),
        )

    if not final_round:
        return

    if result.video_url:
        return

    if result.stream_errors:
        err_type = "moderated_or_stream_errors"
    elif result.saw_video_event:
        err_type = "missing_video_url"
    else:
        err_type = "empty_video_stream"

    raise UpstreamException(
        message=f"Video round {round_index}/{total_rounds} missing final video_url",
        status_code=502,
        details=_round_error_details(
            result,
            err_type=err_type,
            round_index=round_index,
            total_rounds=total_rounds,
        ),
    )


def _format_progress(value: Any) -> str:
    if isinstance(value, bool):
        return str(int(value))
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        if value.is_integer():
            return str(int(value))
        return f"{value:.2f}".rstrip("0").rstrip(".")
    if isinstance(value, str):
        stripped = value.strip()
        if stripped:
            return stripped
    return str(value)


def _get_video_semaphore() -> asyncio.Semaphore:
    """Reverse 接口并发控制（video 服务）。"""
    global _VIDEO_SEMAPHORE, _VIDEO_SEM_VALUE
    raw = get_config("video.concurrent")
    value = max(1, int(raw or 1))
    if value != _VIDEO_SEM_VALUE:
        _VIDEO_SEM_VALUE = value
        _VIDEO_SEMAPHORE = asyncio.Semaphore(value)
    return _VIDEO_SEMAPHORE


def _token_tag(token: str) -> str:
    raw = token[4:] if token.startswith("sso=") else token
    if not raw:
        return "empty"
    if len(raw) <= 14:
        return raw
    return f"{raw[:6]}...{raw[-6:]}"


async def _fetch_media_post_info(token: str, post_id: str) -> dict[str, Any]:
    """查询官方 post 元信息，统一获得 canonical mediaUrl。"""
    token_text = str(token or "").strip()
    post_text = str(post_id or "").strip()
    if not token_text or not post_text:
        return {}
    try:
        async with AsyncSession() as session:
            response = await MediaPostReverse.get(session, token_text, post_text)
        payload = response.json() if response is not None else {}
        if isinstance(payload, dict):
            return payload.get("post", {}) or {}
    except Exception as e:
        logger.warning(
            "Video media_post/get failed: "
            f"post_id={post_text}, token={_token_tag(token_text)}, error={e}"
        )
    return {}


async def _canonicalize_parent_media_url(
    token: str,
    parent_post_id: str,
    source_image_url: str = "",
) -> str:
    """优先使用官方 mediaUrl，避免继续依赖本地猜测路径。"""
    raw_url = str(source_image_url or "").strip()
    if "imagine-public.x.ai/imagine-public/share-images/" in raw_url:
        return raw_url
    post = await _fetch_media_post_info(token, parent_post_id)
    media_url = str(post.get("mediaUrl") or "").strip()
    if media_url:
        return media_url
    thumbnail_url = str(post.get("thumbnailImageUrl") or "").strip()
    if thumbnail_url:
        return thumbnail_url
    if raw_url:
        return raw_url
    return VideoService._build_imagine_public_url(parent_post_id)


def _normalize_assets_url(value: str) -> str:
    raw = str(value or "").strip()
    if not raw:
        return ""
    if raw.startswith("http://") or raw.startswith("https://"):
        return raw
    if raw.startswith("/"):
        return f"https://assets.grok.com{raw}"
    return f"https://assets.grok.com/{raw}"

def _log_final_video_payload(
    *,
    message: str,
    file_attachments: list[str] | None = None,
    tool_overrides: dict | None = None,
    model_config_override: dict | None = None,
    mode: str | None = None,
) -> None:
    payload = {
        "message": str(message or ""),
        "fileAttachments": [
            str(item or "").strip()
            for item in (file_attachments or [])
            if str(item or "").strip()
        ],
        "toolOverrides": tool_overrides or {},
        "modelConfigOverride": model_config_override or {},
        "mode": mode,
    }
    try:
        payload_text = orjson.dumps(payload, option=orjson.OPT_INDENT_2).decode("utf-8")
    except Exception:
        payload_text = str(payload)
    logger.info(f"Video upstream payload before send:\n{payload_text}")


def _classify_video_error(exc: Exception) -> tuple[str, str, int]:
    """将底层异常归一化为用户可读错误。"""
    text = str(exc or "").lower()
    details = getattr(exc, "details", None)
    body = ""
    err_type = ""
    if isinstance(details, dict):
        body = str(details.get("body") or "").lower()
        err_type = str(details.get("type") or "").lower()
    merged = f"{text}\n{body}"

    if (
        "blocked by moderation" in merged
        or "content moderated" in merged
        or "content-moderated" in merged
        or '"code":3' in merged
        or "'code': 3" in merged
    ):
        return ("视频生成被拒绝，请调整提示词或素材后重试", "video_rejected", 400)

    if err_type == "video_extension_token_unbound":
        return (
            "视频延长失败：当前 token 池中没有可访问该视频的账号，请使用通过当前服务生成的视频 post_id，或先绑定原视频账号的 token。",
            "video_extension_token_unbound",
            502,
        )

    if err_type in {"empty_video_stream", "missing_video_url"}:
        return ("视频生成失败：上游未返回视频结果，请稍后重试", "video_empty_stream", 502)

    if (
        (isinstance(details, dict) and details.get("type") == "video_total_timeout")
        or "total timeout" in merged
    ):
        return ("视频生成超时（5分钟），请稍后重试", "video_total_timeout", 504)

    if (
        "tls connect error" in merged
        or "could not establish signal connection" in merged
        or "timed out" in merged
        or "timeout" in merged
        or "connection closed" in merged
        or "http/2" in merged
        or "curl: (35)" in merged
        or "network" in merged
        or "proxy" in merged
    ):
        return ("视频生成失败：网络连接异常，请稍后重试", "video_network_error", 502)

    return ("视频生成失败，请稍后重试", "video_failed", 502)


def _extract_upstream_status(exc: Exception) -> int | None:
    details = getattr(exc, "details", None)
    status = None
    if isinstance(details, dict):
        status = details.get("status")
    if status is None:
        status = getattr(exc, "status_code", None)
    try:
        return int(status) if status is not None else None
    except (TypeError, ValueError):
        return None


def _is_video_auth_error(exc: Exception) -> bool:
    status = _extract_upstream_status(exc)
    if status == 401:
        return True

    text = str(exc or "").lower()
    details = getattr(exc, "details", None)
    body = ""
    if isinstance(details, dict):
        body = str(details.get("body") or details.get("error") or "").lower()
    merged = f"{text}\n{body}"
    markers = (
        "invalid-credentials",
        "unauthenticated",
        "failed to look up session id",
    )
    return any(marker in merged for marker in markers)


async def _resolve_video_asset_path(token: str, asset_id: str) -> tuple[str, str]:
    """当流里未返回 videoUrl 时，尝试从 assets 接口反查 key。"""
    if not asset_id or not token:
        return "", ""

    retries = 3
    delay = 1.5
    page_size = 50
    max_pages = 20
    marker = f"/{asset_id}/"

    async with AsyncSession() as session:
        for attempt in range(1, retries + 1):
            params = {
                "pageSize": page_size,
                "orderBy": "ORDER_BY_LAST_USE_TIME",
                "source": "SOURCE_ANY",
                "isLatest": "true",
            }
            page_token = ""
            page_count = 0
            try:
                while True:
                    if page_token:
                        params["pageToken"] = page_token
                    else:
                        params.pop("pageToken", None)

                    response = await AssetsListReverse.request(session, token, params)
                    data = response.json() if response is not None else {}
                    assets = data.get("assets", []) if isinstance(data, dict) else []

                    for asset in assets:
                        if not isinstance(asset, dict):
                            continue
                        current_asset_id = str(asset.get("assetId", "")).strip()
                        key = str(asset.get("key", "")).strip()
                        mime_type = str(asset.get("mimeType", "")).lower()
                        if (
                            current_asset_id == asset_id
                            or marker in key
                            or key.endswith(f"{asset_id}/content")
                        ):
                            if mime_type.startswith("video/") or "generated_video" in key:
                                preview_key = str(asset.get("previewImageKey", "")).strip()
                                if not preview_key:
                                    aux = asset.get("auxKeys") or {}
                                    if isinstance(aux, dict):
                                        preview_key = str(aux.get("preview-image", "")).strip()
                                logger.info(
                                    "Video asset resolved by assets list: "
                                    f"asset_id={asset_id}, key={key}, preview={preview_key}"
                                )
                                return key, preview_key

                    page_token = str(data.get("nextPageToken", "")).strip()
                    page_count += 1
                    if not page_token or page_count >= max_pages:
                        break
            except Exception as e:
                logger.warning(
                    f"Video asset resolve failed (attempt={attempt}/{retries}): {e}"
                )

            if attempt < retries:
                await asyncio.sleep(delay)

    return "", ""


async def _request_round_stream(
    *,
    token: str,
    message: str,
    model_config_override: Dict[str, Any],
) -> AsyncGenerator[bytes, None]:
    async def _stream():
        session = AsyncSession()
        try:
            async with _get_video_semaphore():
                stream_response = await AppChatReverse.request(
                    session,
                    token,
                    message=message,
                    model=_APP_CHAT_MODEL,
                    tool_overrides={"videoGen": True},
                    model_config_override=model_config_override,
                )
                async for line in stream_response:
                    yield line
        finally:
            try:
                await session.close()
            except Exception:
                pass

    return _stream()


async def _upscale_video_url(token: str, video_url: str) -> Tuple[str, bool]:
    """Returns (url, upscaled)."""
    video_id = _extract_video_id(video_url)
    if not video_id:
        logger.warning("Video upscale skipped: unable to extract video id")
        return video_url, False

    try:
        async with AsyncSession() as session:
            response = await VideoUpscaleReverse.request(session, token, video_id)
        payload = response.json() if response is not None else {}
        hd_url = payload.get("hdMediaUrl") if isinstance(payload, dict) else None
        hd_url = _pick_str(hd_url)
        if hd_url:
            logger.info(f"Video upscale completed: {hd_url}")
            return hd_url, True
    except Exception as e:
        logger.warning(f"Video upscale failed: {e}")

    return video_url, False


def _resolve_upscale_timing() -> str:
    raw = get_config("video.upscale_timing", "complete")
    value = str(raw or "complete").strip().lower()
    if value in {"single", "complete"}:
        return value
    logger.warning(f"Invalid video.upscale_timing={raw!r}, fallback to 'complete'")
    return "complete"


class _VideoChainSSEWriter:
    def __init__(self, model: str, show_think: bool):
        self.model = model
        self.show_think = bool(show_think)
        self.created = int(time.time())
        self.response_id = f"chatcmpl-{uuid.uuid4().hex[:24]}"
        self.role_sent = False
        self.think_opened = False

    def _sse(self, content: str = "", role: str = None, finish: str = None) -> str:
        delta: Dict[str, Any] = {}
        if role:
            delta["role"] = role
            delta["content"] = ""
        elif content:
            delta["content"] = content

        chunk = {
            "id": self.response_id,
            "object": "chat.completion.chunk",
            "created": self.created,
            "model": self.model,
            "choices": [
                {
                    "index": 0,
                    "delta": delta,
                    "logprobs": None,
                    "finish_reason": finish,
                }
            ],
        }
        return f"data: {orjson.dumps(chunk).decode()}\n\n"

    def ensure_role(self) -> List[str]:
        if self.role_sent:
            return []
        self.role_sent = True
        return [self._sse(role="assistant")]

    def emit_progress(self, *, round_index: int, total_rounds: int, progress: Any) -> List[str]:
        if not self.show_think:
            return []

        chunks = self.ensure_role()
        if not self.think_opened:
            self.think_opened = True
            chunks.append(self._sse("<think>\n"))

        progress_text = _format_progress(progress)
        chunks.append(self._sse(f"[round={round_index}/{total_rounds}] progress={progress_text}%\n"))
        return chunks

    def emit_note(self, text: str) -> List[str]:
        if not self.show_think:
            return []

        chunks = self.ensure_role()
        if not self.think_opened:
            self.think_opened = True
            chunks.append(self._sse("<think>\n"))
        chunks.append(self._sse(text))
        return chunks

    def emit_content(self, text: str) -> List[str]:
        chunks = self.ensure_role()
        if self.think_opened:
            self.think_opened = False
            chunks.append(self._sse("\n</think>\n"))
        if text:
            chunks.append(self._sse(text))
        return chunks

    def finish(self) -> List[str]:
        chunks = self.ensure_role()
        if self.think_opened:
            self.think_opened = False
            chunks.append(self._sse("\n</think>\n"))
        chunks.append(self._sse(finish="stop"))
        chunks.append("data: [DONE]\n\n")
        return chunks


class VideoService:
    """Video generation service."""

    def __init__(self):
        self.timeout = None

    @staticmethod
    def is_meaningful_video_prompt(prompt: str) -> bool:
        """判断提示词是否属于“有效自定义视频提示词”。

        以下场景视为非自定义（返回 False）：
        - 空提示词
        - 仅“让它动起来/生成视频/animate this”等泛化短提示
        """
        text = (prompt or "").strip().lower()
        if not text:
            return False

        # 统一空白与常见收尾标点
        text = re.sub(r"\s+", " ", text).strip(
            " \t\r\n.,!?;:，。！？；：'\"`~()[]{}<>《》「」【】"
        )
        key = re.sub(r"\s+", "", text)
        if not text:
            return False

        generic_en = {
            "animate",
            "animate this",
            "animate this image",
            "make it move",
            "make this move",
            "generate video",
            "make video",
            "make a video",
            "create video",
            "turn this into a video",
            "turn it into a video",
            "video",
        }
        generic_zh = {
            "动起来",
            "让它动起来",
            "让图片动起来",
            "让这张图动起来",
            "生成视频",
            "生成一个视频",
            "生成一段视频",
            "做成视频",
            "做个视频",
            "制作视频",
            "变成视频",
            "变成一个视频",
            "视频",
        }
        if text in generic_en or key in generic_zh:
            return False

        # 英文泛化短句：please animate this / please generate a video
        if re.fullmatch(r"(please\s+)?animate(\s+this(\s+image)?)?", text):
            return False
        if re.fullmatch(
            r"(please\s+)?(make|create|generate)\s+(a\s+)?video", text
        ):
            return False

        # 中文泛化短句：请让它动起来 / 帮我生成视频 / 把这张图做成视频
        if re.fullmatch(
            r"(请|请你|帮我|麻烦你)?(把)?(它|图片|这张图)?"
            r"(动起来|生成视频|做成视频|制作视频)(吧|一下|下)?",
            key,
        ):
            return False

        return True

    @staticmethod
    def _map_preset_to_mode(preset: str) -> str:
        """将前端预设名映射为 Grok 官方 mode 参数。"""
        mapping = {
            "spicy": "extremely-spicy-or-crazy",
            "fun": "extremely-crazy",
            "normal": "normal",
        }
        # 如果预设没发送，默认 --mode=extremely-spicy-or-crazy (即 spicy)
        return mapping.get(preset, "extremely-spicy-or-crazy")

    @staticmethod
    def _build_video_message(
        prompt: str,
        preset: str = "normal",
        source_image_url: str = "",
    ) -> str:
        """构造视频请求 message：
        - 有提示词：统一走 custom，并发送 image_url + prompt + mode
        - 无提示词：根据所选 preset 转换 mode
        """
        prompt_text = (prompt or "").strip()
        if not VideoService.is_meaningful_video_prompt(prompt_text):
            prompt_text = ""

        image_core = (source_image_url or "").strip()
        if image_core.startswith("data:"):
            image_core = ""
        if prompt_text:
            mode_flag = "--mode=custom"
            if image_core:
                return f"{image_core}  {prompt_text} {mode_flag}"
            return f"{prompt_text} {mode_flag}"

        # 无提示词（或泛化指令）
        official_mode = VideoService._map_preset_to_mode(preset)
        mode_flag = f"--mode={official_mode}"
        if image_core:
            return f"{image_core}  {mode_flag}"
        return mode_flag

    @staticmethod
    def _build_imagine_public_url(parent_post_id: str) -> str:
        return f"https://imagine-public.x.ai/imagine-public/images/{parent_post_id}.jpg"

    @staticmethod
    def _is_moderated_line(line: bytes) -> bool:
        text = _normalize_line(line)
        if not text:
            return False
        try:
            data = orjson.loads(text)
        except Exception:
            return False
        resp = data.get("result", {}).get("response", {})
        video_resp = resp.get("streamingVideoGenerationResponse", {})
        return bool(video_resp.get("moderated") is True)

    async def create_post(
        self,
        token: str,
        prompt: str,
        media_type: str = "MEDIA_POST_TYPE_VIDEO",
        media_url: str = None,
    ) -> str:
        """Create media post and return post ID."""
        try:
            if media_type == "MEDIA_POST_TYPE_IMAGE" and not media_url:
                raise ValidationException("media_url is required for image posts")

            prompt_value = prompt if media_type == "MEDIA_POST_TYPE_VIDEO" else ""
            media_value = media_url or ""

            async with AsyncSession() as session:
                async with _get_video_semaphore():
                    response = await MediaPostReverse.request(
                        session,
                        token,
                        media_type,
                        media_value,
                        prompt=prompt_value,
                    )

            post_id = response.json().get("post", {}).get("id", "")
            if not post_id:
                raise UpstreamException("No post ID in response")

            logger.info(f"Media post created: {post_id} (type={media_type})")
            return post_id

        except AppException:
            raise
        except Exception as e:
            logger.error(f"Create post error: {e}")
            msg, code, status = _classify_video_error(e)
            raise AppException(
                message=msg,
                error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                code=code,
                status_code=status,
            )

    async def create_image_post(self, token: str, image_url: str) -> str:
        """Create image post and return post ID."""
        media_url = str(image_url or "").strip()
        if media_url.startswith("data:"):
            upload_service = UploadService()
            try:
                _, file_uri = await upload_service.upload_file(media_url, token)
            finally:
                await upload_service.close()
            media_url = _normalize_assets_url(file_uri)
            logger.info(
                "Image post source uploaded: "
                f"original=data-uri, media_url={media_url[:120]}"
            )
        return await self.create_post(
            token, prompt="", media_type="MEDIA_POST_TYPE_IMAGE", media_url=media_url
        )

    async def _resolve_reference_source_url(
        self,
        token: str,
        item: dict[str, Any],
    ) -> str:
        parent_post_id = str(item.get("parent_post_id") or "").strip()
        image_url = str(item.get("image_url") or "").strip()
        source_image_url = str(item.get("source_image_url") or "").strip()

        if parent_post_id:
            return await _canonicalize_parent_media_url(
                token,
                parent_post_id,
                source_image_url or image_url,
            )
        return source_image_url or image_url

    async def _upload_reference_items(
        self,
        token: str,
        reference_items: list[dict[str, Any]],
    ) -> list[dict[str, str]]:
        uploaded: list[dict[str, str]] = []
        upload_service = UploadService()
        try:
            for index, item in enumerate(reference_items):
                source_url = await self._resolve_reference_source_url(token, item)
                if not source_url:
                    raise ValidationException(f"第 {index + 1} 张参考图缺少可用来源")
                uploaded_file_id, file_uri = await upload_service.upload_file(
                    source_url, token
                )
                file_id = str(uploaded_file_id or "").strip()
                asset_url = _normalize_assets_url(file_uri)
                uploaded.append(
                    {
                        "file_id": str(file_id or "").strip(),
                        "asset_url": asset_url,
                        "source_url": source_url,
                        "parent_post_id": str(item.get("parent_post_id") or "").strip(),
                        "mention_alias": str(item.get("mention_alias") or "").strip(),
                    }
                )
        finally:
            await upload_service.close()
        return uploaded

    async def generate_from_reference_items(
        self,
        token: str,
        prompt: str,
        reference_items: list[dict[str, Any]],
        aspect_ratio: str = "3:2",
        video_length: int = 6,
        resolution: str = "480p",
        preset: str = "normal",
    ) -> AsyncGenerator[bytes, None]:
        token_tag = _token_tag(token)
        uploaded_refs = await self._upload_reference_items(token, reference_items)
        if not uploaded_refs:
            raise ValidationException("至少需要 1 张参考图")

        prompt_text = (prompt or "").strip()
        alias_map: dict[str, str] = {}
        ref_tokens: list[str] = []
        for index, item in enumerate(uploaded_refs, start=1):
            file_id = str(item.get("file_id") or "").strip()
            if not file_id:
                continue
            token_text = f"@{file_id}"
            ref_tokens.append(token_text)
            alias = str(item.get("mention_alias") or "").strip() or f"Image {index}"
            alias_map[f"@{alias}"] = token_text
            alias_map[f"@{alias.replace(' ', '')}"] = token_text

        for alias, token_text in alias_map.items():
            if alias in prompt_text:
                prompt_text = prompt_text.replace(alias, token_text)

        if ref_tokens:
            has_mentions = any(token_text in prompt_text for token_text in ref_tokens)
            if not has_mentions:
                prompt_text = f"{' '.join(ref_tokens)} {prompt_text}".strip()

        official_mode = "custom" if VideoService.is_meaningful_video_prompt(prompt_text) else "custom"
        post_id = await self.create_post(token, prompt_text, media_type="MEDIA_POST_TYPE_VIDEO")
        image_references = [item["asset_url"] for item in uploaded_refs if item.get("asset_url")]
        file_attachments = [
            str(item.get("file_id") or "").strip()
            for item in uploaded_refs
            if str(item.get("file_id") or "").strip()
        ]
        message = f"{prompt_text} --mode=custom".strip()
        model_config_override = {
            "modelMap": {
                "videoGenModelConfig": {
                    "parentPostId": post_id,
                    "aspectRatio": aspect_ratio,
                    "videoLength": video_length,
                    "resolutionName": resolution,
                    "isReferenceToVideo": True,
                    "imageReferences": image_references,
                }
            }
        }
        moderated_max_retry = max(1, int(get_config("video.moderated_max_retry", 5)))

        logger.info(
            "Multi-reference video request prepared: "
            f"token={token_tag}, reference_count={len(uploaded_refs)}, post_id={post_id}, "
            f"resolution={resolution}, video_length={video_length}, ratio={aspect_ratio}, mode={official_mode}"
        )

        async def _stream():
            for attempt in range(1, moderated_max_retry + 1):
                session = AsyncSession()
                moderated_hit = False
                try:
                    async with _get_video_semaphore():
                        _log_final_video_payload(
                            message=message,
                            file_attachments=file_attachments,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                            mode=official_mode,
                        )
                        stream_response = await AppChatReverse.request(
                            session,
                            token,
                            message=message,
                            model="grok-3",
                            mode=official_mode,
                            file_attachments=file_attachments,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                        )
                        logger.info(
                            "Multi-reference video generation started: "
                            f"token={token_tag}, post_id={post_id}, attempt={attempt}/{moderated_max_retry}"
                        )
                        async for line in stream_response:
                            if self._is_moderated_line(line):
                                moderated_hit = True
                                logger.warning(
                                    f"Multi-reference video moderated: token={token_tag}, retry {attempt}/{moderated_max_retry}"
                                )
                                break
                            yield line

                    if not moderated_hit:
                        return
                    if attempt < moderated_max_retry:
                        await asyncio.sleep(1.2)
                        continue
                    raise UpstreamException(
                        "Video blocked by moderation",
                        status_code=400,
                        details={"moderated": True, "attempts": moderated_max_retry},
                    )
                except Exception as e:
                    logger.error(f"Multi-reference video generation error: {e}")
                    if isinstance(e, AppException):
                        raise
                    msg, code, status = _classify_video_error(e)
                    raise AppException(
                        message=msg,
                        error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                        code=code,
                        status_code=status,
                    )
                finally:
                    try:
                        await session.close()
                    except Exception:
                        pass

        return _stream()

    async def generate(
        self,
        token: str,
        prompt: str,
        aspect_ratio: str = "3:2",
        video_length: int = 6,
        resolution_name: str = "480p",
        preset: str = "normal",
    ) -> AsyncGenerator[bytes, None]:
        """Generate video."""
        token_tag = _token_tag(token)
        # 确定逻辑上的 mode
        is_custom = VideoService.is_meaningful_video_prompt(prompt)
        official_mode = "custom" if is_custom else VideoService._map_preset_to_mode(preset)

        logger.info(
            f"Video generation: token={token_tag}, prompt='{prompt[:50]}...', ratio={aspect_ratio}, length={video_length}s, mode={official_mode}"
        )
        post_id = await self.create_post(token, prompt)
        message = self._build_video_message(prompt=prompt, preset=preset)
        model_config_override = {
            "modelMap": {
                "videoGenModelConfig": {
                    "aspectRatio": aspect_ratio,
                    "parentPostId": post_id,
                    "resolutionName": resolution_name,
                    "videoLength": video_length,
                    "isVideoEdit": False,
                }
            }
        }
        moderated_max_retry = max(1, int(get_config("video.moderated_max_retry", 5)))

        async def _stream():
            for attempt in range(1, moderated_max_retry + 1):
                session = AsyncSession()
                moderated_hit = False
                try:
                    async with _get_video_semaphore():
                        _log_final_video_payload(
                            message=message,
                            file_attachments=[],
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                            mode=official_mode,
                        )
                        stream_response = await AppChatReverse.request(
                            session,
                            token,
                            message=message,
                            model="grok-3",
                            mode=official_mode,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                        )
                        logger.info(
                            f"Video generation started: token={token_tag}, post_id={post_id}, attempt={attempt}/{moderated_max_retry}"
                        )
                        async for line in stream_response:
                            if self._is_moderated_line(line):
                                moderated_hit = True
                                logger.warning(
                                    f"Video generation moderated: token={token_tag}, retry {attempt}/{moderated_max_retry}"
                                )
                                break
                            yield line

                    if not moderated_hit:
                        return
                    if attempt < moderated_max_retry:
                        await asyncio.sleep(1.2)
                        continue
                    raise UpstreamException(
                        "Video blocked by moderation",
                        status_code=400,
                        details={"moderated": True, "attempts": moderated_max_retry},
                    )
                except Exception as e:
                    logger.error(f"Video generation error: {e}")
                    if isinstance(e, AppException):
                        raise
                    msg, code, status = _classify_video_error(e)
                    raise AppException(
                        message=msg,
                        error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                        code=code,
                        status_code=status,
                    )
                finally:
                    try:
                        await session.close()
                    except Exception:
                        pass

        return _stream()

    async def generate_from_image(
        self,
        token: str,
        prompt: str,
        image_url: str,
        aspect_ratio: str = "3:2",
        video_length: int = 6,
        resolution: str = "480p",
        preset: str = "normal",
    ) -> AsyncGenerator[bytes, None]:
        """Generate video from image."""
        token_tag = _token_tag(token)
        normalized_image_url = str(image_url or "").strip()
        if normalized_image_url.startswith("data:"):
            upload_service = UploadService()
            try:
                _, file_uri = await upload_service.upload_file(normalized_image_url, token)
                normalized_image_url = f"https://assets.grok.com/{file_uri}"
                logger.info(
                    "Image to video source uploaded before generation: "
                    f"token={token_tag}, asset_url={normalized_image_url}"
                )
            finally:
                await upload_service.close()

        # 确定逻辑上的 mode
        is_custom = VideoService.is_meaningful_video_prompt(prompt)
        official_mode = "custom" if is_custom else VideoService._map_preset_to_mode(preset)

        logger.info(
            f"Image to video: token={token_tag}, prompt='{prompt[:50]}...', image={normalized_image_url[:80]}, mode={official_mode}"
        )
        post_id = await self.create_image_post(token, normalized_image_url)
        message = self._build_video_message(
            prompt=prompt,
            preset=preset,
            source_image_url=normalized_image_url,
        )
        model_config_override = {
            "modelMap": {
                "videoGenModelConfig": {
                    "aspectRatio": aspect_ratio,
                    "parentPostId": post_id,
                    "resolutionName": resolution,
                    "videoLength": video_length,
                    "isVideoEdit": False,
                }
            }
        }
        moderated_max_retry = max(1, int(get_config("video.moderated_max_retry", 5)))

        async def _stream():
            for attempt in range(1, moderated_max_retry + 1):
                session = AsyncSession()
                moderated_hit = False
                try:
                    async with _get_video_semaphore():
                        _log_final_video_payload(
                            message=message,
                            file_attachments=[],
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                            mode=official_mode,
                        )
                        stream_response = await AppChatReverse.request(
                            session,
                            token,
                            message=message,
                            model="grok-3",
                            mode=official_mode,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                        )
                        logger.info(
                            f"Video generation started: token={token_tag}, post_id={post_id}, attempt={attempt}/{moderated_max_retry}"
                        )
                        async for line in stream_response:
                            if self._is_moderated_line(line):
                                moderated_hit = True
                                logger.warning(
                                    f"Video generation moderated: token={token_tag}, retry {attempt}/{moderated_max_retry}"
                                )
                                break
                            yield line

                    if not moderated_hit:
                        return
                    if attempt < moderated_max_retry:
                        await asyncio.sleep(1.2)
                        continue
                    raise UpstreamException(
                        "Video blocked by moderation",
                        status_code=400,
                        details={"moderated": True, "attempts": moderated_max_retry},
                    )
                except Exception as e:
                    logger.error(f"Video generation error: {e}")
                    if isinstance(e, AppException):
                        raise
                    msg, code, status = _classify_video_error(e)
                    raise AppException(
                        message=msg,
                        error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                        code=code,
                        status_code=status,
                    )
                finally:
                    try:
                        await session.close()
                    except Exception:
                        pass

        return _stream()

    async def generate_from_parent_post(
        self,
        token: str,
        prompt: str,
        parent_post_id: str,
        source_image_url: str = "",
        aspect_ratio: str = "3:2",
        video_length: int = 6,
        resolution: str = "480p",
        preset: str = "normal",
    ) -> AsyncGenerator[bytes, None]:
        """Generate video by existing parent post ID (preferred path)."""
        token_tag = _token_tag(token)
        is_custom = VideoService.is_meaningful_video_prompt(prompt)
        logger.info(
            f"ParentPost to video: token={token_tag}, prompt='{prompt[:50]}...', parent_post_id={parent_post_id}"
        )
        raw_source_image_url = (source_image_url or "").strip()
        source_image_url = self._build_imagine_public_url(parent_post_id)
        if raw_source_image_url and raw_source_image_url != source_image_url:
            logger.info(
                "ParentPost source image normalized to imagine-public: "
                f"token={token_tag}, parent_post_id={parent_post_id}, "
                f"raw_source_image_url={raw_source_image_url}, normalized_source_image_url={source_image_url}"
            )

        # 对齐官网全链路：先创建 IMAGE 类型 media post，再触发 conversations/new。
        # 注意：videoGenModelConfig.parentPostId 仍使用 imagine 的 image_id。
        try:
            created_image_post_id = await self.create_image_post(token, source_image_url)
            logger.info(
                "ParentPost pre-create media post done: "
                f"parent_post_id={parent_post_id}, image_post_id={created_image_post_id}, "
                f"media_url={source_image_url}"
            )
        except Exception as e:
            logger.warning(
                "ParentPost pre-create media post failed, continue anyway: "
                f"parent_post_id={parent_post_id}, media_url={source_image_url}, error={e}"
            )

        message = self._build_video_message(
            prompt=prompt,
            preset=preset,
            source_image_url=source_image_url,
        )
        model_config_override = {
            "modelMap": {
                "videoGenModelConfig": {
                    "aspectRatio": aspect_ratio,
                    "parentPostId": parent_post_id,
                    "resolutionName": resolution,
                    "videoLength": video_length,
                    "isVideoEdit": False,
                }
            }
        }
        moderated_max_retry = max(1, int(get_config("video.moderated_max_retry", 5)))

        # 确定逻辑上的 mode
        is_custom = VideoService.is_meaningful_video_prompt(prompt)
        official_mode = "custom" if is_custom else VideoService._map_preset_to_mode(preset)

        logger.info(
            "ParentPost video request prepared: "
            f"token={token_tag}, parent_post_id={parent_post_id}, "
            f"message_len={len(message)}, has_prompt={is_custom}, "
            f"resolution={resolution}, video_length={video_length}, ratio={aspect_ratio}, mode={official_mode}"
        )

        async def _stream():
            for attempt in range(1, moderated_max_retry + 1):
                session = AsyncSession()
                moderated_hit = False
                try:
                    async with _get_video_semaphore():
                        _log_final_video_payload(
                            message=message,
                            file_attachments=[],
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                            mode=official_mode,
                        )
                        stream_response = await AppChatReverse.request(
                            session,
                            token,
                            message=message,
                            model="grok-3",
                            mode=official_mode,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                        )
                        logger.info(
                            "Video generation started by parentPostId: "
                            f"token={token_tag}, parent_post_id={parent_post_id}, attempt={attempt}/{moderated_max_retry}"
                        )
                        async for line in stream_response:
                            if self._is_moderated_line(line):
                                moderated_hit = True
                                logger.warning(
                                    f"Video generation moderated: token={token_tag}, retry {attempt}/{moderated_max_retry}"
                                )
                                break
                            yield line

                    if not moderated_hit:
                        return
                    if attempt < moderated_max_retry:
                        await asyncio.sleep(1.2)
                        continue
                    raise UpstreamException(
                        "Video blocked by moderation",
                        status_code=400,
                        details={"moderated": True, "attempts": moderated_max_retry},
                    )
                except Exception as e:
                    logger.error(f"Video generation error: {e}")
                    if isinstance(e, AppException):
                        raise
                    msg, code, status = _classify_video_error(e)
                    raise AppException(
                        message=msg,
                        error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                        code=code,
                        status_code=status,
                    )
                finally:
                    try:
                        await session.close()
                    except Exception:
                        pass

        return _stream()

    async def generate_extend_video(
        self,
        token: str,
        prompt: str,
        extend_post_id: str,
        video_extension_start_time: float,
        original_post_id: str = "",
        file_attachment_id: str = "",
        aspect_ratio: str = "16:9",
        video_length: int = 6,
        resolution: str = "480p",
        preset: str = "normal",
        stitch_with_extend: bool = True,
    ) -> AsyncGenerator[bytes, None]:
        """通过 Grok 官方视频延长 API 延长视频。"""
        token_tag = _token_tag(token)
        # 确定 mode
        prompt_text = (prompt or "").strip()
        is_custom = VideoService.is_meaningful_video_prompt(prompt_text)
        if is_custom:
            mode = "custom"
        else:
            mode = VideoService._map_preset_to_mode(preset)
            prompt_text = ""

        effective_original = (original_post_id or "").strip() or extend_post_id
        effective_file_attachment = (file_attachment_id or "").strip() or effective_original

        logger.info(
            "Video extension request: "
            f"token={token_tag}, extend_post_id={extend_post_id}, "
            f"start_time={video_extension_start_time}, original_post_id={effective_original}, "
            f"prompt='{(prompt_text or '')[:50]}', mode={mode}"
        )

        # 构造 message
        if prompt_text:
            message = f"{prompt_text} --mode={mode}"
        else:
            message = f"--mode={mode}"

        # 构造 videoGenModelConfig —— 对齐官网抓包格式
        video_gen_config = {
            "isVideoExtension": True,
            "videoExtensionStartTime": video_extension_start_time,
            "extendPostId": extend_post_id,
            "originalPostId": effective_original,
            "originalRefType": "ORIGINAL_REF_TYPE_VIDEO_EXTENSION",
            "mode": mode,
            "aspectRatio": aspect_ratio,
            "videoLength": video_length,
            "resolutionName": resolution,
            "parentPostId": extend_post_id,
            "isVideoEdit": False,
        }
        if stitch_with_extend:
            video_gen_config["stitchWithExtendPostId"] = extend_post_id
        if prompt_text:
            video_gen_config["originalPrompt"] = prompt_text

        model_config_override = {
            "modelMap": {
                "videoGenModelConfig": video_gen_config,
            }
        }

        # fileAttachments 对齐官网：始终传最初图转视频时的 parentPostId
        file_attachments = [effective_file_attachment]

        moderated_max_retry = max(1, int(get_config("video.moderated_max_retry", 5)))

        logger.info(
            "Video extension request prepared: "
            f"token={token_tag}, extend_post_id={extend_post_id}, "
            f"file_attachments={file_attachments}, "
            f"start_time={video_extension_start_time}, mode={mode}, "
            f"resolution={resolution}, video_length={video_length}, ratio={aspect_ratio}"
        )

        async def _stream():
            for attempt in range(1, moderated_max_retry + 1):
                session = AsyncSession()
                moderated_hit = False
                try:
                    async with _get_video_semaphore():
                        _log_final_video_payload(
                            message=message,
                            file_attachments=file_attachments,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                            mode=mode,
                        )
                        stream_response = await AppChatReverse.request(
                            session,
                            token,
                            message=message,
                            model="grok-3",
                            mode=mode,
                            file_attachments=file_attachments,
                            tool_overrides={"videoGen": True},
                            model_config_override=model_config_override,
                        )
                        logger.info(
                            "Video extension started: "
                            f"token={token_tag}, extend_post_id={extend_post_id}, "
                            f"attempt={attempt}/{moderated_max_retry}"
                        )
                        async for line in stream_response:
                            if self._is_moderated_line(line):
                                moderated_hit = True
                                logger.warning(
                                    f"Video extension moderated: token={token_tag}, "
                                    f"retry {attempt}/{moderated_max_retry}"
                                )
                                break
                            yield line

                    if not moderated_hit:
                        return
                    if attempt < moderated_max_retry:
                        await asyncio.sleep(1.2)
                        continue
                    raise UpstreamException(
                        "Video extension blocked by moderation",
                        status_code=400,
                        details={"moderated": True, "attempts": moderated_max_retry},
                    )
                except Exception as e:
                    logger.error(f"Video extension error: {e}")
                    if isinstance(e, AppException):
                        raise
                    msg, code, status = _classify_video_error(e)
                    raise AppException(
                        message=msg,
                        error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                        code=code,
                        status_code=status,
                    )
                finally:
                    try:
                        await session.close()
                    except Exception:
                        pass

        return _stream()

    @staticmethod
    async def completions(
        model: str,
        messages: list,
        stream: bool = None,
        reasoning_effort: str | None = None,
        aspect_ratio: str = "3:2",
        video_length: int = 6,
        resolution: str = "480p",
        preset: str = "normal",
        parent_post_id: str | None = None,
        extend_post_id: str | None = None,
        video_extension_start_time: float | None = None,
        original_post_id: str | None = None,
        file_attachment_id: str | None = None,
        stitch_with_extend: bool = True,
        source_image_url: str | None = None,
        reference_items: list[dict[str, Any]] | None = None,
        preferred_token: str | None = None,
        nsfw: bool | None = None,
        single_image_mode: str = "frame",
    ):
        """Video generation entrypoint."""
        token_mgr = await get_token_manager()
        await token_mgr.reload_if_stale()

        max_token_retries = int(
            get_config("video.extension_token_retry", 20)
            if extend_post_id
            else get_config("retry.max_retry")
        )
        last_error: Exception | None = None

        if reasoning_effort is None:
            show_think = get_config("app.thinking")
        else:
            show_think = reasoning_effort != "none"
        is_stream = stream if stream is not None else get_config("app.stream")

        from app.services.grok.services.chat import MessageExtractor
        from app.services.grok.utils.asset_token_map import AssetTokenMap

        prompt, _, image_attachments = MessageExtractor.extract(messages)
        parent_post_id = (parent_post_id or "").strip() or None
        source_image_url = (source_image_url or "").strip()
        reference_items = [item for item in (reference_items or []) if isinstance(item, dict)]
        preferred_token = (preferred_token or "").strip()

        token_map = await AssetTokenMap.get_instance()
        bound_token = None
        if extend_post_id:
            bound_token = await token_map.get_token(extend_post_id)
        elif parent_post_id:
            bound_token = await token_map.get_token(parent_post_id)
        if bound_token:
            preferred_token = bound_token
        has_parent_token_binding = bool(bound_token or preferred_token)

        if preferred_token.startswith("sso="):
            preferred_token = preferred_token[4:]
        if image_attachments and not reference_items:
            reference_items = [
                {
                    "parent_post_id": "",
                    "image_url": str(image_url or "").strip(),
                    "source_image_url": str(image_url or "").strip(),
                    "mention_alias": f"Image {index}",
                }
                for index, image_url in enumerate(image_attachments, start=1)
                if str(image_url or "").strip()
            ]
        used_tokens: set[str] = set()

        for attempt in range(max_token_retries):
            token = ""
            if preferred_token and preferred_token not in used_tokens:
                if token_mgr.get_pool_name_for_token(preferred_token):
                    token = preferred_token
                    logger.info(
                        f"Video token routing: preferred bound token -> token={_token_tag(token)}"
                    )
                else:
                    used_tokens.add(preferred_token)
                    logger.warning(
                        f"Video token routing: preferred bound token not in pool, fallback to normal routing "
                        f"(token={_token_tag(preferred_token)})"
                    )

            if not token:
                pool_candidates = ModelService.pool_candidates_for_model(model)
                preferred_tags = ["nsfw"] if (nsfw is None or nsfw) else None
                token_info = token_mgr.get_token_for_video(
                    resolution=resolution,
                    video_length=video_length,
                    pool_candidates=pool_candidates,
                    exclude=used_tokens,
                    preferred_tags=preferred_tags,
                )

                if not token_info:
                    if last_error:
                        raise last_error
                    raise AppException(
                        message="No available tokens. Please try again later.",
                        error_type=ErrorType.RATE_LIMIT.value,
                        code="rate_limit_exceeded",
                        status_code=429,
                    )

                token = token_info.token
                if token.startswith("sso="):
                    token = token[4:]

            used_tokens.add(token)
            pool_name = token_mgr.get_pool_name_for_token(token) or BASIC_POOL_NAME
            is_super_pool = pool_name != BASIC_POOL_NAME

            requested_resolution = resolution
            auto_upscale = bool(get_config("video.auto_upscale", True))
            should_upscale = auto_upscale and requested_resolution == "720p" and pool_name == BASIC_POOL_NAME
            generation_resolution = "480p" if should_upscale else requested_resolution
            upscale_timing = _resolve_upscale_timing() if should_upscale else "complete"

            try:
                image_url = None
                if (not parent_post_id) and image_attachments and not reference_items:
                    upload_service = UploadService()
                    try:
                        for attach_data in image_attachments:
                            _, file_uri = await upload_service.upload_file(attach_data, token)
                            image_url = f"https://assets.grok.com/{file_uri}"
                            logger.info(f"Image uploaded for video: {image_url}")
                            break
                    finally:
                        await upload_service.close()

                active_parent_post_id = parent_post_id
                active_source_image_url = source_image_url
                active_image_url = image_url
                if active_parent_post_id and (not has_parent_token_binding) and active_source_image_url:
                    upload_service = UploadService()
                    try:
                        _, file_uri = await upload_service.upload_file(active_source_image_url, token)
                        active_image_url = f"https://assets.grok.com/{file_uri}"
                        active_source_image_url = active_image_url
                        active_parent_post_id = None
                        logger.warning(
                            "Video parentPost fallback to uploaded source image: "
                            f"parent_post_id={parent_post_id}, token={_token_tag(token)}, image_url={active_image_url}"
                        )
                    finally:
                        await upload_service.close()

                service = VideoService()

                model_info = ModelService.get(model)
                effort = (
                    EffortType.HIGH
                    if (model_info and model_info.cost.value == "high")
                    else EffortType.LOW
                )

                if extend_post_id and video_extension_start_time is not None:
                    response = await service.generate_extend_video(
                        token=token,
                        prompt=prompt,
                        extend_post_id=extend_post_id,
                        video_extension_start_time=video_extension_start_time,
                        original_post_id=original_post_id or "",
                        file_attachment_id=file_attachment_id or "",
                        aspect_ratio=aspect_ratio,
                        video_length=video_length,
                        resolution=resolution,
                        preset=preset,
                        stitch_with_extend=stitch_with_extend,
                    )
                    if is_stream:
                        processor = VideoStreamProcessor(
                            model,
                            token,
                            show_think,
                            upscale_on_finish=should_upscale,
                        )
                        return wrap_stream_with_usage(
                            processor.process(response), token_mgr, token, model
                        )

                    result = await VideoCollectProcessor(
                        model, token, upscale_on_finish=should_upscale
                    ).process(response)
                    try:
                        await token_mgr.consume(token, effort)
                        logger.debug(
                            f"Video completed, recorded usage (effort={effort.value})"
                        )
                    except Exception as e:
                        logger.warning(f"Failed to record video usage: {e}")
                    return result

                if reference_items:
                    if len(reference_items) > 1:
                        response = await service.generate_from_reference_items(
                            token=token,
                            prompt=prompt,
                            reference_items=reference_items,
                            aspect_ratio=aspect_ratio,
                            video_length=video_length,
                            resolution=generation_resolution,
                            preset=preset,
                        )
                    elif effective_single_image_mode == "reference":
                        response = await service.generate_from_reference_items(
                            token=token,
                            prompt=prompt,
                            reference_items=reference_items,
                            aspect_ratio=aspect_ratio,
                            video_length=video_length,
                            resolution=resolution,
                            preset=preset,
                        )
                    else:
                        item = reference_items[0]
                        single_parent_post_id = str(item.get("parent_post_id") or "").strip()
                        single_source_image_url = str(
                            item.get("source_image_url") or item.get("image_url") or ""
                        ).strip()
                        if single_parent_post_id:
                            if not has_parent_token_binding:
                                fallback_image_url = active_image_url
                                if not fallback_image_url and single_source_image_url:
                                    upload_service = UploadService()
                                    try:
                                        _, file_uri = await upload_service.upload_file(
                                            single_source_image_url, token
                                        )
                                        fallback_image_url = _normalize_assets_url(file_uri)
                                    finally:
                                        await upload_service.close()
                                if fallback_image_url:
                                    logger.warning(
                                        "Video reference parentPost fallback to uploaded source image: "
                                        f"parent_post_id={single_parent_post_id}, token={_token_tag(token)}, "
                                        f"image_url={fallback_image_url}"
                                    )
                                    response = await service.generate_from_image(
                                        token=token,
                                        prompt=prompt,
                                        image_url=fallback_image_url,
                                        aspect_ratio=aspect_ratio,
                                        video_length=video_length,
                                        resolution=generation_resolution,
                                        preset=preset,
                                    )
                                else:
                                    response = await service.generate_from_parent_post(
                                        token=token,
                                        prompt=prompt,
                                        parent_post_id=single_parent_post_id,
                                        source_image_url=single_source_image_url,
                                        aspect_ratio=aspect_ratio,
                                        video_length=video_length,
                                        resolution=generation_resolution,
                                        preset=preset,
                                    )
                            else:
                                response = await service.generate_from_parent_post(
                                    token=token,
                                    prompt=prompt,
                                    parent_post_id=single_parent_post_id,
                                    source_image_url=single_source_image_url,
                                    aspect_ratio=aspect_ratio,
                                    video_length=video_length,
                                    resolution=generation_resolution,
                                    preset=preset,
                                )
                        else:
                            response = await service.generate_from_image(
                                token=token,
                                prompt=prompt,
                                image_url=single_source_image_url,
                                aspect_ratio=aspect_ratio,
                                video_length=video_length,
                                resolution=generation_resolution,
                                preset=preset,
                            )

                    if is_stream:
                        processor = VideoStreamProcessor(
                            model,
                            token,
                            show_think,
                            upscale_on_finish=should_upscale,
                        )
                        return wrap_stream_with_usage(
                            processor.process(response), token_mgr, token, model
                        )

                    result = await VideoCollectProcessor(
                        model, token, upscale_on_finish=should_upscale
                    ).process(response)
                    try:
                        await token_mgr.consume(token, effort)
                        logger.debug(
                            f"Video completed, recorded usage (effort={effort.value})"
                        )
                    except Exception as e:
                        logger.warning(f"Failed to record video usage: {e}")
                    return result

                target_length = int(video_length or 6)
                round_plan = _build_round_plan(target_length, is_super=is_super_pool)

                if len(round_plan) == 1 and active_parent_post_id:
                    response = await service.generate_from_parent_post(
                        token=token,
                        prompt=prompt,
                        parent_post_id=active_parent_post_id,
                        source_image_url=active_source_image_url or "",
                        aspect_ratio=aspect_ratio,
                        video_length=target_length,
                        resolution=generation_resolution,
                        preset=preset,
                    )
                    if is_stream:
                        processor = VideoStreamProcessor(
                            model,
                            token,
                            show_think,
                            upscale_on_finish=should_upscale,
                        )
                        return wrap_stream_with_usage(
                            processor.process(response), token_mgr, token, model
                        )

                    result = await VideoCollectProcessor(
                        model, token, upscale_on_finish=should_upscale
                    ).process(response)
                    try:
                        await token_mgr.consume(token, effort)
                        logger.debug(
                            f"Video completed, recorded usage (effort={effort.value})"
                        )
                    except Exception as e:
                        logger.warning(f"Failed to record video usage: {e}")
                    return result

                if len(round_plan) == 1 and active_image_url:
                    response = await service.generate_from_image(
                        token=token,
                        prompt=prompt,
                        image_url=active_image_url,
                        aspect_ratio=aspect_ratio,
                        video_length=target_length,
                        resolution=generation_resolution,
                        preset=preset,
                    )
                    if is_stream:
                        processor = VideoStreamProcessor(
                            model,
                            token,
                            show_think,
                            upscale_on_finish=should_upscale,
                        )
                        return wrap_stream_with_usage(
                            processor.process(response), token_mgr, token, model
                        )

                    result = await VideoCollectProcessor(
                        model, token, upscale_on_finish=should_upscale
                    ).process(response)
                    try:
                        await token_mgr.consume(token, effort)
                        logger.debug(
                            f"Video completed, recorded usage (effort={effort.value})"
                        )
                    except Exception as e:
                        logger.warning(f"Failed to record video usage: {e}")
                    return result

                message = VideoService._build_video_message(
                    prompt=prompt,
                    preset=preset,
                    source_image_url=active_source_image_url or active_image_url or "",
                )

                if active_parent_post_id:
                    seed_post_id = active_parent_post_id
                elif active_image_url:
                    seed_post_id = await service.create_image_post(token, active_image_url)
                else:
                    seed_post_id = await service.create_post(token, prompt)

                async def _save_round_mapping(post_id: Optional[str]):
                    if post_id and token:
                        await token_map.save_mapping(post_id, token)

                async def _request_first_round_stream(plan: VideoRoundPlan):
                    if plan.is_extension:
                        raise UpstreamException(
                            message="First round cannot be an extension round",
                            status_code=500,
                            details={"type": "invalid_round_plan", "round": plan.round_index},
                        )

                    if active_parent_post_id:
                        return await service.generate_from_parent_post(
                            token=token,
                            prompt=prompt,
                            parent_post_id=active_parent_post_id,
                            source_image_url=active_source_image_url or "",
                            aspect_ratio=aspect_ratio,
                            video_length=plan.video_length,
                            resolution=generation_resolution,
                            preset=preset,
                        )

                    if active_image_url:
                        return await service.generate_from_image(
                            token=token,
                            prompt=prompt,
                            image_url=active_image_url,
                            aspect_ratio=aspect_ratio,
                            video_length=plan.video_length,
                            resolution=generation_resolution,
                            preset=preset,
                        )

                    config_override = _build_round_config(
                        plan,
                        seed_post_id=seed_post_id,
                        last_post_id=seed_post_id,
                        original_post_id=None,
                        prompt=prompt,
                        aspect_ratio=aspect_ratio,
                        resolution_name=generation_resolution,
                        stitch_with_extend=stitch_with_extend,
                    )
                    return await _request_round_stream(
                        token=token,
                        message=message,
                        model_config_override=config_override,
                    )

                async def _run_round_collect(
                    plan: VideoRoundPlan,
                    *,
                    seed_id: str,
                    last_id: str,
                    original_id: Optional[str],
                    source: str,
                    deadline_monotonic: Optional[float],
                ) -> VideoRoundResult:
                    if plan.is_extension:
                        config_override = _build_round_config(
                            plan,
                            seed_post_id=seed_id,
                            last_post_id=last_id,
                            original_post_id=original_id,
                            prompt=prompt,
                            aspect_ratio=aspect_ratio,
                            resolution_name=generation_resolution,
                            stitch_with_extend=stitch_with_extend,
                        )
                        response = await _request_round_stream(
                            token=token,
                            message=message,
                            model_config_override=config_override,
                        )
                    else:
                        response = await _request_first_round_stream(plan)
                    return await _collect_round_result(
                        response,
                        model=model,
                        source=source,
                        total_timeout=_remaining_before_deadline(deadline_monotonic),
                    )

                async def _stream_chain() -> AsyncGenerator[str, None]:
                    writer = _VideoChainSSEWriter(model, show_think)
                    total_timeout = _resolve_video_total_timeout()
                    deadline_monotonic = (
                        time.monotonic() + total_timeout if total_timeout > 0 else None
                    )
                    seed_id = seed_post_id
                    last_id = seed_id
                    original_id: Optional[str] = None if active_parent_post_id else seed_id
                    final_result: Optional[VideoRoundResult] = None

                    try:
                        for plan in round_plan:
                            if plan.is_extension:
                                config_override = _build_round_config(
                                    plan,
                                    seed_post_id=seed_id,
                                    last_post_id=last_id,
                                    original_post_id=original_id,
                                    prompt=prompt,
                                    aspect_ratio=aspect_ratio,
                                    resolution_name=generation_resolution,
                                    stitch_with_extend=stitch_with_extend,
                                )
                                response = await _request_round_stream(
                                    token=token,
                                    message=message,
                                    model_config_override=config_override,
                                )
                            else:
                                response = await _request_first_round_stream(plan)

                            round_result = VideoRoundResult()
                            async for event_type, payload in _iter_round_events(
                                response,
                                model=model,
                                source=f"stream-round-{plan.round_index}",
                                total_timeout=_remaining_before_deadline(deadline_monotonic),
                            ):
                                if event_type == "progress":
                                    for chunk in writer.emit_progress(
                                        round_index=plan.round_index,
                                        total_rounds=plan.total_rounds,
                                        progress=payload,
                                    ):
                                        yield chunk
                                elif event_type == "done":
                                    round_result = payload

                            _ensure_round_result(
                                round_result,
                                round_index=plan.round_index,
                                total_rounds=plan.total_rounds,
                                final_round=(plan.round_index == plan.total_rounds),
                            )
                            await _save_round_mapping(round_result.post_id)

                            if should_upscale and upscale_timing == "single" and round_result.video_url:
                                for chunk in writer.emit_note(
                                    f"[round={plan.round_index}/{plan.total_rounds}] 正在对当前轮结果进行超分辨率\n"
                                ):
                                    yield chunk
                                upgraded_url, upscaled = await _upscale_video_url(token, round_result.video_url)
                                if upscaled:
                                    round_result.video_url = upgraded_url
                                else:
                                    logger.warning(
                                        "Video upscale failed in single mode, fallback to 480p result"
                                    )

                            if plan.round_index == 1 and round_result.post_id:
                                original_id = round_result.post_id
                            if round_result.post_id:
                                last_id = round_result.post_id

                            if plan.round_index == plan.total_rounds:
                                final_result = round_result

                        if final_result is None:
                            raise UpstreamException(
                                message="Video generation produced no final round",
                                status_code=502,
                                details={"type": "empty_video_stream"},
                            )

                        final_video_url = final_result.video_url
                        if should_upscale and upscale_timing == "complete":
                            for chunk in writer.emit_note("正在对视频进行超分辨率\n"):
                                yield chunk
                            final_video_url, upscaled = await _upscale_video_url(token, final_video_url)
                            if not upscaled:
                                logger.warning("Video upscale failed, fallback to 480p result")

                        dl_service = DownloadService()
                        try:
                            rendered = await dl_service.render_video(
                                final_video_url,
                                token,
                                final_result.thumbnail_url,
                            )
                        finally:
                            await dl_service.close()

                        for chunk in writer.emit_content(rendered):
                            yield chunk
                        for chunk in writer.finish():
                            yield chunk
                    except asyncio.CancelledError:
                        logger.debug(
                            "Video stream chain cancelled by client", extra={"model": model}
                        )
                        raise
                    except UpstreamException as e:
                        if rate_limited(e):
                            await token_mgr.mark_rate_limited(token)
                        raise

                async def _collect_chain() -> Dict[str, Any]:
                    total_timeout = _resolve_video_total_timeout()
                    deadline_monotonic = (
                        time.monotonic() + total_timeout if total_timeout > 0 else None
                    )
                    seed_id = seed_post_id
                    last_id = seed_id
                    original_id: Optional[str] = None if active_parent_post_id else seed_id
                    final_result: Optional[VideoRoundResult] = None

                    for plan in round_plan:
                        round_result = await _run_round_collect(
                            plan,
                            seed_id=seed_id,
                            last_id=last_id,
                            original_id=original_id,
                            source=f"collect-round-{plan.round_index}",
                            deadline_monotonic=deadline_monotonic,
                        )

                        _ensure_round_result(
                            round_result,
                            round_index=plan.round_index,
                            total_rounds=plan.total_rounds,
                            final_round=(plan.round_index == plan.total_rounds),
                        )
                        await _save_round_mapping(round_result.post_id)

                        if should_upscale and upscale_timing == "single" and round_result.video_url:
                            upgraded_url, upscaled = await _upscale_video_url(token, round_result.video_url)
                            if upscaled:
                                round_result.video_url = upgraded_url
                            else:
                                logger.warning(
                                    "Video upscale failed in single mode, fallback to 480p result"
                                )

                        if plan.round_index == 1 and round_result.post_id:
                            original_id = round_result.post_id
                        if round_result.post_id:
                            last_id = round_result.post_id

                        if plan.round_index == plan.total_rounds:
                            final_result = round_result

                    if final_result is None:
                        raise UpstreamException(
                            message="Video generation produced no final round",
                            status_code=502,
                            details={"type": "empty_video_stream"},
                        )

                    final_video_url = final_result.video_url
                    if should_upscale and upscale_timing == "complete":
                        final_video_url, upscaled = await _upscale_video_url(token, final_video_url)
                        if not upscaled:
                            logger.warning("Video upscale failed, fallback to 480p result")

                    dl_service = DownloadService()
                    try:
                        content = await dl_service.render_video(
                            final_video_url,
                            token,
                            final_result.thumbnail_url,
                        )
                    finally:
                        await dl_service.close()

                    return {
                        "id": final_result.response_id,
                        "object": "chat.completion",
                        "created": int(time.time()),
                        "model": model,
                        "choices": [
                            {
                                "index": 0,
                                "message": {
                                    "role": "assistant",
                                    "content": content,
                                    "refusal": None,
                                },
                                "finish_reason": "stop",
                            }
                        ],
                        "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
                    }

                if is_stream:
                    return wrap_stream_with_usage(_stream_chain(), token_mgr, token, model)

                result = await _collect_chain()
                try:
                    await token_mgr.consume(token, effort)
                    logger.debug(
                        f"Video completed, recorded usage (effort={effort.value})"
                    )
                except Exception as e:
                    logger.warning(f"Failed to record video usage: {e}")
                return result

            except UpstreamException as e:
                last_error = e
                if rate_limited(e):
                    await token_mgr.mark_rate_limited(token)
                    logger.warning(
                        f"Token {_token_tag(token)} rate limited (429), trying next token (attempt {attempt + 1}/{max_token_retries})"
                    )
                    continue
                if _is_video_auth_error(e):
                    try:
                        await token_mgr.record_fail(token, 401, "video_auth_failed")
                    except Exception:
                        pass
                    logger.warning(
                        f"Video token {_token_tag(token)} auth failed, trying next token (attempt {attempt + 1}/{max_token_retries})"
                    )
                    continue
                msg, code, status = _classify_video_error(e)
                raise AppException(
                    message=msg,
                    error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                    code=code,
                    status_code=status,
                )

        if last_error:
            if extend_post_id and _is_video_auth_error(last_error):
                raise AppException(
                    message="视频延长失败：当前 token 池中没有可访问该视频的账号，请使用通过当前服务生成的视频 post_id，或先绑定原视频账号的 token。",
                    error_type=ErrorType.SERVER.value,
                    code="video_extension_token_unbound",
                    status_code=502,
                )
            raise last_error
        raise AppException(
            message="No available tokens. Please try again later.",
            error_type=ErrorType.RATE_LIMIT.value,
            code="rate_limit_exceeded",
            status_code=429,
        )


class VideoStreamProcessor(BaseProcessor):
    """Video stream response processor."""

    def __init__(
        self,
        model: str,
        token: str = "",
        show_think: bool = None,
        upscale_on_finish: bool = False,
    ):
        super().__init__(model, token)
        self.response_id: Optional[str] = None
        self.think_opened: bool = False
        self.role_sent: bool = False

        self.show_think = bool(show_think)
        self.upscale_on_finish = bool(upscale_on_finish)

    @staticmethod
    def _extract_video_id(video_url: str) -> str:
        if not video_url:
            return ""
        match = re.search(r"/generated/([0-9a-fA-F-]{32,36})/", video_url)
        if match:
            return match.group(1)
        match = re.search(r"/([0-9a-fA-F-]{32,36})/generated_video", video_url)
        if match:
            return match.group(1)
        return ""

    async def _upscale_video_url(self, video_url: str) -> str:
        if not video_url or not self.upscale_on_finish:
            return video_url
        video_id = self._extract_video_id(video_url)
        if not video_id:
            logger.warning("Video upscale skipped: unable to extract video id")
            return video_url
        try:
            async with AsyncSession() as session:
                response = await VideoUpscaleReverse.request(
                    session, self.token, video_id
                )
            payload = response.json() if response is not None else {}
            hd_url = payload.get("hdMediaUrl") if isinstance(payload, dict) else None
            if hd_url:
                logger.info(f"Video upscale completed: {hd_url}")
                return hd_url
        except Exception as e:
            logger.warning(f"Video upscale failed: {e}")
        return video_url

    def _sse(self, content: str = "", role: str = None, finish: str = None) -> str:
        """Build SSE response."""
        delta = {}
        if role:
            delta["role"] = role
            delta["content"] = ""
        elif content:
            delta["content"] = content

        chunk = {
            "id": self.response_id or f"chatcmpl-{uuid.uuid4().hex[:24]}",
            "object": "chat.completion.chunk",
            "created": self.created,
            "model": self.model,
            "choices": [
                {"index": 0, "delta": delta, "logprobs": None, "finish_reason": finish}
            ],
        }
        return f"data: {orjson.dumps(chunk).decode()}\n\n"

    async def process(
        self, response: AsyncIterable[bytes]
    ) -> AsyncGenerator[str, None]:
        """Process video stream response."""
        idle_timeout = float(get_config("video.stream_timeout") or 0)
        total_timeout = _resolve_video_total_timeout()
        started_at = time.monotonic()
        fallback_video_id = ""
        fallback_thumb = ""
        content_emitted = False
        last_progress = None
        stream_errors: List[Any] = []
        iterator = response.__aiter__()

        try:
            while True:
                timeout_seconds = idle_timeout
                timeout_reason = "idle"
                if total_timeout > 0:
                    remaining_total = total_timeout - (time.monotonic() - started_at)
                    if remaining_total <= 0:
                        raise _build_video_total_timeout_exception(
                            timeout_seconds=total_timeout,
                            source="video_stream_processor",
                            result=VideoRoundResult(
                                response_id=self.response_id or "",
                                last_progress=last_progress,
                                stream_errors=list(stream_errors),
                            ),
                        )
                    if timeout_seconds <= 0 or remaining_total < timeout_seconds:
                        timeout_seconds = remaining_total
                        timeout_reason = "total"

                try:
                    raw_line = await _next_with_enforced_timeout(
                        iterator,
                        timeout_seconds=timeout_seconds,
                    )
                except asyncio.TimeoutError:
                    if timeout_reason == "total":
                        raise _build_video_total_timeout_exception(
                            timeout_seconds=total_timeout,
                            source="video_stream_processor",
                            result=VideoRoundResult(
                                response_id=self.response_id or "",
                                last_progress=last_progress,
                                stream_errors=list(stream_errors),
                            ),
                        )
                    raise StreamIdleTimeoutError(timeout_seconds)
                except StopAsyncIteration:
                    break

                line = _normalize_line(raw_line)
                if not line:
                    continue
                try:
                    data = orjson.loads(line)
                except orjson.JSONDecodeError:
                    continue

                resp = data.get("result", {}).get("response", {})
                is_thinking = bool(resp.get("isThinking"))

                if rid := resp.get("responseId"):
                    self.response_id = rid

                _append_unique_errors(stream_errors, resp.get("streamErrors"))

                if not self.role_sent:
                    yield self._sse(role="assistant")
                    self.role_sent = True

                if token := resp.get("token"):
                    if is_thinking:
                        if not self.show_think:
                            continue
                        if not self.think_opened:
                            yield self._sse("<think>\n")
                            self.think_opened = True
                    else:
                        if self.think_opened:
                            yield self._sse("\n</think>\n")
                            self.think_opened = False
                    yield self._sse(token)
                    continue

                if video_resp := resp.get("streamingVideoGenerationResponse"):
                    fallback_video_id = (
                        str(video_resp.get("videoPostId", "")).strip()
                        or str(video_resp.get("assetId", "")).strip()
                        or str(video_resp.get("videoId", "")).strip()
                        or fallback_video_id
                    )
                    thumb_from_stream = str(video_resp.get("thumbnailImageUrl", "")).strip()
                    if thumb_from_stream:
                        fallback_thumb = thumb_from_stream
                    progress = video_resp.get("progress", 0)
                    last_progress = progress

                    if is_thinking:
                        if not self.show_think:
                            continue
                        if not self.think_opened:
                            yield self._sse("<think>\n")
                            self.think_opened = True
                    else:
                        if self.think_opened:
                            yield self._sse("\n</think>\n")
                            self.think_opened = False
                    if self.show_think:
                        yield self._sse(f"正在生成视频中，当前进度{progress}%\n")

                    if progress == 100:
                        video_url = video_resp.get("videoUrl", "")
                        thumbnail_url = video_resp.get("thumbnailImageUrl", "")

                        video_post_id = fallback_video_id or self._extract_video_id(video_url)
                        if video_post_id and self.token:
                            from app.services.grok.utils.asset_token_map import AssetTokenMap
                            token_map = await AssetTokenMap.get_instance()
                            await token_map.save_mapping(video_post_id, self.token)

                        if self.think_opened:
                            yield self._sse("\n</think>\n")
                            self.think_opened = False

                        if not video_url and video_post_id:
                            asset_video_path, asset_thumb_path = await _resolve_video_asset_path(
                                self.token, video_post_id
                            )
                            if asset_video_path:
                                video_url = asset_video_path
                                thumbnail_url = asset_thumb_path or thumbnail_url or fallback_thumb

                        if video_url:
                            if self.upscale_on_finish:
                                yield self._sse("正在对视频进行超分辨率\n")
                                video_url = await self._upscale_video_url(video_url)
                            dl_service = self._get_dl()
                            rendered = await dl_service.render_video(
                                video_url, self.token, thumbnail_url or fallback_thumb
                            )
                            yield self._sse(rendered)
                            content_emitted = True

                            logger.info(f"Video generated: {video_url} (post_id={video_post_id})")
                    continue

                elif model_resp := resp.get("modelResponse"):
                    _append_unique_errors(stream_errors, model_resp.get("streamErrors"))
                    file_attachments = model_resp.get("fileAttachments", [])
                    if isinstance(file_attachments, list):
                        for fid in file_attachments:
                            fid = str(fid).strip()
                            if fid:
                                fallback_video_id = fid
                                break
                    continue

            if self.think_opened:
                yield self._sse("</think>\n")
                self.think_opened = False

            if not content_emitted and fallback_video_id:
                asset_video_path, asset_thumb_path = await _resolve_video_asset_path(
                    self.token, fallback_video_id
                )
                if asset_video_path:
                    if self.upscale_on_finish:
                        yield self._sse("正在对视频进行超分辨率\n")
                        asset_video_path = await self._upscale_video_url(asset_video_path)
                    dl_service = self._get_dl()
                    rendered = await dl_service.render_video(
                        asset_video_path, self.token, asset_thumb_path or fallback_thumb
                    )
                    yield self._sse(rendered)
                    content_emitted = True
                    logger.info(
                        "Video generated via stream assets fallback: "
                        f"video_id={fallback_video_id}, key={asset_video_path}"
                    )
                else:
                    logger.warning(
                        "Video stream finished without video url and assets fallback missed: "
                        f"video_id={fallback_video_id}"
                    )

            if not content_emitted:
                raise UpstreamException(
                    message="Video stream finished without any playable result",
                    status_code=502,
                    details={
                        "type": "empty_video_stream",
                        "response_id": self.response_id or "",
                        "last_progress": last_progress,
                        "stream_errors": list(stream_errors),
                    },
                )

            yield self._sse(finish="stop")
            yield "data: [DONE]\n\n"
        except asyncio.CancelledError:
            logger.debug(
                "Video stream cancelled by client", extra={"model": self.model}
            )
        except StreamIdleTimeoutError as e:
            raise AppException(
                message="视频生成失败：网络连接异常，请稍后重试",
                error_type=ErrorType.SERVER.value,
                code="video_network_error",
                status_code=504,
            )
        except RequestsError as e:
            if _is_http2_error(e):
                logger.warning(
                    f"HTTP/2 stream error in video: {e}", extra={"model": self.model}
                )
                raise AppException(
                    message="视频生成失败：网络连接异常，请稍后重试",
                    error_type=ErrorType.SERVER.value,
                    code="video_network_error",
                    status_code=502,
                )
            logger.error(
                f"Video stream request error: {e}", extra={"model": self.model}
            )
            raise AppException(
                message="视频生成失败：网络连接异常，请稍后重试",
                error_type=ErrorType.SERVER.value,
                code="video_network_error",
                status_code=502,
            )
        except Exception as e:
            logger.error(
                f"Video stream processing error: {e}",
                extra={"model": self.model, "error_type": type(e).__name__},
            )
            msg, code, status = _classify_video_error(e)
            raise AppException(
                message=msg,
                error_type=ErrorType.SERVER.value if status >= 500 else ErrorType.INVALID_REQUEST.value,
                code=code,
                status_code=status,
            )
        finally:
            await _best_effort_close_async_iterable(iterator)
            await self.close()


class VideoCollectProcessor(BaseProcessor):
    """Video non-stream response processor."""

    def __init__(self, model: str, token: str = "", upscale_on_finish: bool = False):
        super().__init__(model, token)
        self.upscale_on_finish = bool(upscale_on_finish)

    @staticmethod
    def _extract_video_id(video_url: str) -> str:
        if not video_url:
            return ""
        match = re.search(r"/generated/([0-9a-fA-F-]{32,36})/", video_url)
        if match:
            return match.group(1)
        match = re.search(r"/([0-9a-fA-F-]{32,36})/generated_video", video_url)
        if match:
            return match.group(1)
        return ""

    async def _upscale_video_url(self, video_url: str) -> str:
        if not video_url or not self.upscale_on_finish:
            return video_url
        video_id = self._extract_video_id(video_url)
        if not video_id:
            logger.warning("Video upscale skipped: unable to extract video id")
            return video_url
        try:
            async with AsyncSession() as session:
                response = await VideoUpscaleReverse.request(
                    session, self.token, video_id
                )
            payload = response.json() if response is not None else {}
            hd_url = payload.get("hdMediaUrl") if isinstance(payload, dict) else None
            if hd_url:
                logger.info(f"Video upscale completed: {hd_url}")
                return hd_url
        except Exception as e:
            logger.warning(f"Video upscale failed: {e}")
        return video_url

    async def _resolve_video_asset_path(self, asset_id: str) -> tuple[str, str]:
        return await _resolve_video_asset_path(self.token, asset_id)

    async def process(self, response: AsyncIterable[bytes]) -> dict[str, Any]:
        """Process and collect video response."""
        response_id = ""
        content = ""
        fallback_video_id = ""
        fallback_thumb = ""
        idle_timeout = get_config("video.stream_timeout")

        try:
            async for line in _with_idle_timeout(response, idle_timeout, self.model):
                line = _normalize_line(line)
                if not line:
                    continue
                try:
                    data = orjson.loads(line)
                except orjson.JSONDecodeError:
                    continue

                resp = data.get("result", {}).get("response", {})

                if video_resp := resp.get("streamingVideoGenerationResponse"):
                    fallback_video_id = (
                        str(video_resp.get("videoPostId", "")).strip()
                        or str(video_resp.get("assetId", "")).strip()
                        or str(video_resp.get("videoId", "")).strip()
                        or fallback_video_id
                    )
                    thumb_from_stream = str(
                        video_resp.get("thumbnailImageUrl", "")
                    ).strip()
                    if thumb_from_stream:
                        fallback_thumb = thumb_from_stream

                    if video_resp.get("progress") == 100:
                        response_id = resp.get("responseId", "")
                        video_url = video_resp.get("videoUrl", "")
                        thumbnail_url = video_resp.get("thumbnailImageUrl", "")
                        
                        # [NEW] 记录生成的视频对应的 postId 与 token 以备延长
                        if fallback_video_id and self.token:
                            from app.services.grok.utils.asset_token_map import AssetTokenMap
                            token_map = await AssetTokenMap.get_instance()
                            await token_map.save_mapping(fallback_video_id, self.token)

                        if video_url:
                            if self.upscale_on_finish:
                                video_url = await self._upscale_video_url(video_url)
                            dl_service = self._get_dl()
                            content = await dl_service.render_video(
                                video_url, self.token, thumbnail_url
                            )
                            self.video_post_id = fallback_video_id
                            logger.info(f"Video generated: {video_url} (post_id={fallback_video_id})")
                elif model_resp := resp.get("modelResponse"):
                    file_attachments = model_resp.get("fileAttachments", [])
                    if isinstance(file_attachments, list):
                        for fid in file_attachments:
                            fid = str(fid).strip()
                            if fid:
                                fallback_video_id = fid
                                break

        except asyncio.CancelledError:
            logger.debug(
                "Video collect cancelled by client", extra={"model": self.model}
            )
        except StreamIdleTimeoutError as e:
            logger.warning(
                f"Video collect idle timeout: {e}", extra={"model": self.model}
            )
        except RequestsError as e:
            if _is_http2_error(e):
                logger.warning(
                    f"HTTP/2 stream error in video collect: {e}",
                    extra={"model": self.model},
                )
            else:
                logger.error(
                    f"Video collect request error: {e}", extra={"model": self.model}
                )
        except UpstreamException as e:
            # 对于上游明确返回的业务终止错误（如 moderation 封禁），
            # 不应吞掉并伪装成“空结果”，否则上层会误判为 parentPost 空结果继续下一轮。
            details = getattr(e, "details", {}) or {}
            is_moderated_block = bool(details.get("moderated")) or (
                "blocked by moderation" in str(e).lower()
            )
            if is_moderated_block:
                logger.error(
                    f"Video collect got terminal moderation error: {e}",
                    extra={"model": self.model},
                )
                raise
            logger.error(
                f"Video collect upstream error: {e}",
                extra={"model": self.model, "error_type": type(e).__name__},
            )
        except Exception as e:
            logger.error(
                f"Video collect processing error: {e}",
                extra={"model": self.model, "error_type": type(e).__name__},
            )
        finally:
            await self.close()

        # [NEW] 提取并包含 post_id
        post_id = getattr(self, "video_post_id", fallback_video_id)

        if not content and fallback_video_id:
            asset_video_path, asset_thumb_path = await self._resolve_video_asset_path(
                fallback_video_id
            )
            if asset_video_path:
                if self.upscale_on_finish:
                    asset_video_path = await self._upscale_video_url(asset_video_path)
                dl_service = self._get_dl()
                content = await dl_service.render_video(
                    asset_video_path, self.token, asset_thumb_path or fallback_thumb
                )
                response_id = response_id or f"chatcmpl-{uuid.uuid4().hex[:24]}"
                logger.info(
                    "Video generated via assets fallback: "
                    f"video_id={fallback_video_id}, key={asset_video_path}"
                )

        return {
            "id": response_id,
            "object": "chat.completion",
            "created": self.created,
            "model": self.model,
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": content,
                        "refusal": None,
                    },
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
        }


__all__ = ["VideoService"]
