# Basic Web Image-to-Video Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Basic Grok Web image-to-video and reference-image video generation while keeping text-to-video and existing recovery behavior unchanged.

**Architecture:** The gateway admits Web image inputs again, then the Web adapter prepares image references and selects one of two payload builders: current `textToVideo` for text-only requests, or the proven attachment/config payload for image requests. Local validation prevents image requests from silently degrading into text requests.

**Tech Stack:** Go 1.26, Gin, SQLite/GORM, Grok Web adapter, Docker Compose.

## Global Constraints

- Do not add Redis or change the SQLite deployment.
- Do not recreate, restart, or modify WARP services.
- Do not touch OpenClaw, unrelated services, account credentials, quotas, media files, caches, or database volumes.
- Preserve five-minute result recovery and cross-account create-stage retry behavior.
- Deploy only `grok2api_go` and never run Compose `down`.

---

### Task 1: Admit Web image video routes

**Files:**
- Modify: `gateway/backend/internal/application/gateway/video.go`
- Test: `gateway/backend/internal/application/gateway/video_test.go`

**Interfaces:**
- Consumes: `validateVideoRouteParameters(provider, operation, upstreamModel, resolution, hasImage, referenceCount, duration)`.
- Produces: Web routes remain compatible for image and reference input; existing resolution and Console limits remain intact.

- [ ] **Step 1: Write the failing tests**

Add assertions that `validateVideoRouteParameters` returns nil for Basic Web generate requests with `hasImage=true` and with `referenceCount=1` at 720p.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/application/gateway -run 'TestVideoWebImageInputAdmission' -count=1`

Expected: FAIL with `Grok Web 当前仅支持文本生视频`.

- [ ] **Step 3: Implement the minimal admission change**

Remove only the blanket Web image/reference rejection from `validateVideoRouteParameters`. Keep every Console limit and 1080p restriction unchanged.

- [ ] **Step 4: Run focused gateway tests and verify GREEN**

Run: `go test ./internal/application/gateway -run 'TestVideo(WebImageInputAdmission|1080pValidationUsesResolvedUpstreamModel)' -count=1`

Expected: PASS.

### Task 2: Restore Web image payload generation

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/video.go`
- Test: `gateway/backend/internal/infra/provider/web/protocol_test.go`

**Interfaces:**
- Consumes: `prepareVideoReference`, `createMediaPost`, `videoCreatePayload`, `provider.VideoRequest`.
- Produces: text-only payloads with `mediaGenInput.textToVideo`; image payloads with uploaded attachments, image URIs, and `videoGenModelConfig` without `textToVideo`.

- [ ] **Step 1: Write failing payload tests**

Add tests for:

```go
textPayload := videoCreatePayload("move", "16:9", "720p", 6)
singlePayload := videoCreatePayload("move", parentID, "16:9", "720p", 6, []uploadedFile{{ID: "file-1", URI: "https://assets.grok.com/image.png"}}, "custom")
multiPayload := videoCreatePayload("move", parentID, "16:9", "720p", 6, []uploadedFile{{ID: "file-1", URI: "https://assets.grok.com/a.png"}, {ID: "file-2", URI: "https://assets.grok.com/b.png"}}, "custom")
```

Assert text has `mediaGenInput.textToVideo`; single and multi image payloads do not, retain `videoGenModelConfig`, and single image includes `fileAttachments`.

- [ ] **Step 2: Run provider tests and verify RED**

Run: `go test ./internal/infra/provider/web -run 'TestVideoCreatePayload(CurrentText|SingleImage|MultipleReferences)' -count=1`

Expected: FAIL because image payloads still contain `textToVideo` or image generation is rejected before preparation.

- [ ] **Step 3: Restore reference preparation in `GenerateVideo`**

Build `rawReferences` from `ImageURL` and `ReferenceURLs`, prepare each reference, reject any prepared reference missing `ID` or `URI`, create the image media post, and call the image form of `videoCreatePayload`. Text-only requests continue directly to the current payload.

- [ ] **Step 4: Split payload behavior without changing public signatures**

Keep the existing variadic compatibility helper. Add `mediaGenInput.textToVideo` only when `len(references) == 0`; image payloads keep the existing message, attachment, and `videoGenModelConfig` fields.

- [ ] **Step 5: Run provider and gateway tests**

Run: `go test ./internal/infra/provider/web ./internal/application/gateway ./internal/transport/http/legacy -count=1`

Expected: PASS.

### Task 3: Full verification and deployment

**Files:**
- No production file additions.

**Interfaces:**
- Consumes: production Docker Compose stack and existing `legacy-pages` key.
- Produces: a healthy deployment and one live Basic Web image-to-video task created from an existing cached image.

- [ ] **Step 1: Run complete backend verification**

Run: `go test ./... -count=1` from `gateway/backend`, then `git diff --check`.

Expected: all packages PASS and no whitespace errors.

- [ ] **Step 2: Commit and push**

Commit the implementation and tests, then push `HEAD:main`.

- [ ] **Step 3: Deploy only the application**

On `netcup`, fetch `origin/main`, check out the new commit, run `docker compose build grok2api_go`, then `docker compose up -d --no-deps grok2api_go`.

- [ ] **Step 4: Verify persistence and health**

Confirm the container is healthy, `/healthz` returns 200, and `/app/data` is still mounted from `grok2api-go-release_grok2api_gateway_data`.

- [ ] **Step 5: Live cached-image probe**

Use one existing cached image through `/v1/public/video/start`, request a one-second 720p video, verify task creation, inspect logs for selected `grok_web` account and image input, and poll until completion or a classified upstream failure. Do not launch repeated quota-consuming probes.
