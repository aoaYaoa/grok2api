# Grok2API Go Replacement Implementation Plan

> **Execution:** Continue in the isolated `codex/integrate-chenyme-go-20260714` worktree. Use test-driven development for compatibility behavior and verification-before-completion before push or deployment.

**Goal:** Make the latest Go gateway the sole backend while preserving the customized pages and their complete workflows, then verify locally and deploy a server canary without Redis.

**Architecture:** Vendor upstream below `gateway/`; copy `app/static` into the Go image; serve legacy pages and upstream React from Go; implement legacy browser/admin contracts as Go transport adapters over Go services and SQLite.

## Task 1: Correct The Architecture And Inventory Contracts

- [x] Vendor upstream commit `dd6624c` below `gateway/` and record upstream metadata.
- [x] Add and verify the upstream React `/gateway` base path.
- [x] Replace dual-backend design/plan text with this Go-only architecture.
- [x] Inventory every legacy `fetch`, `EventSource`, WebSocket, file, PWA, and authentication route.
- [x] Add a machine-checked route inventory so an unimplemented page endpoint is visible during tests.

## Task 2: Host Preserved Pages In Go

- [x] Write failing tests for public/admin page routes, `/static/*`, PWA assets, asset-version replacement, and no-store page headers.
- [x] Add a Go legacy-page host using copied `app/static` assets.
- [x] Serve `/`, all public/admin pages, `/manifest.webmanifest`, `/sw.js`, and `/favicon.ico`.
- [x] Copy `app/static` in the backend Docker build and verify both legacy pages and `/gateway` assets render in route tests.

## Task 3: Port Authentication, Models, Chat, And Image Workflows

- [x] Implement `/v1/public/verify`, `/v1/admin/verify`, and legacy storage/auth helpers over Go config.
- [x] Map legacy public models and `/v1/public/chat/completions` to existing Go inference services with legacy response/stream behavior.
- [ ] Port imagine config/start/stop/SSE/WebSocket, edit, parent-post, workbench edit, and prompt enhancement contracts.
- [ ] Add contract tests derived from the preserved JavaScript request and response handling.

## Task 4: Port Video, NSFW, Voice, Files, And Cache

- [ ] Port video start/stop/SSE/cache-list/rename with existing upstream async video generation.
- [ ] Port custom long-duration chaining, extension, parent-post continuation, and restart-safe task state.
- [ ] Port NSFW image/video routing using the existing Go Web provider capability.
- [ ] Port voice token handling or document a verified direct Go provider mapping.
- [ ] Serve and manage persisted `/v1/files/*` media safely.
- [ ] Port admin cache list/clear/delete/online-load/online-clear/video-rename behavior.

## Task 5: Port Token, Quota, Config, And Batch Admin APIs

- [ ] Map legacy token import/update/delete/enable/refresh endpoints to Go account services and SQLite.
- [ ] Normalize quota output for the preserved Token page, including `未返回` for absent values.
- [ ] Port config, model-routing metadata, Cloudflare refresh, Statsig, log clearing, and storage APIs.
- [ ] Port batch task SSE/cancel behavior used by admin pages.
- [ ] Verify the five SSO tokens survive container restart without Redis or an active browser login session.

## Task 6: Make Compose And Images Go-Only

- [x] Remove `grok2api_python` and all Python upstream routing from Compose/Nginx.
- [x] Publish one Go application endpoint on an isolated canary port.
- [x] Use SQLite, memory, persistent media paths, WARP, and optional FlareSolverr only.
- [x] Add static assertions that Compose contains no Python service and no Redis service.
- [ ] Build and start the local Go-only stack without disturbing existing `grok2api` or `grok2api_xianyudaxian` containers.

## Task 7: Local Acceptance

- [ ] Run all Go tests, vet, build, frontend lint/build, Node compatibility tests, and Compose validation.
- [ ] Browser-check every preserved public/admin page and every upstream `/gateway` page on desktop and mobile.
- [ ] Import five local SSO accounts, verify quota/account health, restart, and verify persistence.
- [ ] Run real Chat, Image, Image Edit, Video, NSFW, parent-post, video-extension, and supported voice checks.
- [ ] Keep the task incomplete if any preserved control still calls an unimplemented route.

## Task 8: Push And Deploy

- [ ] Write the Go-only operations/rollback guide.
- [ ] Commit intentional changes and verify a clean worktree.
- [ ] Merge to local `main` without touching unrelated changes or `grok2api-xianyudaxian`.
- [ ] Push the verified commit.
- [ ] Back up netcup production and start a Go-only canary on a separate port.
- [ ] Import the five server tokens into Go SQLite and verify pages, quotas, persistence, Chat, Image, and Video.
- [ ] Cut over only after server canary success; retain rollback to `9e1e808` and its data snapshot.
