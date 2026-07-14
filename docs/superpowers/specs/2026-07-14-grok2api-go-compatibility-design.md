# Grok2API Go Replacement Design

**Date:** 2026-07-14

## Objective

Replace the Python runtime with the latest `chenyme/grok2api` Go backend while preserving every customized HTML/CSS/JS page and the workflows behind those pages. Go is the only application backend in the final image. The existing production deployment remains unchanged until a Go-only canary passes automated, browser, persistence, and real-account generation checks.

## Non-Negotiable Constraints

- Do not modify or deploy `grok2api-xianyudaxian`.
- Preserve `/login`, `/chat`, `/imagine`, `/imagine-workbench`, `/video`, `/nsfw`, `/voice`, `/admin/login`, `/admin/token`, `/admin/config`, and `/admin/cache`.
- Preserve the page behavior behind those URLs, including SSE progress, image editing, parent-post reuse, NSFW generation, video extension and chaining, rename, cache access, and quota display.
- The final runtime contains Go only. Python may be read as migration reference but is not built, started, proxied, or deployed.
- Use SQLite plus in-memory runtime state. Do not add Redis.
- Never commit SSO credentials, browser cookies, encryption keys, databases, or production configuration.

## Source Layout

- `gateway/` is the reviewed snapshot of upstream commit `dd6624c` plus bounded compatibility changes.
- `app/static/` remains the source of the customized legacy pages and assets.
- The Go build copies those assets and serves them directly.
- New compatibility handlers live in the Go transport layer and call Go application services rather than invoking Python or shelling out to a Python process.

## Runtime Architecture

The final stack has one application service:

1. `grok2api_go` serves the upstream React management app, standard APIs, customized pages, legacy-compatible APIs, media files, health checks, and PWA assets.
2. WARP remains the egress proxy where required.
3. FlareSolverr remains optional and is not duplicated.

An edge proxy is unnecessary for route ownership once Go serves every route. If Nginx remains for production TLS or buffering behavior, it has only one upstream: Go.

## Route Ownership

| Route | Go behavior |
| --- | --- |
| `/`, public page routes, `/static/*`, `/manifest.webmanifest`, `/sw.js`, `/favicon.ico` | Serve preserved customized assets with asset-version replacement and no-store page headers |
| `/admin/*` | Serve preserved admin pages and legacy-compatible admin APIs |
| `/gateway/*` | Serve the upstream React management application |
| `/v1/*` | Serve standard upstream APIs plus exact legacy compatibility routes used by preserved pages |
| `/api/admin/v1/*` | Serve upstream Go management APIs |
| `/v1/files/*` | Serve persisted image, video, and audio files safely |
| `/health`, `/healthz`, `/readyz` | Report the Go-only runtime and storage readiness |

## Compatibility Strategy

The browser pages remain unchanged unless a small client adjustment is required to match a stronger Go contract. Each page request is classified into one of three groups:

1. Direct mapping to an existing Go handler, such as models, chat completions, image generation/editing, and asynchronous video generation.
2. A thin compatibility handler that translates the legacy request/response shape to an existing Go application service.
3. A missing custom workflow that must be ported to Go, such as legacy SSE orchestration, video chaining/extension, rename, cache operations, prompt enhancement, or voice token handling.

Compatibility handlers must not proxy to Python. Accepted generation work has one owner and one task ID so retries do not duplicate quota consumption.

## Accounts And Session Longevity

The five Web SSO accounts are imported directly into Go SQLite at deployment time. There is no Python peer database and no synchronization protocol.

- Store encrypted Web SSO credentials in SQLite.
- Keep persistent per-account browser/cookie state under the Go data volume when browser-assisted refresh is used.
- Merge Cloudflare cookies without overwriting SSO cookies.
- Classify authentication, Cloudflare, quota, moderation, timeout, and network failures separately.
- Cool down only the affected account or capability when possible.
- Mark truly revoked credentials as requiring re-login instead of repeatedly returning a generic `401`.
- Browser logout, password change, or upstream security revocation still requires importing a fresh credential.

## Quota Presentation

The preserved `/admin/token` page remains the compact account view and uses Go management data.

- Normalize scalar and object quota responses.
- Show Basic, Super, Heavy, image, and video values independently when available.
- Show `未返回` for missing upstream values instead of inventing allowance, printing objects, or producing `NaN`.
- Aggregate enabled accounts only.
- Keep the upstream `/gateway/accounts` page for detailed account, billing, model capability, and concurrency management.

## Error Handling

Legacy and standard APIs map failures to a common taxonomy: `auth_invalid`, `permission_denied`, `quota_exhausted`, `account_cooling`, `egress_blocked`, `moderated`, `generation_timeout`, and `upstream_protocol_error`.

Streaming endpoints keep SSE heartbeats. Timeouts preserve accepted task IDs for later polling. Logs redact credentials, cookies, authorization headers, and secret-bearing request bodies.

## Storage And Configuration

- Go uses SQLite and memory; Redis is absent from configuration and Compose.
- Media, browser profiles, SQLite, logs, and production config use persistent server-side paths.
- The Go image contains the upstream React build and preserved `app/static` assets.
- Generated JWT, encryption, admin, and account secrets remain outside Git.

## Verification

Required checks before deployment:

- Go unit and contract tests for every compatibility route.
- Frontend lint/build and router base-path tests.
- Static page/API inventory tests that fail when a page starts calling an unregistered endpoint.
- Go-only Docker build, Compose validation, health, restart, persistence, and explicit no-Python/no-Redis assertions.
- Desktop and mobile browser checks for every preserved page and upstream management page.
- Five-account import, quota display, and restart persistence.
- Real Chat, Image, Image Edit, Video, NSFW, parent-post, video-extension, and voice checks where the account supports them.

Historical Python test failures are reference information only; the final acceptance suite runs against Go and the preserved browser assets.

## Deployment And Rollback

1. Complete and verify the Go-only stack in the isolated integration worktree and local canary port.
2. Push the verified commit without credentials or generated data.
3. Back up `/root/grok2api`, current production configuration, tokens, browser profiles, logs, and media.
4. Start the Go-only server canary on a separate port and import the five current tokens into Go SQLite.
5. Verify pages, quotas, persistence, Chat, Image, and Video against the server canary.
6. Switch the public upstream only after the canary passes.
7. Keep commit `9e1e808` and its data snapshot available for rollback. Do not perform destructive database downgrade operations.

## Acceptance Criteria

- Go is the only application runtime in the deployed stack.
- No customized page or required workflow is removed or left with broken controls.
- The new upstream dashboard and management pages work below `/gateway`.
- SQLite and memory are used with no Redis dependency.
- The five Web SSO accounts persist across restarts and expose understandable health/quota state.
- Real server Chat, Image, and Video generation pass before cutover.
- `grok2api-xianyudaxian` remains untouched.
