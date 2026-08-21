# Public Video Console Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route normal React `/video` generation explicitly to Console while preserving the existing public video workflow and NSFW/Web compatibility.

**Architecture:** Add an optional provider selector to the existing legacy public video start contract. The React `/video` page selects Console for generation and Web for its existing timeline extension path; the backend converts the selector into a namespaced model before shared route resolution. The task queue, persistence, SSE, caching, and account scheduling stay shared.

**Tech Stack:** Go 1.26, Gin, existing gateway/provider adapters, React 19, TypeScript, Node test runner, pnpm.

## Global Constraints

- Do not modify `xianyuandaxian`.
- Do not add Redis or change existing storage volumes.
- Preserve React public pages, media cache, SQLite data, account data, and WARP egress configuration.
- Keep NSFW on its current Web route.
- Keep the existing timeline extension UI and payload behavior.

---

### Task 1: Lock the provider contract with failing tests

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/video_test.go`
- Modify: `gateway/frontend/test/public-app-contract.test.mjs`

**Interfaces:**
- Consumes: Existing `/v1/public/video/start` handler and public React source files.
- Produces: Regression assertions for explicit Console routing and frontend provider selection.

- [ ] **Step 1: Add the Go failing test**

Add a handler test that posts `provider: "grok_console"` and asserts the captured `gateway.VideoInput.PublicModel` is `Console/grok-imagine-video`, while the request still carries its prompt, duration, resolution, and reference input.

- [ ] **Step 2: Add the frontend failing test**

Add a contract assertion that `video-page.tsx` selects `grok_console` for normal generation, `grok_web` for timeline extension, limits the Console reference selection to seven images, and exposes ten seconds as the maximum reference-video duration.

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
cd gateway/backend && go test ./internal/transport/http/legacy -run 'TestVideoStartUsesExplicitConsoleModel' -count=1
cd ../frontend && pnpm test -- public-app-contract.test.mjs
```

Expected: both new assertions fail because the request has no provider field and the page has no Console selection/limit logic.

### Task 2: Implement explicit backend route selection

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/video.go`
- Test: `gateway/backend/internal/transport/http/legacy/video_test.go`

**Interfaces:**
- Consumes: `videoStartRequest`, `gateway.VideoInput`, and the provider namespace conventions in `domain/model`.
- Produces: Optional `provider` request field and a namespaced `PublicModel` for Console requests.

- [ ] **Step 1: Add the provider field and validation**

Add `Provider string json:"provider"` to `videoStartRequest`. Normalize an empty value to `grok_web`; accept only `grok_web` and `grok_console`; return HTTP 400 for any other value.

- [ ] **Step 2: Map the provider to the public model**

Use `Console/grok-imagine-video` when the normalized provider is `grok_console`, otherwise keep `grok-imagine-video`. Pass the selected model into every job created by the existing concurrency loop. Set normal generation explicitly to `provider.VideoOperationGenerate` without changing extension request mapping.

- [ ] **Step 3: Run the focused Go test and verify GREEN**

Run:

```bash
cd gateway/backend && go test ./internal/transport/http/legacy -run 'TestVideoStart(UsesExplicitConsoleModel|KeepsWebModel|Selects)' -count=1
```

Expected: the new Console test and existing Web compatibility tests pass.

### Task 3: Adapt the React `/video` workspace

**Files:**
- Modify: `gateway/frontend/src/public/features/video/video-api.ts`
- Modify: `gateway/frontend/src/public/pages/video-page.tsx`
- Test: `gateway/frontend/test/public-app-contract.test.mjs`

**Interfaces:**
- Consumes: The backend `provider` field and Console video limits.
- Produces: Normal `/video` requests with `provider: "grok_console"`; timeline extension requests with `provider: "grok_web"`; reference-aware limits in the existing controls.

- [ ] **Step 1: Extend the public request type**

Add an optional `provider?: "grok_web" | "grok_console"` field to `VideoStartBody`.

- [ ] **Step 2: Select the provider by operation**

In the shared `generate` request builder, set `provider` to `grok_web` when `bodyOverride.is_video_extension` is true and to `grok_console` otherwise. Keep the existing reference arrays, batch count, cache source data, and extension metadata unchanged.

- [ ] **Step 3: Apply Console reference limits without changing NSFW**

In `VideoPage`, cap selected references at seven and use `6`/`10` second options when references are present; retain `6`/`10`/`15` for prompt-only generation. Normalize an existing `15` second selection to `10` when the first reference is added. Keep NSFW untouched.

- [ ] **Step 4: Run the focused frontend test and verify GREEN**

Run:

```bash
cd gateway/frontend && pnpm test -- public-app-contract.test.mjs
```

Expected: the provider and Console-limit assertions pass without changing existing cache, SSE, or extension assertions.

### Task 4: Verify, build, and integrate

**Files:**
- Modify only the files listed in Tasks 1-3 plus the committed design and plan documents.

**Interfaces:**
- Consumes: Completed backend and frontend changes.
- Produces: Tested commit ready for push and deployment.

- [ ] **Step 1: Run backend tests**

Run `cd gateway/backend && go test ./... -count=1` and require exit code 0.

- [ ] **Step 2: Run frontend tests and builds**

Run `cd gateway/frontend && pnpm test && pnpm build`, requiring exit code 0 for both.

- [ ] **Step 3: Review the diff and commit**

Run `git diff --check`, inspect `git diff --stat`, confirm no `xianyuandaxian`, Redis configuration, or unrelated generated files changed, then commit with:

```bash
git add docs/superpowers/specs/2026-08-22-public-video-console-routing-design.md docs/superpowers/plans/2026-08-22-public-video-console-routing-plan.md gateway/backend/internal/transport/http/legacy/video.go gateway/backend/internal/transport/http/legacy/video_test.go gateway/frontend/src/public/features/video/video-api.ts gateway/frontend/src/public/pages/video-page.tsx gateway/frontend/test/public-app-contract.test.mjs
git commit -m "fix: route public video generation through console"
```

- [ ] **Step 4: Push and deploy only the gateway service**

Push the current branch, update `/root/grok2api-go-release` on `netcup`, rebuild only `grok2api_go`, preserve WARP containers, SQLite volumes, cache files, and `seed/`, then verify the container health and logs show `provider: "grok_console"` for a new `/video` generation request.
