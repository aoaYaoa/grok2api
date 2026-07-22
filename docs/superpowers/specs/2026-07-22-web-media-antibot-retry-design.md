# Grok Web Media Anti-Bot Retry Design

## Goal

Make Grok Web image editing and video generation handle stream-level anti-bot errors consistently with Chat, while keeping retries bounded and preventing duplicate media jobs.

## Scope

- Image editing and video generation requests sent to `/rest/app-chat/conversations/new` use a shared streaming POST helper.
- The helper inspects the first upstream stream event before exposing the body to callers.
- A first `code: 7` or `anti-bot` event invalidates the signed Statsig value and retries once.
- A second anti-bot event records `403` feedback for the egress node and returns the existing sanitized anti-bot provider response.
- Imagine WebSocket errors use the same anti-bot classifier and egress feedback, but are not replayed after generation has begun.
- Existing Chat and Lite-image retry behavior remains unchanged.

## Data Flow

1. Build the media request and acquire the existing account-bound egress lease.
2. Send one signed HTTP request.
3. For non-2xx HTTP responses, retain the existing one-time Statsig refresh behavior.
4. For 2xx streaming responses, run `preflightUpstream` before returning the body.
5. Preserve prefetched bytes when the first event is valid.
6. Retry once when the first event is classified as anti-bot.
7. Return a sanitized 403 response and update egress health when the retry is also rejected.

## Error Handling

- Only anti-bot errors are retried by the new helper.
- Context cancellation, transport errors, usage limits, moderation, and malformed streams preserve their existing behavior.
- WebSocket anti-bot frames produce the shared anti-bot error and `403` egress feedback; no automatic replay occurs after a WebSocket generation request is written.

## Tests

- First stream response is anti-bot and the second is valid: two requests, refreshed Statsig, valid stream preserved.
- Both stream responses are anti-bot: two requests, final 403 provider response, egress feedback.
- Video stream error payload with `code: 7` is classified as `errWebAntiBot`.
- Imagine WebSocket error payload with `code: 7` is classified as `errWebAntiBot`.

