import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

from app.core.exceptions import UpstreamException
from app.services.grok.services.video import VideoService
import app.services.grok.utils.asset_token_map as asset_token_map


class FakeTokenManager:
    def __init__(self):
        self.record_fail = AsyncMock(return_value=True)
        self.consume = AsyncMock(return_value=True)
        self.reload_if_stale = AsyncMock(return_value=None)
        self._fallback = SimpleNamespace(token="good-token")

    def get_pool_name_for_token(self, token):
        return "pool-a" if token in {"bad-token", "good-token"} else None

    def get_token_for_video(self, **kwargs):
        return self._fallback


class FakeCollector:
    def __init__(self, *args, **kwargs):
        pass

    async def process(self, response):
        return {"ok": True, "response": response}


class VideoAuthFallbackTests(unittest.IsolatedAsyncioTestCase):
    async def test_falls_back_when_preferred_token_is_invalid(self):
        manager = FakeTokenManager()
        auth_error = UpstreamException(
            "Failed to look up session ID. [WKE=unauthenticated:invalid-credentials]",
            details={"body": "unauthenticated:invalid-credentials"},
        )

        with patch("app.services.grok.services.video.get_token_manager", AsyncMock(return_value=manager)), \
             patch("app.services.grok.services.video.get_config", side_effect=lambda key, default=None: {
                 "retry.max_retry": 2,
                 "video.auto_upscale": False,
             }.get(key, default)), \
             patch.object(asset_token_map.AssetTokenMap, "get_instance", AsyncMock(return_value=SimpleNamespace(get_token=AsyncMock(return_value=None)))), \
             patch("app.services.grok.services.model.ModelService.pool_candidates_for_model", return_value=[]), \
             patch("app.services.grok.services.chat.MessageExtractor.extract", return_value=("video prompt", [], [])), \
             patch("app.services.grok.services.video.VideoCollectProcessor", FakeCollector), \
             patch.object(VideoService, "generate_from_parent_post", AsyncMock(side_effect=[auth_error, "fake-stream"])):
            result = await VideoService.completions(
                "grok-imagine-1.0-video",
                messages=[{"role": "user", "content": "video prompt"}],
                stream=False,
                reasoning_effort="none",
                aspect_ratio="16:9",
                video_length=6,
                resolution="480p",
                preset="normal",
                parent_post_id="12345678-1234-1234-1234-123456789abc",
                preferred_token="bad-token",
            )

        self.assertEqual(result["ok"], True)
        manager.record_fail.assert_awaited_once_with("bad-token", 401, "video_auth_failed")

    async def test_parent_post_with_preferred_bound_token_skips_source_upload_fallback(self):
        manager = FakeTokenManager()

        with patch("app.services.grok.services.video.get_token_manager", AsyncMock(return_value=manager)), \
             patch("app.services.grok.services.video.get_config", side_effect=lambda key, default=None: {
                 "retry.max_retry": 1,
                 "video.auto_upscale": False,
             }.get(key, default)), \
             patch.object(asset_token_map.AssetTokenMap, "get_instance", AsyncMock(return_value=SimpleNamespace(get_token=AsyncMock(return_value=None)))), \
             patch("app.services.grok.services.model.ModelService.pool_candidates_for_model", return_value=[]), \
             patch("app.services.grok.services.chat.MessageExtractor.extract", return_value=("video prompt", [], [])), \
             patch("app.services.grok.services.video.VideoCollectProcessor", FakeCollector), \
             patch("app.services.grok.utils.upload.UploadService.upload_file", AsyncMock(side_effect=AssertionError("source upload fallback should not run when preferred token is already bound"))), \
             patch.object(VideoService, "generate_from_image", AsyncMock(side_effect=AssertionError("image fallback should not be used when preferred token is already bound"))), \
             patch.object(VideoService, "generate_from_parent_post", AsyncMock(return_value="fake-stream")) as generate_from_parent_post:
            result = await VideoService.completions(
                "grok-imagine-1.0-video",
                messages=[{"role": "user", "content": "video prompt"}],
                stream=False,
                reasoning_effort="none",
                aspect_ratio="3:2",
                video_length=6,
                resolution="480p",
                preset="normal",
                parent_post_id="37b7e1ef-2bba-4a9c-b47c-8d2580734617",
                source_image_url="http://localhost:18000/v1/files/image/users/demo/generated/37b7e1ef-2bba-4a9c-b47c-8d2580734617/image.jpg",
                preferred_token="good-token",
            )

        self.assertEqual(result["ok"], True)
        kwargs = generate_from_parent_post.await_args.kwargs
        self.assertEqual(kwargs["token"], "good-token")
        self.assertEqual(kwargs["parent_post_id"], "37b7e1ef-2bba-4a9c-b47c-8d2580734617")

    async def test_parent_post_without_bound_token_falls_back_to_uploaded_source_image(self):
        manager = FakeTokenManager()
        uploaded_uri = "users/demo/uploaded/fallback-source.jpg"

        with patch("app.services.grok.services.video.get_token_manager", AsyncMock(return_value=manager)), \
             patch("app.services.grok.services.video.get_config", side_effect=lambda key, default=None: {
                 "retry.max_retry": 1,
                 "video.auto_upscale": False,
             }.get(key, default)), \
             patch.object(asset_token_map.AssetTokenMap, "get_instance", AsyncMock(return_value=SimpleNamespace(get_token=AsyncMock(return_value=None)))), \
             patch("app.services.grok.services.model.ModelService.pool_candidates_for_model", return_value=[]), \
             patch("app.services.grok.services.chat.MessageExtractor.extract", return_value=("video prompt", [], [])), \
             patch("app.services.grok.services.video.VideoCollectProcessor", FakeCollector), \
             patch("app.services.grok.utils.upload.UploadService.upload_file", AsyncMock(return_value=("file-id", uploaded_uri))) as upload_file, \
             patch.object(VideoService, "create_image_post", AsyncMock(return_value="uploaded-post")), \
             patch.object(VideoService, "generate_from_parent_post", AsyncMock(side_effect=AssertionError("parentPost path should not be used when token binding is missing"))), \
             patch.object(VideoService, "generate_from_image", AsyncMock(return_value="fake-stream")) as generate_from_image:
            result = await VideoService.completions(
                "grok-imagine-1.0-video",
                messages=[{"role": "user", "content": "video prompt"}],
                stream=False,
                reasoning_effort="none",
                aspect_ratio="3:2",
                video_length=6,
                resolution="480p",
                preset="normal",
                parent_post_id="f262a257-e0e8-4041-a645-a5ea3b3c4ef2",
                source_image_url="http://localhost:18000/v1/files/image/users/demo/generated/f262a257-e0e8-4041-a645-a5ea3b3c4ef2/image.jpg",
            )

        self.assertEqual(result["ok"], True)
        upload_file.assert_awaited_once()
        kwargs = generate_from_image.await_args.kwargs
        self.assertEqual(kwargs["image_url"], f"https://assets.grok.com/{uploaded_uri}")
        self.assertEqual(kwargs["token"], "good-token")


if __name__ == "__main__":
    unittest.main()
