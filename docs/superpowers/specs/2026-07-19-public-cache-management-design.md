# Public Cache Management And Reference Picker Design

## Goal

Add permanent multi-select deletion for cached images and videos, reuse cached images as references in Video and NSFW, and remove the obsolete manual parent-post ID feature.

## Unified Cache Model

Every public cache item exposes a stable deletion identity:

- `source`: `legacy`, `mediaAsset`, or `mediaJob`.
- `cacheKey`: legacy filename, media asset ID, or media job ID.
- Existing display URL, poster URL, name, timestamps, post ID, and extension metadata remain unchanged.

The list response remains backward compatible. New fields are additive so older clients can still display cache entries.

## Permanent Deletion

Public image and video cache APIs accept a bounded list of deletion targets. The server validates every source/key pair instead of accepting arbitrary paths.

- Legacy image/video deletion removes the actual cache file. Video metadata is removed when it no longer belongs to another cached file.
- Media asset deletion removes the image object and its database metadata.
- Media job deletion removes only a completed or failed job owned by the authenticated client key. Queued and in-progress jobs cannot be deleted through cache management.
- Missing items are reported as skipped, making repeated deletion requests idempotent.
- The response reports `deleted`, `skipped`, and `failed` counts without exposing filesystem paths or database errors.

Deletion is permanent and requires a confirmation dialog in the UI.

## Cache Picker UI

Image and video cache dialogs share the same interaction pattern:

- Card selection uses an inset checkbox state that remains inside the card bounds.
- The dialog toolbar shows selected count, select-current-page, clear-selection, and delete-selected actions.
- Deletion removes successful items from the visible list without forcing a full page reload.
- Image cards retain preview, edit, download, and use-as-reference actions.
- Video cards retain playback, extension, rename, open, and download actions.
- The picker remains usable on mobile with a sticky compact toolbar and stable card dimensions.

The implementation may use separate image and video dialog components, but selection state, confirmation wording, and deletion result handling use shared helpers.

## Reference Workflows

Video adds a `Cached images` action beside local image upload. The picker supports selecting multiple cached images and appends them to the existing reference list, respecting the eight-image limit and duplicate detection.

NSFW adds a `Cached image` action in the reference-image control group. Selecting an image sets it as the single active reference, clears an existing local upload, closes the picker, and scrolls the operation area into view when necessary.

Workbench continues to support multiple cached references. Imagine and all cache dialogs gain management selection and permanent deletion.

## Manual ID Removal

The visible parent-post ID feature is removed completely:

- Remove the ID input, paste/add button, state, and `addParent` logic from Workbench and Video.
- Remove the frontend `resolveParentPost` function and `parentPost` endpoint contract.
- Remove the public `/v1/public/imagine/parent-post` route, handler, and route tests.
- Remove `extractParentPostID` when no remaining caller exists.

Internal `parentPostID` fields remain because generated results, image edits, cached metadata, and video extension ancestry still use them. Existing edit fallback behavior that derives an official image URL from an internally supplied parent post ID also remains.

Reference acquisition is limited to local upload, pasted image files, generated results, and the visual cache picker.

## Error Handling

- Selection remains intact when deletion fails so the user can retry.
- Partial deletion reports the result once and removes only confirmed successes.
- A media database failure does not prevent legacy cache entries from being listed or deleted when their source is available.
- Unauthorized media-job deletion is reported as skipped/not found rather than revealing another client's task.

## Verification

- Backend tests cover each deletion source, path validation, ownership, active-job rejection, idempotency, partial failure, and metadata cleanup.
- Frontend tests cover selection, confirmation, partial results, Video multi-reference insertion, NSFW single-reference insertion, duplicate/limit handling, and absence of the manual ID UI and endpoint.
- Existing image edit, video generation, video extension, cache playback, lazy loading, and mobile layout contracts continue to pass.

