"""
视频操作专用 API 路由
"""

import re
import time
from typing import Any, Optional, Union

from fastapi import APIRouter, Depends, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.services.grok.services.video import VideoService
from app.core.auth import verify_api_key
from app.core.exceptions import AppException, ErrorType, ValidationException
from app.core.logger import logger
from app.api.v1.chat import _build_streaming_response, _chat_error_as_success_response, _video_error_message

router = APIRouter(tags=["Video"])

SIZE_TO_ASPECT_RATIO = {
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

QUALITY_TO_RESOLUTION = {
    "standard": "480p",
    "high": "720p",
}


def _clean_url(url: str) -> str:
    value = str(url or "").strip().strip('"').strip("'")
    return value.rstrip("\\")


def _resolve_aspect_ratio(size: str) -> str:
    resolved = SIZE_TO_ASPECT_RATIO.get(str(size or "").strip())
    if resolved:
        return resolved
    raise ValidationException(
        message=f"size must be one of {sorted(SIZE_TO_ASPECT_RATIO)}",
        param="size",
        code="invalid_size",
    )


def _resolve_resolution(quality: str) -> str:
    resolved = QUALITY_TO_RESOLUTION.get(str(quality or "").strip().lower())
    if resolved:
        return resolved
    raise ValidationException(
        message="quality must be one of ['standard', 'high']",
        param="quality",
        code="invalid_quality",
    )


def _extract_reference_url(image_reference: Optional[Union[str, dict[str, Any]]]) -> Optional[str]:
    if image_reference is None:
        return None
    if isinstance(image_reference, str):
        value = image_reference.strip()
        return value or None
    if isinstance(image_reference, dict):
        raw = image_reference.get("image_url")
        if isinstance(raw, str):
            value = raw.strip()
            return value or None
        if isinstance(raw, dict):
            nested = str(raw.get("url") or "").strip()
            return nested or None
    raise ValidationException(
        message="image_reference must be a string or an object with image_url",
        param="image_reference",
        code="invalid_image_reference",
    )


def _extract_video_url(content: str) -> str:
    text = str(content or "").strip()
    if not text:
        return ""

    src_match = re.search(r'<source[^>]+src="([^"]+)"', text, re.IGNORECASE)
    if src_match:
        return _clean_url(src_match.group(1))

    video_match = re.search(r'<video[^>]+src="([^"]+)"', text, re.IGNORECASE)
    if video_match:
        return _clean_url(video_match.group(1))

    markdown_match = re.search(r"\[video\]\(([^)]+)\)", text, re.IGNORECASE)
    if markdown_match:
        return _clean_url(markdown_match.group(1))

    url_matches = re.findall(r"https?://[^\s<)]+", text)
    if url_matches:
        return _clean_url(url_matches[-1])

    return ""


def _videos_error_response(exc: Exception) -> JSONResponse:
    status = 500
    code = "video_internal_error"
    error_type = ErrorType.SERVER.value

    if isinstance(exc, ValidationException):
        status = 400
        code = exc.code or "invalid_request"
        error_type = ErrorType.INVALID_REQUEST.value
    elif isinstance(exc, AppException):
        status = exc.status_code or 500
        code = exc.code or code
        error_type = exc.error_type

    message = _video_error_message(exc)
    if isinstance(exc, ValidationException):
        message = exc.message

    return JSONResponse(
        status_code=status,
        content={
            "created": int(time.time()),
            "data": [],
            "error": {
                "message": message,
                "type": error_type,
                "code": code,
            },
        },
    )


class VideoGenerationRequest(BaseModel):
    """兼容纯 HTML 客户端的 /videos 请求"""

    prompt: str = Field(..., description="视频提示词")
    model: str = Field("grok-imagine-1.0-video", description="模型名称")
    size: str = Field("1280x720", description="视频尺寸/比例")
    seconds: int = Field(6, description="目标时长（秒）")
    quality: str = Field("standard", description="输出质量")
    image_reference: Optional[Union[str, dict[str, Any]]] = Field(
        None, description="可选参考图"
    )
    stream: bool = Field(False, description="是否流式输出")


class VideoExtendRequest(BaseModel):
    """视频延长请求"""
    model: str = Field("grok-imagine-1.0-video", description="模型名称")
    post_id: str = Field(..., description="原始视频的 post_id")
    prompt: Optional[str] = Field(None, description="延长描述词")
    video_length: Optional[int] = Field(6, description="延长时长 (6/10/15)")
    aspect_ratio: Optional[str] = Field("16:9", description="视频比例")
    resolution: Optional[str] = Field("480p", description="分辨率 (480p/720p)")
    stream: Optional[bool] = Field(True, description="是否流式返回")
    video_extension_start_time: Optional[float] = Field(None, description="延长开始时间点")
    stitch_with_extend: Optional[bool] = Field(True, description="是否拼接之前的视频")


@router.post("/videos")
async def create_video(request: VideoGenerationRequest):
    """OpenAI videos.create 风格兼容接口，返回统一的 url 数据结构。"""
    try:
        prompt = str(request.prompt or "").strip()
        if not prompt:
            raise ValidationException(
                message="Prompt cannot be empty",
                param="prompt",
                code="empty_prompt",
            )
        if request.model != "grok-imagine-1.0-video":
            raise ValidationException(
                message="The model `grok-imagine-1.0-video` is required for video generation.",
                param="model",
                code="model_not_supported",
            )
        if request.stream:
            raise ValidationException(
                message="stream=true is not supported on /videos compatibility endpoint",
                param="stream",
                code="invalid_stream",
            )
        if int(request.seconds) < 6 or int(request.seconds) > 30:
            raise ValidationException(
                message="seconds must be between 6 and 30",
                param="seconds",
                code="invalid_seconds",
            )

        aspect_ratio = _resolve_aspect_ratio(request.size)
        resolution = _resolve_resolution(request.quality)
        reference_url = _extract_reference_url(request.image_reference)

        if reference_url:
            messages = [
                {
                    "role": "user",
                    "content": [
                        {"type": "text", "text": prompt},
                        {"type": "image_url", "image_url": {"url": reference_url}},
                    ],
                }
            ]
        else:
            messages = [{"role": "user", "content": prompt}]

        result = await VideoService.completions(
            model=request.model,
            messages=messages,
            stream=False,
            aspect_ratio=aspect_ratio,
            video_length=int(request.seconds),
            resolution=resolution,
            preset="custom",
            single_image_mode="reference",
        )

        content = ""
        if isinstance(result, dict):
            direct_url = (
                result.get("url") or result.get("video_url") or result.get("file_url") or ""
            )
            if isinstance(direct_url, str) and direct_url.strip():
                content = direct_url.strip()
            else:
                choices = result.get("choices") or []
                if choices:
                    message = choices[0].get("message") or {}
                    content = str(message.get("content") or "")

        video_url = _extract_video_url(content)
        if not video_url:
            raise AppException(
                message="Video generation succeeded but no playable video URL was returned.",
                error_type=ErrorType.SERVER.value,
                code="video_missing_url",
                status_code=502,
            )

        created = int(result.get("created") or time.time()) if isinstance(result, dict) else int(time.time())
        return JSONResponse(
            content={
                "created": created,
                "data": [{"url": video_url}],
            }
        )
    except Exception as exc:
        logger.error(f"Videos compatibility API error: {exc}")
        return _videos_error_response(exc)


@router.post("/video/extend", dependencies=[Depends(verify_api_key)])
async def extend_video(request: VideoExtendRequest, raw_request: Request):
    """专用视频延长接口"""
    logger.info(f"Video extension requested via dedicated API: post_id={request.post_id}")
    
    # 自动偏移并限制 30s 逻辑
    # Grok 延长时长固定为 6s 或 10s，总长上限 30s
    extension_start = request.video_extension_start_time or 0.0
    requested_length = request.video_length or 6

    # 如果用户请求的长超过 30s 剩余空间，则我们将开始时间“往前回拨”以容纳该长度，
    # 但最高不能超过 30s。若回拨到 0 还是放不下，则由底层处理。
    if extension_start + requested_length > 30.0:
        new_start = max(0.0, 30.0 - requested_length)
        logger.info(
            f"Optimizing extension start time: {extension_start}s -> {new_start}s "
            f"to fit {requested_length}s extension (total capped at 30s)"
        )
        extension_start = new_start
    
    video_length = requested_length

    # 构造兼容 VideoService.completions 的参数
    # 如果没有传 prompt，传入 None 或空，VideoService 会处理为默认 animate 指令
    messages = []
    if request.prompt:
        messages.append({"role": "user", "content": request.prompt})
    else:
        # 即使留空，VideoService 也会处理，这里确保至少有一个 user 消息体以便提取
        messages.append({"role": "user", "content": ""})

    try:
        # 使用 VideoService.completions 统一入口
        result = await VideoService.completions(
            model=request.model,
            messages=messages,
            stream=request.stream,
            aspect_ratio=request.aspect_ratio or "16:9",
            video_length=video_length,
            resolution=request.resolution or "480p",
            extend_post_id=request.post_id,
            video_extension_start_time=extension_start,
            stitch_with_extend=request.stitch_with_extend
        )
        
        if request.stream:
            return _build_streaming_response(result, raw_request, request.model)
        else:
            return result
            
    except Exception as e:
        logger.error(f"Video extension API error: {e}")
        return _chat_error_as_success_response(request.model, _video_error_message(e))
