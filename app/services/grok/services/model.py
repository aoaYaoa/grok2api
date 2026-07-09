"""
Grok 模型管理服务
"""

from enum import Enum
from typing import Optional, Tuple, List
from pydantic import BaseModel, Field

from app.core.config import get_config
from app.core.exceptions import ValidationException


class Tier(str, Enum):
    """模型档位"""

    BASIC = "basic"
    SUPER = "super"
    HEAVY = "heavy"


class Cost(str, Enum):
    """计费类型"""

    LOW = "low"
    HIGH = "high"


class ModelInfo(BaseModel):
    """模型信息"""

    model_id: str
    grok_model: str
    model_mode: str
    tier: Tier = Field(default=Tier.BASIC)
    cost: Cost = Field(default=Cost.LOW)
    display_name: str
    description: str = ""
    is_image: bool = False
    is_image_edit: bool = False
    is_video: bool = False
    public: bool = True
    prefer_best: bool = False


class ModelService:
    """模型管理服务"""

    MODELS = [
        ModelInfo(
            model_id="grok-3",
            grok_model="grok-3",
            model_mode="MODEL_MODE_GROK_3",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-3",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-3-mini",
            grok_model="grok-3",
            model_mode="MODEL_MODE_GROK_3_MINI_THINKING",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-3-MINI",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-3-thinking",
            grok_model="grok-3",
            model_mode="MODEL_MODE_GROK_3_THINKING",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-3-THINKING",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4",
            grok_model="grok-4",
            model_mode="MODEL_MODE_GROK_4",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4-thinking",
            grok_model="grok-4",
            model_mode="MODEL_MODE_GROK_4_THINKING",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4-THINKING",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4-heavy",
            grok_model="grok-4",
            model_mode="MODEL_MODE_HEAVY",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-4-HEAVY",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.1-mini",
            grok_model="grok-4-1-thinking-1129",
            model_mode="MODEL_MODE_GROK_4_1_MINI_THINKING",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4.1-MINI",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.1-fast",
            grok_model="grok-4-1-thinking-1129",
            model_mode="MODEL_MODE_FAST",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4.1-FAST",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.1-expert",
            grok_model="grok-4-1-thinking-1129",
            model_mode="MODEL_MODE_EXPERT",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="GROK-4.1-EXPERT",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.1-thinking",
            grok_model="grok-4-1-thinking-1129",
            model_mode="MODEL_MODE_GROK_4_1_THINKING",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="GROK-4.1-THINKING",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-non-reasoning",
            grok_model="grok-420",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309-NON-REASONING",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-reasoning",
            grok_model="grok-420",
            model_mode="expert",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-4.20-0309-REASONING",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-non-reasoning-super",
            grok_model="grok-420",
            model_mode="fast",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309-NON-REASONING-SUPER",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-super",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309-SUPER",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-reasoning-super",
            grok_model="grok-420",
            model_mode="expert",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-4.20-0309-REASONING-SUPER",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-non-reasoning-heavy",
            grok_model="grok-420",
            model_mode="fast",
            tier=Tier.HEAVY,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309-NON-REASONING-HEAVY",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-heavy",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.HEAVY,
            cost=Cost.LOW,
            display_name="GROK-4.20-0309-HEAVY",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-0309-reasoning-heavy",
            grok_model="grok-420",
            model_mode="expert",
            tier=Tier.HEAVY,
            cost=Cost.HIGH,
            display_name="GROK-4.20-0309-REASONING-HEAVY",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-multi-agent-0309",
            grok_model="grok-420",
            model_mode="heavy",
            tier=Tier.HEAVY,
            cost=Cost.HIGH,
            display_name="GROK-4.20-MULTI-AGENT-0309",
            is_image=False,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-4.20-auto",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.20-AUTO",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.5",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.5",
            description="Latest Grok 4.5 model",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.5-fast",
            grok_model="grok-420",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4.5-FAST",
            description="Latest Grok 4.5 fast mode",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.5-expert",
            grok_model="grok-420",
            model_mode="expert",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-4.5-EXPERT",
            description="Latest Grok 4.5 reasoning mode",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.5-heavy",
            grok_model="grok-420",
            model_mode="heavy",
            tier=Tier.HEAVY,
            cost=Cost.HIGH,
            display_name="GROK-4.5-HEAVY",
            description="Latest Grok 4.5 heavy mode",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.20-fast",
            grok_model="grok-420",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.LOW,
            display_name="GROK-4.20-FAST",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.20-expert",
            grok_model="grok-420",
            model_mode="expert",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-4.20-EXPERT",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.20-heavy",
            grok_model="grok-420",
            model_mode="heavy",
            tier=Tier.HEAVY,
            cost=Cost.HIGH,
            display_name="GROK-4.20-HEAVY",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            prefer_best=True,
        ),
        ModelInfo(
            model_id="grok-4.20-beta",
            grok_model="grok-420",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.20-AUTO",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            public=False,
        ),
        ModelInfo(
            model_id="grok-4.3-beta",
            grok_model="grok-420",
            model_mode="grok-420-computer-use-sa",
            tier=Tier.SUPER,
            cost=Cost.LOW,
            display_name="GROK-4.3-BETA",
            description="Grok 4.3 beta model",
            is_image=False,
            is_image_edit=False,
            is_video=False,
            public=False,
        ),
        ModelInfo(
            model_id="grok-imagine-image-lite",
            grok_model="grok-3",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="GROK-IMAGINE-IMAGE-LITE",
            description="Image generation model",
            is_image=True,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-imagine-image",
            grok_model="grok-3",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-IMAGINE-IMAGE",
            description="Image generation model",
            is_image=True,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-imagine-image-pro",
            grok_model="grok-3",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-IMAGINE-IMAGE-PRO",
            description="Image generation model",
            is_image=True,
            is_image_edit=False,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-imagine-image-edit",
            grok_model="imagine-image-edit",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-IMAGINE-IMAGE-EDIT",
            description="Image edit model",
            is_image=False,
            is_image_edit=True,
            is_video=False,
        ),
        ModelInfo(
            model_id="grok-imagine-video",
            grok_model="grok-3",
            model_mode="auto",
            tier=Tier.SUPER,
            cost=Cost.HIGH,
            display_name="GROK-IMAGINE-VIDEO",
            description="Video generation model",
            is_image=False,
            is_image_edit=False,
            is_video=True,
        ),
        ModelInfo(
            model_id="grok-imagine-1.0",
            grok_model="grok-3",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="Grok Image (Legacy)",
            description="Legacy image generation model",
            is_image=True,
            is_image_edit=False,
            is_video=False,
            public=False,
        ),
        ModelInfo(
            model_id="grok-imagine-1.0-edit",
            grok_model="imagine-image-edit",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="Grok Image Edit (Legacy)",
            description="Legacy image edit model",
            is_image=False,
            is_image_edit=True,
            is_video=False,
            public=False,
        ),
        ModelInfo(
            model_id="grok-imagine-1.0-video",
            grok_model="grok-3",
            model_mode="fast",
            tier=Tier.BASIC,
            cost=Cost.HIGH,
            display_name="Grok Video (Legacy)",
            description="Legacy video generation model",
            is_image=False,
            is_image_edit=False,
            is_video=True,
            public=False,
        ),
    ]

    _map = {m.model_id: m for m in MODELS}

    @classmethod
    def _default_pool_candidates(cls, model_id: str) -> List[str]:
        """返回模型默认池顺序。"""
        model = cls.get(model_id)
        if not model:
            return ["ssoBasic", "ssoSuper"]
        if model_id in {
            "grok-imagine-1.0",
            "grok-imagine-1.0-edit",
            "grok-imagine-1.0-video",
            "grok-imagine-image-lite",
            "grok-imagine-image",
            "grok-imagine-image-pro",
            "grok-imagine-image-edit",
            "grok-imagine-video",
        }:
            return ["ssoHeavy", "ssoSuper", "ssoBasic"]
        if model.prefer_best:
            if model.tier == Tier.HEAVY:
                return ["ssoHeavy", "ssoSuper"]
            if model.tier == Tier.SUPER:
                return ["ssoHeavy", "ssoSuper"]
            return ["ssoHeavy", "ssoSuper", "ssoBasic"]
        if model.tier == Tier.HEAVY:
            return ["ssoHeavy", "ssoSuper"]
        if model.tier == Tier.SUPER:
            return ["ssoSuper", "ssoHeavy"]
        return ["ssoBasic", "ssoSuper", "ssoHeavy"]

    @classmethod
    def _routing_lookup_keys(cls, model_id: str) -> List[str]:
        """返回模型池路由查找顺序，兼容历史别名。"""
        keys = [model_id]
        if model_id == "grok-4.20-beta":
            keys.append("grok-4.20-auto")
        if model_id.startswith("grok-imagine-image") and model_id != "grok-imagine-image-edit":
            keys.append("grok-imagine-1.0")
        if model_id == "grok-imagine-image-edit":
            keys.append("grok-imagine-1.0-edit")
        if model_id == "grok-imagine-video":
            keys.append("grok-imagine-1.0-video")
        return keys

    @classmethod
    def _configured_pool_candidates(cls, model_id: str) -> Optional[List[str]]:
        """读取管理配置中的模型池路由。"""
        routing = get_config("model_routing.model_pools", {})
        if not isinstance(routing, dict):
            return None

        for key in cls._routing_lookup_keys(model_id):
            value = routing.get(key)
            if isinstance(value, str):
                pool = value.strip()
                if pool:
                    return [pool]
            elif isinstance(value, list):
                pools = [
                    str(item).strip()
                    for item in value
                    if str(item).strip()
                ]
                if pools:
                    return pools
        return None

    @classmethod
    def get(cls, model_id: str) -> Optional[ModelInfo]:
        """获取模型信息"""
        return cls._map.get(model_id)

    @classmethod
    def list(cls) -> list[ModelInfo]:
        """获取所有模型"""
        return [model for model in cls._map.values() if model.public]

    @classmethod
    def valid(cls, model_id: str) -> bool:
        """模型是否有效"""
        return model_id in cls._map

    @classmethod
    def to_grok(cls, model_id: str) -> Tuple[str, str]:
        """转换为 Grok 参数"""
        model = cls.get(model_id)
        if not model:
            raise ValidationException(f"Invalid model ID: {model_id}")
        return model.grok_model, model.model_mode

    @classmethod
    def pool_for_model(cls, model_id: str) -> str:
        """根据模型选择 Token 池"""
        pools = cls.pool_candidates_for_model(model_id)
        return pools[0] if pools else "ssoBasic"

    @classmethod
    def pool_candidates_for_model(cls, model_id: str) -> List[str]:
        """按优先级返回可用 Token 池列表"""
        configured = cls._configured_pool_candidates(model_id)
        if configured:
            return configured
        return cls._default_pool_candidates(model_id)

    @classmethod
    def quota_mode_for_model(cls, model_id: str) -> str:
        """返回模型对应的额度窗口键。"""
        model = cls.get(model_id)
        if not model:
            return "auto"

        if model.model_id == "grok-4.3-beta":
            return "grok_4_3"

        if model.model_id in {
            "grok-4.20-fast",
            "grok-4.1-fast",
            "grok-imagine-1.0",
            "grok-imagine-1.0-edit",
            "grok-imagine-1.0-video",
        }:
            return "fast"

        if model.model_id in {
            "grok-4.20-expert",
            "grok-4.1-expert",
            "grok-4.20-heavy",
        }:
            return "expert"

        if model.model_id == "grok-4-heavy":
            return "heavy"

        mode = str(model.model_mode or "").strip().lower()
        if mode == "fast" or mode == "model_mode_fast":
            return "fast"
        if mode == "expert" or mode == "model_mode_expert":
            return "expert"
        if mode == "heavy":
            return "heavy"
        if mode == "model_mode_heavy":
            return "heavy"
        if mode == "grok-420-computer-use-sa":
            return "grok_4_3"
        return "auto"


__all__ = ["ModelService"]
