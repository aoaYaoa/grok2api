"""Public Chat router (public_key protected)."""

from fastapi import APIRouter, Depends, Request

from app.core.auth import verify_public_key
from app.api.v1.chat import ChatCompletionRequest, chat_completions
from app.services.grok.services.model import ModelService

router = APIRouter(tags=["Public Chat"])


@router.get("/models", dependencies=[Depends(verify_public_key)])
async def public_list_models():
    """公开页面模型列表接口。"""
    data = [
        {
            "id": m.model_id,
            "object": "model",
            "created": 0,
            "owned_by": "grok2api@chenyme",
        }
        for m in ModelService.list()
    ]
    return {"object": "list", "data": data}


@router.post("/chat/completions", dependencies=[Depends(verify_public_key)])
async def public_chat_completions(request: ChatCompletionRequest, raw_request: Request):
    """Public chat completions endpoint."""
    return await chat_completions(request, raw_request)


__all__ = ["router"]
