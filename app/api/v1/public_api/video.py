import asyncio
import re
import time
import uuid
from typing import Optional, List, Dict, Any

import orjson
from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from app.core.auth import verify_public_key
from app.core.logger import logger
from app.core.exceptions import AppException
from app.services.grok.services.video import VideoService
from app.services.grok.services.model import ModelService
from app.services.grok.utils.cache import CacheService

router = APIRouter()

VIDEO_SESSION_TTL = 600
_VIDEO_SESSIONS: dict[str, dict] = {}
_VIDEO_SESSIONS_LOCK = asyncio.Lock()

_VIDEO_RATIO_MAP = {
    "1280x720": "16:9",
    "720x1280": "9:16",
    "1792x1024": "3:2",
    "1024x1792": "2:3",
    "1024x1024": "1:1",
    "16:9": "16:9",
    "9:16": "9:16",
    "3:2": "3:2",
    "2:3": "2:3",
    "1:1": "1:1",
}


def _public_video_error_payload(exc: Exception) -> dict:
    """统一 public video 错误文案，避免透传工具层异常。"""
    if isinstance(exc, AppException):
        return {"error": exc.message, "code": exc.code or "video_failed"}

    text = str(exc or "").lower()
    if (
        "blocked by moderation" in text
        or "content moderated" in text
        or "content-moderated" in text
        or '"code":3' in text
        or "'code': 3" in text
    ):
        return {"error": "视频生成被拒绝，请调整提示词或素材后重试", "code": "video_rejected"}
    if (
        "tls connect error" in text
        or "timed out" in text
        or "timeout" in text
        or "connection closed" in text
        or "http/2" in text
        or "curl: (35)" in text
        or "network" in text
        or "proxy" in text
    ):
        return {"error": "视频生成失败：网络连接异常，请稍后重试", "code": "video_network_error"}
    return {"error": "视频生成失败，请稍后重试", "code": "video_failed"}


def _extract_parent_post_id_from_url(url: str) -> str:
    text = str(url or "").strip()
    if not text:
        return ""
    if re.fullmatch(r"[0-9a-fA-F-]{32,36}", text):
        return text
    for pattern in (
        r"/generated/([0-9a-fA-F-]{32,36})(?:/|$)",
        r"/imagine-public/images/([0-9a-fA-F-]{32,36})(?:\.jpg|/|$)",
        r"/images/([0-9a-fA-F-]{32,36})(?:\.jpg|/|$)",
    ):
        match = re.search(pattern, text)
        if match:
            return match.group(1)
    matches = re.findall(r"([0-9a-fA-F-]{32,36})", text)
    return matches[-1] if matches else ""

async def _clean_sessions(now: float) -> None:
    expired = [
        key
        for key, info in _VIDEO_SESSIONS.items()
        if now - float(info.get("created_at") or 0) > VIDEO_SESSION_TTL
    ]
    for key in expired:
        _VIDEO_SESSIONS.pop(key, None)


async def _new_session(
    prompt: str,
    aspect_ratio: str,
    video_length: int,
    resolution_name: str,
    preset: str,
    image_url: Optional[str],
    parent_post_id: Optional[str],
    source_image_url: Optional[str],
    reference_items: Optional[List[Dict[str, str]]],
    reasoning_effort: Optional[str],
    single_image_mode: str = "frame",
    # 视频延长相关
    is_video_extension: bool = False,
    extend_post_id: Optional[str] = None,
    video_extension_start_time: Optional[float] = None,
    original_post_id: Optional[str] = None,
    file_attachment_id: Optional[str] = None,
    stitch_with_extend: bool = True,
) -> str:
    task_id = uuid.uuid4().hex
    now = time.time()
    async with _VIDEO_SESSIONS_LOCK:
        await _clean_sessions(now)
        _VIDEO_SESSIONS[task_id] = {
            "prompt": prompt,
            "aspect_ratio": aspect_ratio,
            "video_length": video_length,
            "resolution_name": resolution_name,
            "preset": preset,
            "image_url": image_url,
            "parent_post_id": parent_post_id,
            "source_image_url": source_image_url,
            "reference_items": reference_items or [],
            "reasoning_effort": reasoning_effort,
            "single_image_mode": single_image_mode,
            "is_video_extension": is_video_extension,
            "extend_post_id": extend_post_id,
            "video_extension_start_time": video_extension_start_time,
            "original_post_id": original_post_id,
            "file_attachment_id": file_attachment_id,
            "stitch_with_extend": stitch_with_extend,
            "created_at": now,
        }
    return task_id


async def _get_session(task_id: str) -> Optional[dict]:
    if not task_id:
        return None
    now = time.time()
    async with _VIDEO_SESSIONS_LOCK:
        await _clean_sessions(now)
        info = _VIDEO_SESSIONS.get(task_id)
        if not info:
            return None
        created_at = float(info.get("created_at") or 0)
        if now - created_at > VIDEO_SESSION_TTL:
            _VIDEO_SESSIONS.pop(task_id, None)
            return None
        return dict(info)


async def _drop_session(task_id: str) -> None:
    if not task_id:
        return
    async with _VIDEO_SESSIONS_LOCK:
        _VIDEO_SESSIONS.pop(task_id, None)


async def _drop_sessions(task_ids: List[str]) -> int:
    if not task_ids:
        return 0
    removed = 0
    async with _VIDEO_SESSIONS_LOCK:
        for task_id in task_ids:
            if task_id and task_id in _VIDEO_SESSIONS:
                _VIDEO_SESSIONS.pop(task_id, None)
                removed += 1
    return removed


def _normalize_ratio(value: Optional[str]) -> str:
    raw = (value or "").strip()
    return _VIDEO_RATIO_MAP.get(raw, "")


def _validate_image_url(image_url: str) -> None:
    value = (image_url or "").strip()
    if not value:
        return
    if value.startswith("data:"):
        return
    if value.startswith("http://") or value.startswith("https://"):
        return
    raise HTTPException(
        status_code=400,
        detail="image_url must be a URL or data URI (data:<mime>;base64,...)",
    )


def _validate_parent_post_id(parent_post_id: str) -> str:
    value = (parent_post_id or "").strip()
    if not value:
        return ""
    if not re.fullmatch(r"[0-9a-fA-F-]{32,36}", value):
        raise HTTPException(status_code=400, detail="parent_post_id format is invalid")
    return value


def _normalize_string_list(values: Optional[List[str]]) -> List[str]:
    if not values:
        return []
    normalized: List[str] = []
    for value in values:
        text = str(value or "").strip()
        if text:
            normalized.append(text)
    return normalized


def _build_reference_items(data: "VideoStartRequest") -> List[Dict[str, str]]:
    items: List[Dict[str, str]] = []

    for raw in data.reference_items or []:
        if not isinstance(raw, dict):
            continue
        parent_post_id = _validate_parent_post_id(str(raw.get("parent_post_id") or ""))
        image_url = str(raw.get("image_url") or "").strip()
        source_image_url = str(raw.get("source_image_url") or "").strip()
        mention_alias = str(raw.get("mention_alias") or "").strip()
        if image_url:
            _validate_image_url(image_url)
        if source_image_url:
            _validate_image_url(source_image_url)
        if parent_post_id or image_url or source_image_url:
            items.append(
                {
                    "parent_post_id": parent_post_id,
                    "image_url": image_url,
                    "source_image_url": source_image_url,
                    "mention_alias": mention_alias,
                }
            )

    for value in _normalize_string_list(data.image_references):
        _validate_image_url(value)
        items.append({"parent_post_id": "", "image_url": value, "source_image_url": value})

    for value in _normalize_string_list(data.source_image_urls):
        _validate_image_url(value)
        items.append({"parent_post_id": "", "image_url": value, "source_image_url": value})

    for value in _normalize_string_list(data.parent_post_ids):
        items.append({"parent_post_id": _validate_parent_post_id(value), "image_url": "", "source_image_url": ""})

    single_parent = _validate_parent_post_id(data.parent_post_id or "")
    single_image_url = (data.image_url or "").strip()
    single_source_image_url = (data.source_image_url or "").strip()
    if single_image_url:
        _validate_image_url(single_image_url)
    if single_source_image_url:
        _validate_image_url(single_source_image_url)
    if single_parent or single_image_url or single_source_image_url:
        items.insert(
            0,
            {
                "parent_post_id": single_parent,
                "image_url": single_image_url,
                "source_image_url": single_source_image_url,
            },
        )

    deduped: List[Dict[str, str]] = []
    seen: set[tuple[str, str, str]] = set()
    for item in items:
        key = (
            str(item.get("parent_post_id") or "").strip(),
            str(item.get("image_url") or "").strip(),
            str(item.get("source_image_url") or "").strip(),
        )
        if key in seen:
            continue
        seen.add(key)
        deduped.append(
            {
                "parent_post_id": key[0],
                "image_url": key[1],
                "source_image_url": key[2],
                "mention_alias": str(item.get("mention_alias") or "").strip(),
            }
        )
    return deduped


class VideoStartRequest(BaseModel):
    prompt: Optional[str] = ""
    aspect_ratio: Optional[str] = "3:2"
    video_length: Optional[int] = 6
    resolution_name: Optional[str] = "480p"
    preset: Optional[str] = "normal"
    single_image_mode: Optional[str] = "frame"
    concurrent: Optional[int] = Field(1, ge=1, le=4)
    image_url: Optional[str] = None
    parent_post_id: Optional[str] = None
    source_image_url: Optional[str] = None
    image_references: Optional[List[str]] = None
    parent_post_ids: Optional[List[str]] = None
    source_image_urls: Optional[List[str]] = None
    reference_items: Optional[List[Dict[str, Any]]] = None
    reasoning_effort: Optional[str] = None
    edit_context: Optional[Dict[str, Any]] = None
    # 视频延长相关字段
    is_video_extension: Optional[bool] = False
    extend_post_id: Optional[str] = None
    video_extension_start_time: Optional[float] = None
    original_post_id: Optional[str] = None
    file_attachment_id: Optional[str] = None
    stitch_with_extend: Optional[bool] = True


class VideoRenameRequest(BaseModel):
    post_id: Optional[str] = None
    share_link: Optional[str] = None
    name: Optional[str] = None
    display_name: Optional[str] = ""


@router.post("/video/start", dependencies=[Depends(verify_public_key)])
async def public_video_start(data: VideoStartRequest):
    prompt = (data.prompt or "").strip()

    aspect_ratio = _normalize_ratio(data.aspect_ratio)
    if not aspect_ratio:
        raise HTTPException(
            status_code=400,
            detail="aspect_ratio must be one of ['16:9','9:16','3:2','2:3','1:1']",
        )

    video_length = int(data.video_length or 6)
    if video_length not in (6, 10, 15):
        raise HTTPException(
            status_code=400, detail="video_length must be 6, 10, or 15 seconds"
        )

    resolution_name = str(data.resolution_name or "480p")
    if resolution_name not in ("480p", "720p"):
        raise HTTPException(
            status_code=400,
            detail="resolution_name must be one of ['480p','720p']",
        )

    preset = str(data.preset or "normal")
    if preset not in ("fun", "normal", "spicy", "custom"):
        raise HTTPException(
            status_code=400,
            detail="preset must be one of ['fun','normal','spicy','custom']",
        )
    concurrent = int(data.concurrent or 1)
    if concurrent < 1 or concurrent > 4:
        raise HTTPException(status_code=400, detail="concurrent must be between 1 and 4")

    single_image_mode = str(data.single_image_mode or "frame").strip().lower()
    if single_image_mode not in ("frame", "reference"):
        raise HTTPException(status_code=400, detail="single_image_mode must be one of ['frame','reference']")

    reference_items = _build_reference_items(data)
    if len(reference_items) > 7:
        raise HTTPException(status_code=400, detail="最多支持 7 张参考图")
    parent_post_refs = [item for item in reference_items if item.get("parent_post_id")]
    parent_post_id = str(parent_post_refs[0].get("parent_post_id") or "").strip() if parent_post_refs else ""
    # 带 parent_post_id 的参考项按“基于已有 post 引用”处理，不再同时抽取 image_url，
    # 避免单个 reference_item 既有 parent_post_id 又有 image_url 时被误判为冲突参数。
    pure_image_refs = [item for item in reference_items if not item.get("parent_post_id")]
    image_url = (
        str(pure_image_refs[0].get("image_url") or "").strip() or None
        if pure_image_refs
        else None
    )
    source_image_url = (
        str(pure_image_refs[0].get("source_image_url") or "").strip() or None
        if pure_image_refs
        else None
    )

    # 视频延长参数解析
    is_video_extension = bool(data.is_video_extension)
    extend_post_id = _validate_parent_post_id(data.extend_post_id or "")
    video_extension_start_time = data.video_extension_start_time
    original_post_id = _validate_parent_post_id(data.original_post_id or "")
    file_attachment_id = _validate_parent_post_id(data.file_attachment_id or "")
    stitch_with_extend = bool(data.stitch_with_extend if data.stitch_with_extend is not None else True)

    if is_video_extension:
        # 视频延长模式校验
        if not extend_post_id:
            raise HTTPException(
                status_code=400,
                detail="extend_post_id is required for video extension",
            )
        if video_extension_start_time is None or video_extension_start_time <= 0:
            video_extension_start_time = 6.0

        logger.info(
            "Public video extension request: "
            f"extend_post_id={extend_post_id}, "
            f"start_time={video_extension_start_time}, "
            f"original_post_id={original_post_id}, "
            f"file_attachment_id={file_attachment_id}, "
            f"concurrent={concurrent}"
        )
    else:
        if parent_post_id and image_url and len(reference_items) <= 1:
            raise HTTPException(
                status_code=400, detail="image_url and parent_post_id cannot be used together"
            )
        if not prompt and not reference_items:
            raise HTTPException(
                status_code=400,
                detail="Prompt cannot be empty when no image_url/parent_post_id is provided",
            )

    reasoning_effort = (data.reasoning_effort or "").strip() or None
    if reasoning_effort:
        allowed = {"none", "minimal", "low", "medium", "high", "xhigh"}
        if reasoning_effort not in allowed:
            raise HTTPException(
                status_code=400,
                detail=f"reasoning_effort must be one of {sorted(allowed)}",
            )
    edit_context = data.edit_context or {}
    if not isinstance(edit_context, dict):
        raise HTTPException(status_code=400, detail="edit_context must be an object")
    # 限制 edit_context 仅用于审计字段，避免日志/请求体膨胀。
    if len(orjson.dumps(edit_context)) > 8192:
        raise HTTPException(status_code=400, detail="edit_context too large")
    if edit_context:
        logger.info(
            "Public video edit context: "
            f"source_video_url={str(edit_context.get('source_video_url') or '')[:120]}, "
            f"splice_at_ms={edit_context.get('splice_at_ms')}, "
            f"frame_index={edit_context.get('frame_index')}, "
            f"round={edit_context.get('round')}"
        )

    task_ids: List[str] = []
    for _ in range(concurrent):
        task_id = await _new_session(
            prompt,
            aspect_ratio,
            video_length,
            resolution_name,
            preset,
            image_url,
            parent_post_id,
            source_image_url,
            reference_items,
            reasoning_effort,
            single_image_mode=single_image_mode,
            is_video_extension=is_video_extension,
            extend_post_id=extend_post_id,
            video_extension_start_time=video_extension_start_time,
            original_post_id=original_post_id,
            file_attachment_id=file_attachment_id,
            stitch_with_extend=stitch_with_extend,
        )
        task_ids.append(task_id)

    return {
        "task_id": task_ids[0],
        "task_ids": task_ids,
        "concurrent": concurrent,
        "aspect_ratio": aspect_ratio,
        "parent_post_id": parent_post_id,
        "reference_count": len(reference_items),
        "extend_post_id": extend_post_id,
        "file_attachment_id": file_attachment_id or "",
        "single_image_mode": single_image_mode,
    }


@router.get("/video/sse")
async def public_video_sse(request: Request, task_id: str = Query("")):
    session = await _get_session(task_id)
    if not session:
        raise HTTPException(status_code=404, detail="Task not found")

    prompt = str(session.get("prompt") or "").strip()
    aspect_ratio = str(session.get("aspect_ratio") or "3:2")
    video_length = int(session.get("video_length") or 6)
    resolution_name = str(session.get("resolution_name") or "480p")
    preset = str(session.get("preset") or "normal")
    image_url = session.get("image_url")
    parent_post_id = str(session.get("parent_post_id") or "").strip()
    source_image_url = str(session.get("source_image_url") or "").strip() or None
    reference_items = session.get("reference_items") or []
    reasoning_effort = session.get("reasoning_effort")
    single_image_mode = str(session.get("single_image_mode") or "frame").strip() or "frame"

    async def event_stream():
        try:
            model_id = "grok-imagine-1.0-video"
            model_info = ModelService.get(model_id)
            if not model_info or not model_info.is_video:
                payload = {
                    "error": "Video model is not available.",
                    "code": "model_not_supported",
                }
                yield f"data: {orjson.dumps(payload).decode()}\n\n"
                yield "data: [DONE]\n\n"
                return

            if image_url:
                messages: List[Dict[str, Any]] = [
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": prompt},
                            {"type": "image_url", "image_url": {"url": image_url}},
                        ],
                    }
                ]
            else:
                messages = [{"role": "user", "content": prompt}]

            # 从 session 取得视频延长参数
            is_video_extension = bool(session.get("is_video_extension"))
            extend_post_id = str(session.get("extend_post_id") or "").strip() or None
            video_extension_start_time = session.get("video_extension_start_time")
            original_post_id = str(session.get("original_post_id") or "").strip() or None
            file_attachment_id = str(session.get("file_attachment_id") or "").strip() or None
            stitch_with_extend = bool(session.get("stitch_with_extend", True))

            stream = await VideoService.completions(
                model_id,
                messages,
                stream=True,
                reasoning_effort=reasoning_effort,
                aspect_ratio=aspect_ratio,
                video_length=video_length,
                resolution=resolution_name,
                preset=preset,
                parent_post_id=parent_post_id or None,
                extend_post_id=extend_post_id if is_video_extension else None,
                video_extension_start_time=video_extension_start_time if is_video_extension else None,
                original_post_id=original_post_id if is_video_extension else None,
                file_attachment_id=file_attachment_id if is_video_extension else None,
                stitch_with_extend=stitch_with_extend,
                source_image_url=source_image_url,
                reference_items=reference_items,
                single_image_mode=single_image_mode,
            )

            async for chunk in stream:
                if await request.is_disconnected():
                    logger.info(f"Public video client disconnected: {task_id}")
                    break
                if task_id not in _VIDEO_SESSIONS:
                    logger.info(f"Public video task stopped by user: {task_id}")
                    break
                yield chunk
        except Exception as e:
            logger.warning(f"Public video SSE error: {e}")
            payload = _public_video_error_payload(e)
            yield f"data: {orjson.dumps(payload).decode()}\n\n"
            yield "data: [DONE]\n\n"
        finally:
            await _drop_session(task_id)

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "Connection": "keep-alive"},
    )


class VideoStopRequest(BaseModel):
    task_ids: List[str]


@router.post("/video/stop", dependencies=[Depends(verify_public_key)])
async def public_video_stop(data: VideoStopRequest):
    removed = await _drop_sessions(data.task_ids or [])
    return {"status": "success", "removed": removed}


@router.get("/video/cache/list", dependencies=[Depends(verify_public_key)])
async def public_video_cache_list(page: int = 1, page_size: int = 100):
    page = max(1, int(page or 1))
    page_size = max(1, min(200, int(page_size or 100)))
    cache_service = CacheService()
    result = cache_service.list_files("video", page=page, page_size=page_size)
    return {"status": "success", **result}


@router.post("/video/rename", dependencies=[Depends(verify_public_key)])
async def public_video_rename(data: VideoRenameRequest):
    cache_service = CacheService()
    try:
        result = cache_service.update_video_display_name(
            post_id=str(data.post_id or "").strip(),
            share_link=str(data.share_link or "").strip(),
            name=str(data.name or "").strip(),
            display_name=str(data.display_name or "").strip(),
        )
        return {"status": "success", "result": result}
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


__all__ = ["router"]
