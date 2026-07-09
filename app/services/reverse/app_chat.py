"""
Reverse interface: app chat conversations.
"""

import orjson
import inspect
import asyncio
from typing import Any, Dict, List, Optional
from curl_cffi.requests import AsyncSession
from curl_cffi.requests.errors import RequestsError

from app.core.logger import logger
from app.core.config import get_config
from app.core.exceptions import UpstreamException
from app.services.token.service import TokenService
from app.services.grok.services.model import ModelService
from app.services.reverse.rate_limits import MODE_NAME_BY_KEY, RateLimitsReverse
from app.services.reverse.browser_bridge import (
    refresh_browser_probe_managed,
    wait_for_browser_probe_refresh,
    warmup_browser_session,
)
from app.services.reverse.utils.headers import build_headers
from app.services.reverse.utils.retry import retry_on_status

CHAT_API = "https://grok.com/rest/app-chat/conversations/new"
CHAT_RESPONSE_API = "https://grok.com/rest/app-chat/conversations/{conversation_id}/responses"
CHAT_RESPONSES_LIST_API = "https://grok.com/rest/app-chat/conversations/{conversation_id}/responses"


def _is_transient_network_error(err: Exception) -> bool:
    """判断是否为可快速重试的弱网/连接类错误。"""
    s = str(err or "").lower()
    keywords = (
        "curl: (28)",  # operation timeout
        "curl: (35)",  # tls connect error
        "tls connect error",
        "ssl connect error",
        "failed to connect",
        "couldn't connect",
        "connection timed out",
        "connection reset",
        "network is unreachable",
        "timed out",
    )
    return any(k in s for k in keywords)


def _extract_error_payload(text: str) -> dict[str, Any]:
    raw = str(text or "").strip()
    if not raw:
        return {}
    try:
        payload = orjson.loads(raw)
    except Exception:
        return {}
    return payload if isinstance(payload, dict) else {}


async def _diagnose_chat_429(
    session: AsyncSession,
    token: str,
    requested_model: str | None,
) -> dict[str, Any]:
    model_id = str(requested_model or "").strip()
    quota_mode = ModelService.quota_mode_for_model(model_id) if model_id else "auto"
    rate_limit_model = MODE_NAME_BY_KEY.get(quota_mode)
    if not rate_limit_model:
        return {"quota_mode": quota_mode}

    try:
        response = await RateLimitsReverse.request(
            session,
            token,
            model_name=rate_limit_model,
        )
        payload = response.json() if response is not None else {}
        if not isinstance(payload, dict):
            payload = {}
        remaining = payload.get("remainingTokens")
        if remaining is None:
            remaining = payload.get("remainingQueries")
        total = payload.get("totalTokens")
        if total is None:
            total = payload.get("totalQueries")
        details = {
            "quota_mode": quota_mode,
            "rate_limit_model": rate_limit_model,
            "quota_remaining": int(remaining or 0),
            "quota_total": int(total if total is not None else remaining or 0),
            "quota_window_seconds": payload.get("windowSizeSeconds"),
            "quota_wait_seconds": payload.get("waitTimeSeconds"),
        }
        if int(remaining or 0) <= 0:
            details["error_code"] = "quota_exhausted"
        return details
    except Exception as e:
        logger.warning(
            "AppChat 429 diagnose failed: "
            f"requested_model={requested_model or '-'}, error={e}"
        )
        return {"quota_mode": quota_mode}


class AppChatReverse:
    """/rest/app-chat/conversations/new reverse interface."""

    @staticmethod
    async def _refresh_probe_background(token: str, reason: str) -> None:
        """Refresh the short-lived browser probe without blocking the active stream."""
        if reason == "app_chat_sse_start" and not get_config("cloakbrowser.refresh_probe_on_sse_start", True):
            return
        if reason == "app_chat_success" and not get_config("cloakbrowser.refresh_probe_after_success", True):
            return
        try:
            logger.info(f"Browser probe background refresh started: reason={reason}")
            await refresh_browser_probe_managed(token, False, reason="app_chat_sse_start")
            logger.info(f"Browser probe background refresh completed: reason={reason}")
        except Exception as exc:
            logger.warning(f"Browser probe background refresh failed: reason={reason}, error={exc}")

    @staticmethod
    def _resolve_custom_personality() -> Optional[str]:
        """Resolve optional custom personality from app config."""
        value = get_config("app.custom_instruction", "")
        if value is None:
            return None
        if not isinstance(value, str):
            value = str(value)
        if not value.strip():
            return None
        return value

    @staticmethod
    def build_payload(
        message: str,
        model: str,
        mode: str = None,
        file_attachments: List[str] = None,
        tool_overrides: Dict[str, Any] = None,
        model_config_override: Dict[str, Any] = None,
        image_generation_count: int | None = None,
        omit_file_attachments: bool = False,
        minimal_payload: bool = False,
        parent_response_id: str | None = None,
        conversation_id: str | None = None,
    ) -> Dict[str, Any]:
        """Build chat payload for Grok app-chat API."""

        attachments = file_attachments or []
        is_follow_up = bool(conversation_id)

        if minimal_payload:
            payload = {
                "temporary": get_config("app.temporary"),
                "modelName": model,
                "message": message,
                "toolOverrides": tool_overrides or {},
                "enableSideBySide": True,
                "responseMetadata": {
                    "experiments": [],
                },
            }
            if model_config_override:
                payload["responseMetadata"]["modelConfigOverride"] = model_config_override
            if (not omit_file_attachments) and attachments:
                payload["fileAttachments"] = attachments
            if model == "grok-420":
                payload["enable420"] = True
                if mode:
                    payload["modeId"] = mode
            elif mode:
                payload["modelMode"] = mode
            return payload

        payload = {
            "deviceEnvInfo": {
                "darkModeEnabled": False,
                "devicePixelRatio": 2,
                "screenWidth": 2056,
                "screenHeight": 1329,
                "viewportWidth": 2056,
                "viewportHeight": 1083,
            },
            "disableMemory": get_config("app.disable_memory"),
            "disableSearch": False,
            "disableSelfHarmShortCircuit": False,
            "disableTextFollowUps": False,
            "enableImageGeneration": True,
            "enableImageStreaming": True,
            "enableSideBySide": True,
            "fileAttachments": attachments,
            "forceConcise": False,
            "forceSideBySide": False,
            "imageAttachments": [],
            "imageGenerationCount": image_generation_count
            if image_generation_count is not None
            else 2,
            "isAsyncChat": False,
            "isReasoning": False,
            "message": message,
            "modelName": model,
            "responseMetadata": {
                "experiments": [],
                "requestModelDetails": {"modelId": model},
            },
            "returnImageBytes": False,
            "returnRawGrokInXaiRequest": False,
            "sendFinalMetadata": True,
            "temporary": get_config("app.temporary"),
            "toolOverrides": tool_overrides or {},
        }

        if is_follow_up:
            payload.pop("modelName", None)
            payload.pop("temporary", None)
            payload.pop("responseMetadata", None)
            payload.pop("isReasoning", None)
            payload.pop("toolOverrides", None)
            payload["parentResponseId"] = parent_response_id or ""
            payload["metadata"] = {"request_metadata": {}}
            payload["isFromGrokFiles"] = False
            payload["skipCancelCurrentInflightRequests"] = False
            payload["isRegenRequest"] = False
            payload["collectionIds"] = []
            payload["disabledConnectorIds"] = []

        if model_config_override:
            if not is_follow_up:
                payload["responseMetadata"]["modelConfigOverride"] = model_config_override

        if omit_file_attachments:
            payload.pop("fileAttachments", None)

        if model == "grok-420":
            if not is_follow_up:
                payload["enable420"] = True
            if mode:
                payload["modeId"] = mode
        elif mode:
            payload["modelMode"] = mode

        custom_personality = AppChatReverse._resolve_custom_personality()
        if custom_personality is not None and "Greet the user" not in message[-1000:]:
            payload["customPersonality"] = custom_personality

        return payload

    @staticmethod
    async def request(
        session: AsyncSession,
        token: str,
        message: str,
        model: str,
        requested_model: str | None = None,
        mode: str = None,
        file_attachments: List[str] = None,
        tool_overrides: Dict[str, Any] = None,
        model_config_override: Dict[str, Any] = None,
        image_generation_count: int | None = None,
        omit_file_attachments: bool = False,
        minimal_payload: bool = False,
        conversation_id: str | None = None,
        parent_response_id: str | None = None,
    ) -> Any:
        """Send app chat request to Grok.
        
        Args:
            session: AsyncSession, the session to use for the request.
            token: str, the SSO token.
            message: str, the message to send.
            model: str, the model to use.
            mode: str, the mode to use.
            file_attachments: List[str], the file attachments to send.
            tool_overrides: Dict[str, Any], the tool overrides to use.
            model_config_override: Dict[str, Any], the model config override to use.

        Returns:
            Any: The response from the request.
        """
        try:
            if (
                get_config("cloakbrowser.enabled", False)
                and get_config("cloakbrowser.sync_session", True)
            ):
                try:
                    if get_config("cloakbrowser.wait_probe_before_request", True):
                        timeout = float(get_config("cloakbrowser.wait_probe_timeout", 8) or 8)
                        await asyncio.to_thread(wait_for_browser_probe_refresh, timeout)
                    session_data = await warmup_browser_session(token)
                    logger.info(
                        "Browser session warmup complete: "
                        f"cookie_len={len(str((session_data or {}).get('cookie_header') or ''))}, "
                        f"ua={'yes' if (session_data or {}).get('user_agent') else 'no'}, "
                        f"statsig={'yes' if (session_data or {}).get('x_statsig_id') else 'no'}"
                    )
                except Exception as sync_error:
                    logger.warning(f"Browser session warmup failed before chat request: {sync_error}")

            # Get proxies
            base_proxy = get_config("proxy.base_proxy_url")
            proxies = {"http": base_proxy, "https": base_proxy} if base_proxy else None

            # Build payload
            payload = AppChatReverse.build_payload(
                message=message,
                model=model,
                mode=mode,
                file_attachments=file_attachments,
                tool_overrides=tool_overrides,
                model_config_override=model_config_override,
                image_generation_count=image_generation_count,
                omit_file_attachments=omit_file_attachments,
                minimal_payload=minimal_payload,
                conversation_id=conversation_id,
                parent_response_id=parent_response_id,
            )
            url = (
                CHAT_RESPONSE_API.format(conversation_id=conversation_id)
                if conversation_id
                else CHAT_API
            )

            def _build_chat_headers(target_url: str) -> Dict[str, str]:
                return build_headers(
                    cookie_token=token,
                    content_type="application/json",
                    origin="https://grok.com",
                    referer="https://grok.com/",
                    request_url=target_url,
                    method="POST",
                )

            # Build headers
            headers = _build_chat_headers(url)
            logger.info(
                "AppChat request prepared: "
                f"requested_model={requested_model or model}, "
                f"upstream_model={model}, "
                f"mode={mode or '-'}, "
                f"message_len={len(message or '')}, "
                f"file_attachments={len(file_attachments or [])}, "
                f"conversation_id={conversation_id or '-'}, "
                f"parent_response_id={parent_response_id or '-'}, "
                f"tools={','.join((tool_overrides or {}).keys()) or '-'}"
            )

            # Curl Config
            base_timeout = max(
                float(get_config("chat.timeout") or 60.0),
                float(get_config("video.timeout") or 60.0),
                float(get_config("image.timeout") or 60.0),
            )
            connect_timeout = float(
                get_config("chat.connect_timeout")
                or min(max(base_timeout, 1.0), 12.0)
            )
            # curl_cffi 支持 (connect_timeout, read_timeout)；流读取阶段仍由上层 idle timeout 控制。
            timeout = (connect_timeout, base_timeout)
            browser = get_config("proxy.browser") or "chrome136"

            async def _post_once(request_headers: Dict[str, str]):
                try:
                    response = await session.post(
                        url,
                        headers=request_headers,
                        data=orjson.dumps(payload),
                        timeout=timeout,
                        stream=True,
                        proxies=proxies,
                        impersonate=browser,
                    )
                except RequestsError as e:
                    if _is_transient_network_error(e):
                        raise UpstreamException(
                            message=f"AppChatReverse transient network error: {e}",
                            details={"status": 599, "error": str(e)},
                        ) from e
                    raise

                return response

            async def _do_request():
                nonlocal headers
                response = await _post_once(headers)

                if (
                    response.status_code == 403
                    and get_config("cloakbrowser.refresh_probe_on_403", True)
                ):
                    try:
                        content = ""
                        try:
                            content = await response.text()
                        except Exception:
                            pass
                        logger.warning(
                            "AppChat 403 with browser probe headers, force refreshing probe and retrying once"
                        )
                        await refresh_browser_probe_managed(token, True, reason="app_chat_403")
                        headers = _build_chat_headers(url)
                        response = await _post_once(headers)
                        logger.info(
                            "AppChat retry after browser probe refresh completed: "
                            f"status={response.status_code}"
                        )
                    except UpstreamException:
                        raise
                    except Exception as refresh_error:
                        logger.warning(f"Browser probe refresh on 403 failed: {refresh_error}")

                if response.status_code != 200:

                    # Get response content
                    content = ""
                    try:
                        content = await response.text()
                    except Exception:
                        pass

                    logger.error(
                        f"AppChatReverse: Chat failed, {response.status_code}",
                        extra={"error_type": "UpstreamException"},
                    )
                    logger.error(f"Response Headers: {response.headers}")
                    logger.error(f"Response Body: {content}")
                    details = {"status": response.status_code, "body": content}
                    payload_data = _extract_error_payload(content)
                    if payload_data:
                        details["payload"] = payload_data
                        if isinstance(payload_data.get("error"), dict):
                            error_obj = payload_data.get("error") or {}
                            error_code = str(error_obj.get("code") or "").strip()
                            error_message = str(error_obj.get("message") or "").strip()
                            if error_code:
                                details["error_code"] = error_code
                            if error_message:
                                details["error_message"] = error_message
                    if response.status_code == 429:
                        details.update(
                            await _diagnose_chat_429(
                                session,
                                token,
                                requested_model or model,
                            )
                        )
                    raise UpstreamException(
                        message=f"AppChatReverse: Chat failed, {response.status_code}",
                        details=details,
                    )

                if get_config("cloakbrowser.refresh_probe_on_sse_start", True):
                    asyncio.create_task(
                        AppChatReverse._refresh_probe_background(token, "app_chat_sse_start")
                    )

                return response

            def extract_status(e: Exception) -> Optional[int]:
                if isinstance(e, UpstreamException):
                    if e.details and "status" in e.details:
                        status = e.details["status"]
                    else:
                        status = getattr(e, "status_code", None)
                    if status == 429:
                        return None
                    return status
                return None

            response = await retry_on_status(
                _do_request,
                extract_status=extract_status,
                retry_status_codes=[502, 599],
            )

            # Stream response
            async def stream_response():
                stream_completed = False
                try:
                    async for line in response.aiter_lines():
                        if line is None:
                            continue
                        if isinstance(line, (bytes, bytearray)):
                            text = line.decode("utf-8", errors="ignore")
                        else:
                            text = str(line)
                        for item in text.splitlines():
                            item = item.strip()
                            if item:
                                yield item
                    stream_completed = True
                finally:
                    try:
                        close_fn = getattr(response, "aclose", None)
                        if callable(close_fn):
                            result = close_fn()
                            if inspect.isawaitable(result):
                                await result
                        else:
                            close_fn = getattr(response, "close", None)
                            if callable(close_fn):
                                result = close_fn()
                                if inspect.isawaitable(result):
                                    await result
                    except Exception:
                        pass
                    if (
                        stream_completed
                        and get_config("cloakbrowser.refresh_probe_after_success", True)
                        and not get_config("cloakbrowser.refresh_probe_on_sse_start", True)
                    ):
                        asyncio.create_task(
                            AppChatReverse._refresh_probe_background(token, "app_chat_success")
                        )

            return stream_response()

        except Exception as e:
            # Handle upstream exception
            if isinstance(e, UpstreamException):
                status = None
                if e.details and "status" in e.details:
                    status = e.details["status"]
                else:
                    status = getattr(e, "status_code", None)
                if status == 401:
                    try:
                        await TokenService.record_fail(
                            token, status, "app_chat_auth_failed"
                        )
                    except Exception:
                        pass
                raise

            # Handle other non-upstream exceptions
            logger.error(
                f"AppChatReverse: Chat failed, {str(e)}",
                extra={"error_type": type(e).__name__},
            )
            raise UpstreamException(
                message=f"AppChatReverse: Chat failed, {str(e)}",
                details={"status": 502, "error": str(e)},
            )

    @staticmethod
    async def fetch_responses(
        session: AsyncSession,
        token: str,
        conversation_id: str,
    ) -> Dict[str, Any]:
        """Fetch stored responses for an existing Grok conversation."""
        try:
            base_proxy = get_config("proxy.base_proxy_url")
            proxies = {"http": base_proxy, "https": base_proxy} if base_proxy else None
            headers = build_headers(
                cookie_token=token,
                content_type="application/json",
                origin="https://grok.com",
                referer=f"https://grok.com/c/{conversation_id}",
            )
            url = CHAT_RESPONSES_LIST_API.format(conversation_id=conversation_id)
            timeout = float(get_config("chat.timeout") or 60.0)
            browser = get_config("proxy.browser")
            response = await session.get(
                url,
                headers=headers,
                timeout=timeout,
                proxies=proxies,
                impersonate=browser,
            )
            text_value = getattr(response, "text", "")
            if callable(text_value):
                maybe_text = text_value()
                text = await maybe_text if inspect.isawaitable(maybe_text) else maybe_text
            else:
                text = text_value
            text = str(text or "")
            if response.status_code != 200:
                logger.warning(
                    "AppChat responses fetch failed: "
                    f"status={response.status_code}, conversation_id={conversation_id}"
                )
                raise UpstreamException(
                    message=f"AppChatReverse: Responses fetch failed, {response.status_code}",
                    details={"status": response.status_code, "body": text},
                )
            payload = orjson.loads(text)
            return payload if isinstance(payload, dict) else {}
        except UpstreamException:
            raise
        except Exception as e:
            logger.warning(
                "AppChat responses fetch failed: "
                f"conversation_id={conversation_id}, error={e}"
            )
            raise UpstreamException(
                message=f"AppChatReverse: Responses fetch failed, {str(e)}",
                details={"status": 502, "error": str(e)},
            )


__all__ = ["AppChatReverse"]
