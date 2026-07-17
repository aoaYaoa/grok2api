# Video Request Alignment Design

## Goal

Make Go video generation send the same request shape as the working official-style implementation, especially for single-image-to-video and NSFW preset requests, so completed jobs are not reported as instant 99% failures.

## Root Cause

The Go path currently drops `preset`, omits the single-image attachment ID and canonical image URL from the message, enables side-by-side mode, and treats every reference list as multi-reference video. A live Chrome Network capture of a successful official request confirms the current video model remains `imagine-video-gen`; Grok 4.5 is not the video request model. The official single-image payload sends `fileAttachments` with the image post ID, starts `message` with the canonical asset URL, sets `enableSideBySide=false`, and uses `parentPostId` without the multi-reference-only flags. Moderated stream events are also ignored instead of retried.

## Design

1. Pass `preset` through the public request, gateway input, persisted video metadata, and provider request.
2. Keep `modelName=imagine-video-gen` and build the single-reference payload from the captured official request: attachment post ID in `fileAttachments`, canonical source image URL in `message`, `enableSideBySide=false`, and a normal `videoGenModelConfig` containing `parentPostId` and `isVideoEdit=false`.
3. Keep the existing multi-reference path isolated; do not change its public API contract in this fix.
4. Detect moderated stream events before publishing their progress, retry up to five times with the same prepared source post, and return a terminal upstream error only after the retry budget is exhausted.
5. Preserve all candidate IDs from the stream for result recovery, preferring video/asset IDs over a parent post ID.

## Verification

- Unit tests cover preset propagation, single-reference payload fields, mode selection, moderated events, and candidate ID ordering.
- Existing Go provider and gateway tests must remain green.
- Local canary logs must show a normal video stream progressing beyond the immediate terminal event, with no recovery loop against an image parent post.
