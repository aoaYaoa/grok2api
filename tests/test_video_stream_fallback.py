import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

import orjson

from app.services.grok.services.video import VideoStreamProcessor
import app.services.grok.utils.asset_token_map as asset_token_map


class FakeAssetsResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


async def stream_response_without_video_url(post_id: str):
    payload = {
        "result": {
            "response": {
                "responseId": "resp_stream_fallback",
                "streamingVideoGenerationResponse": {
                    "progress": 100,
                    "videoPostId": post_id,
                    "thumbnailImageUrl": "/thumbs/fallback.jpg",
                },
            }
        }
    }
    yield orjson.dumps(payload)


async def stream_response_with_attachment_only(post_id: str):
    payload = {
        "result": {
            "response": {
                "responseId": "resp_stream_attachment_only",
                "modelResponse": {
                    "fileAttachments": [post_id],
                },
            }
        }
    }
    yield orjson.dumps(payload)


class VideoStreamFallbackTests(unittest.IsolatedAsyncioTestCase):
    async def test_stream_uses_asset_lookup_when_video_url_is_missing(self):
        post_id = "12345678-1234-1234-1234-123456789abc"
        fake_token_map = SimpleNamespace(save_mapping=AsyncMock(return_value=None))
        fake_asset_payload = {
            "assets": [
                {
                    "assetId": post_id,
                    "key": f"/users/demo/generated/{post_id}/generated_video.mp4",
                    "mimeType": "video/mp4",
                    "previewImageKey": "/thumbs/fallback.jpg",
                }
            ],
            "nextPageToken": "",
        }

        processor = VideoStreamProcessor(
            "grok-imagine-1.0-video",
            token="good-token",
            show_think=False,
            upscale_on_finish=False,
        )

        with patch.object(
            asset_token_map.AssetTokenMap,
            "get_instance",
            AsyncMock(return_value=fake_token_map),
        ), patch(
            "app.services.grok.services.video.get_config",
            side_effect=lambda key, default=None: {
                "video.stream_timeout": 0,
            }.get(key, default),
        ), patch(
            "app.services.grok.services.video.AssetsListReverse.request",
            AsyncMock(return_value=FakeAssetsResponse(fake_asset_payload)),
        ), patch(
            "app.services.grok.utils.download.DownloadService.render_video",
            AsyncMock(return_value="/v1/files/video/fallback.mp4"),
        ) as render_video:
            chunks = []
            async for chunk in processor.process(
                stream_response_without_video_url(post_id)
            ):
                chunks.append(chunk)

        joined = "\n".join(chunks)
        self.assertIn("/v1/files/video/fallback.mp4", joined)
        render_video.assert_awaited_once()

    async def test_stream_uses_attachment_fallback_before_emitting_empty_stop(self):
        post_id = "abcdef12-3456-7890-abcd-ef1234567890"
        fake_asset_payload = {
            "assets": [
                {
                    "assetId": post_id,
                    "key": f"/users/demo/generated/{post_id}/generated_video.mp4",
                    "mimeType": "video/mp4",
                    "previewImageKey": "/thumbs/fallback-attachment.jpg",
                }
            ],
            "nextPageToken": "",
        }

        processor = VideoStreamProcessor(
            "grok-imagine-1.0-video",
            token="good-token",
            show_think=False,
            upscale_on_finish=False,
        )

        with patch(
            "app.services.grok.services.video.get_config",
            side_effect=lambda key, default=None: {
                "video.stream_timeout": 0,
            }.get(key, default),
        ), patch(
            "app.services.grok.services.video.AssetsListReverse.request",
            AsyncMock(return_value=FakeAssetsResponse(fake_asset_payload)),
        ), patch(
            "app.services.grok.utils.download.DownloadService.render_video",
            AsyncMock(return_value="/v1/files/video/fallback-attachment.mp4"),
        ) as render_video:
            chunks = []
            async for chunk in processor.process(
                stream_response_with_attachment_only(post_id)
            ):
                chunks.append(chunk)

        joined = "\n".join(chunks)
        self.assertIn("/v1/files/video/fallback-attachment.mp4", joined)
        render_video.assert_awaited_once()


if __name__ == "__main__":
    unittest.main()
