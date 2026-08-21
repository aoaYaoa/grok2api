# Public Video Console Routing Design

## Goal

Make the React `/video` workspace use the explicit `Console/grok-imagine-video` route for normal image-to-video and text-to-video creation, while preserving its cache, batch, history, and extension controls. Keep the NSFW workspace on its existing Web route until its payload and safety semantics are migrated separately.

## Constraints

- Do not modify `xianyuandaxian`.
- Do not add Redis or change existing storage volumes.
- Preserve the React public pages, media cache, SQLite data, account data, and WARP egress configuration.
- Do not silently change or remove the timeline extension UI.

## Design

The legacy public video endpoint will accept an optional `provider` field with `grok_web` or `grok_console`. Omitting it keeps the current Web behavior for existing callers, including NSFW. The `/video` React page sends `grok_console` for normal generation and keeps `grok_web` for the existing timeline-based extension path, because the Web extension protocol carries the current `post_id` and selected start time while the Console extension contract only accepts a source video, prompt, and duration.

The backend maps `grok_console` to the namespaced model `Console/grok-imagine-video` before model resolution. This prevents the generic `grok-imagine-video` alias from selecting Web based on available routes. The task queue, account selection, result persistence, SSE polling, cache listing, and batch loop remain shared and unchanged.

Console reference-to-video limits are reflected in the `/video` controls: at most seven reference images and at most ten seconds when reference images are present. Prompt-only Console generation continues to allow fifteen seconds. NSFW retains its existing eight-reference and Web duration behavior.

## Data Flow

1. `/video` submits `/v1/public/video/start` with `provider: "grok_console"` for generation.
2. The legacy handler resolves the request to `Console/grok-imagine-video` and creates the normal persistent media job.
3. The worker selects only Console accounts for that namespaced route and calls `/v1/videos/generations` through the existing Console adapter.
4. The existing public SSE endpoint reports the persistent job status and local media URL exactly as before.
5. Timeline extension requests from `/video` explicitly retain the Web route, preserving the existing `post_id`, selected start time, and stitching behavior.

## Error Handling

- Unknown `provider` values return a public 400 response without creating a job.
- Console-incompatible reference count or duration is rejected before task creation with the existing parameter error path.
- Console account exhaustion and upstream failures use the existing media-job retry and public error handling.

## Testing

- Go handler test proves an explicit Console request creates a `Console/grok-imagine-video` job while default requests remain Web-compatible.
- Frontend contract test proves `/video` selects Console for generation, Web for timeline extension, and exposes Console-compatible reference/duration limits.
- Run targeted tests, full Go tests, frontend tests, and both frontend builds before deployment.
