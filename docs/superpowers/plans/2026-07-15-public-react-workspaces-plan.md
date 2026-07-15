# Public React Workspaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the seven public legacy pages with production React workspaces backed completely by the Go gateway.

**Architecture:** Build the existing frontend twice, with separate admin and public entries, and serve both SPAs from Go. Shared typed clients own public authentication, streaming, media conversion, and downloads; route pages compose those clients into the preserved workflows.

**Tech Stack:** Go, Gin, Gorilla WebSocket, React 19, React Router, TanStack Query, Tailwind CSS, Radix UI, Lucide, LiveKit client, Vite, Node test runner.

---

### Task 1: Public build and route contracts

**Files:**
- Create: `gateway/frontend/test/public-app-contract.test.mjs`
- Create: `gateway/frontend/public.html`
- Create: `gateway/frontend/src/public-main.tsx`
- Create: `gateway/frontend/src/public/app/router.tsx`
- Modify: `gateway/frontend/vite.config.ts`
- Modify: `gateway/frontend/package.json`

- [ ] Write contract tests for all seven routes, both Vite entries, root asset paths, and no legacy script tags.
- [ ] Run the focused Node test and confirm it fails because the public entry does not exist.
- [ ] Add the public HTML entry, router, and dual-build scripts.
- [ ] Run the focused test and TypeScript build.

### Task 2: Public authentication and shell

**Files:**
- Create: `gateway/frontend/src/public/auth/public-auth.tsx`
- Create: `gateway/frontend/src/public/api/client.ts`
- Create: `gateway/frontend/src/public/components/public-shell.tsx`
- Create: `gateway/frontend/src/public/pages/login-page.tsx`
- Create: `gateway/frontend/src/public/public.css`

- [ ] Test public-key persistence, bearer headers, unauthorized cleanup, and navigation labels.
- [ ] Implement the auth provider, API error decoder, guarded routes, login form, desktop navigation, mobile navigation, theme toggle, and logout.
- [ ] Verify keyboard focus, 44px touch targets, and mobile overflow behavior.

### Task 3: Streaming and media utilities

**Files:**
- Create: `gateway/frontend/src/public/api/sse.ts`
- Create: `gateway/frontend/src/public/lib/media.ts`
- Create: `gateway/frontend/src/public/hooks/use-persistent-state.ts`
- Create: `gateway/frontend/test/public-stream-contract.test.mjs`

- [ ] Test SSE frame decoding across chunk boundaries, named events, abort cleanup, parent-post parsing, and file conversion.
- [ ] Implement minimal utilities and rerun focused tests.

### Task 4: Chat workspace

**Files:**
- Create: `gateway/frontend/src/public/pages/chat-page.tsx`
- Create: `gateway/frontend/src/public/features/chat/chat-api.ts`
- Create: `gateway/frontend/src/public/features/chat/chat-types.ts`

- [ ] Add tests for model loading, request payload options, stream delta parsing, cancellation, and attachment encoding.
- [ ] Implement conversation history, composer, files, settings, stream rendering, copy, clear, retry, and empty/error states.

### Task 5: Imagine and Workbench

**Files:**
- Create: `gateway/frontend/src/public/features/image/image-api.ts`
- Create: `gateway/frontend/src/public/features/image/image-types.ts`
- Create: `gateway/frontend/src/public/components/image-gallery.tsx`
- Create: `gateway/frontend/src/public/pages/imagine-page.tsx`
- Create: `gateway/frontend/src/public/pages/workbench-page.tsx`

- [ ] Test image start/stop/SSE/edit payloads and parent-post normalization.
- [ ] Implement continuous generation, concurrency, selection/download, lightbox editing/history, reference upload/reorder/paste/drop, and workbench streaming edits.

### Task 6: Video and NSFW workspaces

**Files:**
- Create: `gateway/frontend/src/public/features/video/video-api.ts`
- Create: `gateway/frontend/src/public/features/video/video-types.ts`
- Create: `gateway/frontend/src/public/components/video-gallery.tsx`
- Create: `gateway/frontend/src/public/pages/video-page.tsx`
- Create: `gateway/frontend/src/public/pages/nsfw-page.tsx`

- [ ] Test generation, concurrency, cache list, rename, stop, and continuation payloads.
- [ ] Implement generation controls, references, progress, active workspace, cache selection, rename/download, continuation timeline, and the combined NSFW flow.

### Task 7: Voice Go contract and React workspace

**Files:**
- Create: `gateway/backend/internal/transport/http/legacy/voice.go`
- Create: `gateway/backend/internal/transport/http/legacy/voice_test.go`
- Modify: `gateway/backend/internal/transport/http/legacy/handler.go`
- Create: `gateway/frontend/src/public/pages/voice-page.tsx`
- Create: `gateway/frontend/src/public/features/voice/voice-api.ts`
- Modify: `gateway/frontend/package.json`

- [ ] Add failing Go tests for auth, token response normalization, proxy URL construction, and signal target sanitization.
- [ ] Implement credential-backed LiveKit token retrieval and same-origin WebSocket relay with token-safe logs.
- [ ] Add LiveKit dependency and implement microphone publish, remote audio, fallback connections, diagnostics, and disconnect.

### Task 8: Image WebSocket and video continuation compatibility

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/image.go`
- Modify: `gateway/backend/internal/transport/http/legacy/image_test.go`
- Modify: `gateway/backend/internal/transport/http/legacy/video.go`
- Modify: `gateway/backend/internal/transport/http/legacy/video_test.go`
- Modify: `gateway/backend/internal/application/gateway/video.go`

- [ ] Add failing tests for `/imagine/ws` task streaming and continuation requests.
- [ ] Adapt image task events to WebSocket and map continuation inputs into the gateway media job.
- [ ] Run focused race tests.

### Task 9: Go SPA serving and container assets

**Files:**
- Create: `gateway/backend/internal/transport/http/public_frontend.go`
- Create: `gateway/backend/internal/transport/http/public_frontend_test.go`
- Modify: `gateway/backend/internal/transport/http/server.go`
- Modify: `gateway/backend/internal/transport/http/legacy.go`
- Modify: `gateway/backend/internal/infra/config/config.go`
- Modify: `gateway/backend/internal/app/application.go`
- Modify: `gateway/Dockerfile`
- Modify: `gateway/docker/entrypoint.sh`

- [ ] Add failing route tests proving public routes return the React index and `/static/public/js/*.js` is no longer required.
- [ ] Register the public SPA before the admin SPA, copy both build outputs, retain cache/PWA assets only where required, and keep legacy admin redirects.
- [ ] Run Go route tests and Docker build.

### Task 10: Verification and deployment

- [ ] Run all frontend Node tests, lint, TypeScript builds, `go test ./...`, and focused race tests.
- [ ] Start a local Docker canary and verify every public route plus real Chat, Image, and Video requests.
- [ ] Use Playwright desktop and mobile screenshots to check overflow, media framing, dialogs, and loading states.
- [ ] Commit the migration, push the branch, deploy `/root/grok2api-go-release`, preserve existing cache/data and rollback container, and verify `runtimeStore.driver: memory` with no grok2api Redis.
- [ ] Re-run production route, authentication, Chat, Image, and Video smoke tests through `https://grok.uonoe.com`.
