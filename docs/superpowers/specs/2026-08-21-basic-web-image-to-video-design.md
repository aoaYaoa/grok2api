# Basic Web Image-to-Video Design

## Goal

Restore image-to-video and reference-image video generation for Basic Grok Web accounts without changing text-to-video, Console media, cache storage, video recovery, or account inheritance behavior.

## Confirmed State

- Basic Web accounts still have image and 720p video quota; production currently records positive media allowance for 222 of 227 Basic accounts.
- Text-to-video through `Web/grok-imagine-video` still creates an upstream job.
- Upstream commit `d6f6e9f5` disabled every Web request containing `image` or `reference_images`, even though the Web adapter still contains the proven upload and image-reference payload builders used by successful jobs before that sync.
- Console media quota is nearly exhausted, so excluding Web leaves image-to-video with no schedulable account.

## Design

1. Re-enable Web video routes for image input at the gateway admission layer. Existing provider, client-key, model, tier, quota, cooldown, and retry filters remain unchanged.
2. Restore Web reference preparation before video creation. Local cache references, data URLs, and HTTPS images continue through `prepareVideoReference`; each image must resolve to both an upstream attachment ID and image URI.
3. Keep the current `mediaGenInput.textToVideo` payload only for requests without images. Image requests use the previously validated Web image/reference payload:
   - one image: image-to-video message, attachment ID, image URI, and `videoGenModelConfig`;
   - multiple images: reference-to-video configuration with `imageReferences` and the existing reference count limit.
4. Never fall back from image-to-video to text-to-video. Missing upload IDs, image URIs, or rejected references fail before video quota is consumed.
5. Preserve existing account switching. Create-stage retryable failures may move to another Basic Web account; polling and recovery remain pinned to the account that created the upstream post.

## Error Handling

- Invalid, unreadable, oversized, or unsupported references fail during preparation.
- A prepared reference missing its attachment ID or URI is rejected as a local preparation error.
- Structured upstream policy failures retain their current status and retry classification.
- Empty-result recovery remains bounded by the existing five-minute policy.

## Verification

- Unit tests prove Web route admission accepts image and reference input.
- Provider tests prove text requests retain `mediaGenInput.textToVideo`, while one-image and multi-image requests use image/reference payloads and never include `textToVideo`.
- Legacy public-page tests prove a selected cached image remains attached to the created video request.
- Full Go tests, production build, health check, and one live Basic Web cached-image task verify the end-to-end path.

## Scope

- No Redis or deployment topology changes.
- No database, account, quota, cache, media, WARP, or OpenClaw migration.
- No changes to unrelated public-page layout or React behavior.
