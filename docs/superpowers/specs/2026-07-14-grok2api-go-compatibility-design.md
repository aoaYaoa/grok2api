# Grok2API Go Compatibility Integration Design

**Date:** 2026-07-14

## Objective

Integrate the latest `chenyme/grok2api` Go gateway without replacing or degrading the customized Python application. The existing public pages, admin pages, quota presentation, media workflows, browser-session behavior, and compatibility APIs remain available at their current URLs. The new Go dashboard, account pool, model routes, client keys, audits, documentation, settings, and standard APIs are added as real working capabilities.

## Non-Negotiable Constraints

- Do not modify or deploy `grok2api-xianyudaxian`.
- Do not replace the existing `/chat`, `/imagine`, `/imagine-workbench`, `/video`, `/nsfw`, `/voice`, `/admin/token`, `/admin/config`, or `/admin/cache` pages.
- Preserve the backend behavior required by those pages, including parent-post reuse, multi-reference image editing, NSFW routing, video extension, video rename, long-duration chaining, SSE progress, cache access, and file serving.
- Deploy a single-instance stack with SQLite and an in-memory runtime store. Do not add Redis.
- Keep the current production version available until the replacement passes local and server-side real-generation checks.
- Never commit SSO credentials, browser cookies, encryption keys, databases, or production configuration.

## Chosen Architecture

The repository will contain two application runtimes behind one edge proxy:

1. `grok2api-python` remains the compatibility and custom-workflow application.
2. `grok2api-go` contains a vendored snapshot of the current `chenyme/main` Go backend and React frontend.
3. `grok2api-edge` is the only externally published HTTP service and routes requests to the correct runtime.
4. The existing WARP service remains the shared egress proxy. FlareSolverr remains optional and is not duplicated.

The Go source is stored below a dedicated `gateway/` directory so future upstream updates can be reviewed as a bounded subtree. The existing Python files stay at their current paths.

## Route Ownership

The edge proxy applies the following stable ownership rules:

| Route | Owner | Purpose |
| --- | --- | --- |
| `/`, `/login`, `/chat`, `/imagine`, `/imagine-workbench`, `/video`, `/nsfw`, `/voice` | Python | Existing public experience |
| `/admin`, `/admin/login`, `/admin/token`, `/admin/config`, `/admin/cache` | Python | Existing optimized administration |
| `/v1/public/*`, `/v1/admin/*`, `/v1/files/*` | Python | Existing page and management APIs |
| Existing custom video, NSFW, parent-post, cache, media, and compatibility paths | Python | Features not represented by the upstream Go API |
| `/gateway/*` | Go frontend | New dashboard and management pages |
| `/api/admin/v1/*` | Go backend | New management API |
| `/gateway/v1/*` | Go backend with `/gateway` stripped | New standard gateway API without taking over the existing `/v1` contract |
| `/health` | Python | Existing health contract |
| `/gateway/healthz`, `/gateway/readyz` | Go | New gateway health contracts |

The Go React router and asset base are configured for `/gateway`. Navigation links from the existing admin header open the new dashboard, accounts, models, client keys, audits, docs, and settings pages without removing the current Token, Config, and Cache entries.

## Request Capability Routing

The first production release does not redirect the existing external `/v1/*` endpoints to Go. This avoids changing working clients before parity is proven.

The compatibility layer classifies requests explicitly:

- Standard Chat Completions, Responses, Messages, image generation, image editing, and asynchronous video generation can be exercised through `/gateway/v1/*`.
- Parent-post image continuation, outpainting workflows, multi-step workbench behavior, NSFW generation, current video SSE, long-duration chaining, video extension, rename, voice, local cache, and current file URLs stay on Python.
- A request accepted by one runtime is never replayed automatically to the other runtime. This prevents duplicate generation and quota consumption.
- A later release may selectively route standard existing `/v1` requests to Go only after contract tests and real production checks prove compatibility.

## Account Synchronization

Web SSO credentials must be imported once and remain usable by both runtimes. Each runtime keeps its native storage, while a private synchronization contract keeps account mutations aligned.

- Python remains capable of using Web SSO credentials for custom media workflows and Browser Bridge.
- Go stores its encrypted account copy in SQLite for the new account pool and standard APIs.
- A shared `GATEWAY_SYNC_SECRET` authenticates container-internal synchronization endpoints. These endpoints are not routed by the public edge proxy.
- Account identity is mapped with a stable credential fingerprint and provider, not by exposing plaintext credentials in logs or mapping files.
- Import, enable, disable, metadata update, and delete operations enqueue an idempotent sync operation to the peer runtime.
- Sync operations include an origin marker so peer updates do not loop back.
- Failed sync operations remain pending and are retried with bounded backoff. The UI shows local state, peer state, last successful sync time, and the latest failure reason.
- Existing production tokens are migrated at deployment time from the server data volume into Go SQLite through the private import path. No secret material enters Git.
- Grok Build OAuth accounts remain Go-only because the Python custom workflows do not consume them and Go can refresh them natively.

## Session Longevity

Each Web SSO account gets an isolated persistent browser profile and cookie state below the data volume. Profiles must survive application and container restarts.

- Cloudflare cookie refresh merges only Cloudflare-related cookie fields and must not replace SSO cookies.
- Health and quota probes use bounded intervals and per-account cooldowns.
- Authentication, quota, Cloudflare, moderation, network, and timeout failures are classified separately.
- A capability failure disables or cools down only the affected capability where possible; it does not immediately disable every use of the account.
- Invalid accounts leave the active pool and show a clear re-login requirement instead of repeatedly producing generic `401` errors.
- Explicit logout, password changes, or upstream security revocation cannot be reversed by the application. These cases require a new SSO credential.

## Quota Presentation

The existing `/admin/token` page remains the primary compact token view and keeps its current styling and controls.

- Normalize scalar and object quota responses before rendering.
- Show Basic, Super, Heavy, image, and video capability values independently when available.
- Display `未返回` for missing upstream values instead of inventing allowance, rendering objects, or producing `NaN`.
- Aggregate only enabled accounts.
- Display Go peer sync state and last successful quota refresh without turning the page into the new React account screen.
- The Go account page remains available for detailed account, billing, model capability, and concurrency management.

## Error Handling

Both runtimes expose a common user-facing error taxonomy:

- `auth_invalid`: upstream credential revoked or invalid.
- `permission_denied`: account tier or capability restriction.
- `quota_exhausted`: quota unavailable.
- `account_cooling`: temporary cooldown or concurrency gate.
- `egress_blocked`: proxy, Cloudflare, or network path failure.
- `moderated`: upstream content moderation rejection.
- `generation_timeout`: accepted work did not complete before its deadline.
- `upstream_protocol_error`: malformed or incompatible upstream response.

Streaming endpoints retain SSE keepalive events. Accepted asynchronous work preserves its task ID on timeout so the client can continue polling. Diagnostic logs redact credentials, cookies, authorization headers, and request bodies that may contain secrets.

## Storage and Configuration

- Python continues using its current local data and log directories.
- Go uses SQLite at a persistent path and the memory runtime store.
- Go media uses a persistent local directory.
- Both services receive the same WARP SOCKS address where required.
- Generated JWT secrets, credential encryption keys, bootstrap admin credentials, sync secrets, databases, tokens, and browser profiles are supplied by server-side environment/config files excluded from Git.
- Docker Compose health checks gate the edge proxy and deployment verification.

## Test Strategy

The repository starts from a historically non-green Python baseline: some committed tests describe functionality removed or changed by commit `1538fe2`, while the current production service is operational for its exercised paths. This integration must not claim a clean baseline without distinguishing these historical failures.

Required automated checks:

- Existing route, page asset, quota rendering, Browser Bridge, image, video, NSFW, and compatibility tests relevant to preserved behavior.
- New edge route tests proving old URLs stay on Python and `/gateway` routes reach Go.
- Go backend `go test ./...`, `go vet ./...`, and binary build.
- React lint and production build with `/gateway` base-path assertions.
- Account sync tests for import, update, delete, idempotency, origin-loop prevention, retry, and redaction.
- Contract tests for Chat, image generation/edit, and asynchronous video APIs through `/gateway/v1`.
- Docker health, persistence, restart, and no-Redis checks.

Required browser and real-account checks:

- Desktop and mobile screenshots of every preserved public page and the modified admin navigation.
- Existing Token, Config, and Cache operations.
- New dashboard, accounts, models, keys, audits, docs, and settings pages.
- Five-account status and quota synchronization.
- Real Chat, Image, Image Edit, Video, NSFW, parent-post, and video-extension requests through the intended runtime.
- Container restart followed by account, cookie, page, and media persistence checks.

## Deployment and Rollback

1. Implement and test on an isolated branch and local ports.
2. Build immutable Python, Go, frontend, and edge images.
3. Push the verified commit to the configured Git repository.
4. Back up `/root/grok2api`, production configuration, token data, browser profiles, SQLite, and media before server changes.
5. Start the new stack on non-production ports using copied production secrets and data.
6. Run health checks and real Chat, Image, and Video generation against the canary stack.
7. Switch the server edge route only after canary success.
8. Verify public pages, admin pages, new gateway pages, APIs, logs, and persistence after the switch.
9. Keep the old container and commit `9e1e808` available until the observation window completes.
10. Roll back by restoring the previous edge target and data snapshot; do not attempt destructive in-place database downgrades.

## Acceptance Criteria

- No existing page or customized workflow is removed.
- New upstream management pages are available below `/gateway` and backed by the Go service.
- The Go service uses SQLite and memory, with no Redis dependency.
- Existing public and management URLs preserve their behavior.
- Five Web SSO accounts have understandable health, quota, and synchronization status.
- Real Chat, Image, and Video requests succeed on the server after deployment.
- Account or capability failures no longer surface only as generic `401` messages.
- The deployment has a tested rollback to commit `9e1e808`.
