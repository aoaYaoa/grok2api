# Video Rate-Limit Backoff Design

## Goal

Prevent Web image-to-video jobs from exhausting the entire account pool when the upstream returns a shared `429 Too many requests` response.

## Current Failure

The cache image is resolved successfully and the Web provider creates a video post candidate. When the upstream rejects the request with HTTP 429, the gateway treats the create-stage response as safe account failover. A large video attempt budget then rapidly tries many accounts through the WARP exits. Because the limit is shared by the upstream service, account switching amplifies the rejection and makes every account appear unavailable.

## Design

1. Classify Web media HTTP 429 responses whose sanitized message indicates rate limiting as request-scoped. This classification preserves the upstream status and does not expose response bodies or credentials.
2. In the video job runner, defer request-scoped 429 jobs for the existing five-minute recovery window instead of marking the account failed or switching accounts immediately.
3. Release the current account lease before returning from the deferral path. Keep the billing reservation and persisted job in progress so the recovery worker can retry it.
4. Keep account failover unchanged for credential rejection, account-specific quota exhaustion, bot protection, and other retry-safe create failures.
5. Add regression tests proving that a 429 performs one upstream attempt, leaves the job in progress with a future lease, and does not consume the next account. Existing forbidden failover coverage must remain unchanged.

## Non-Goals

- No changes to cache image resolution or reference-image payload construction.
- No changes to WARP topology, Redis, OpenClaw, or `xianyuandaxian`.
- No public error text changes for successful or ordinary failed jobs.

## Verification

- Run the focused gateway and provider tests.
- Run `go test ./...`, `go build ./cmd/grok2api`, and `git diff --check`.
- Deploy the Go service without rebuilding or deleting WARP containers.
- Confirm health, container status, and a cache-image video request's logs show a single 429 followed by deferral rather than rapid account cycling.
