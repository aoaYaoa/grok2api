# Video Request Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align Go video requests with the working official protocol and prevent moderated or completed video streams from becoming false 99% failures.

**Architecture:** Carry the preset through the existing media-job metadata, build the provider payload from the captured official single-image request shape, and keep stream parsing responsible for moderation and candidate identifiers. Recovery will consume ordered candidate IDs without changing the public SSE contract.

**Tech Stack:** Go, Gin, existing provider/gateway tests, Docker Compose canary.

---

### Task 1: Add failing protocol tests

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/protocol_test.go`
- Modify: `gateway/backend/internal/transport/http/legacy/video_test.go`

- [x] Add tests asserting a single-reference payload keeps `imagine-video-gen`, sends the image post ID in `fileAttachments`, uses the canonical source image URL in `message`, disables side-by-side mode, and omits multi-reference-only fields.
- [x] Add a test asserting `preset` is preserved from `/v1/public/video/start` into `gateway.VideoInput`.
- [x] Add a test asserting moderated stream events do not publish progress and are classified for retry.
- [x] Run the focused tests and confirm they fail against the current implementation.

### Task 2: Carry preset and align the single-image payload

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/video.go`
- Modify: `gateway/backend/internal/application/gateway/video.go`
- Modify: `gateway/backend/internal/infra/provider/provider.go`
- Modify: `gateway/backend/internal/infra/provider/web/video.go`

- [x] Add validated `Preset` fields to the public request, gateway input, persisted metadata, and provider request.
- [x] Add mode mapping for `normal`, `fun`, `spicy`, and `custom`.
- [x] Build the single-reference message and `imagine-video-gen` payload with the official attachment ID, canonical image URL, mode suffix, and side-by-side setting.
- [x] Keep multi-reference fields only when there are multiple references.

### Task 3: Handle moderation and result candidates

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/video.go`
- Modify: `gateway/backend/internal/infra/provider/web/protocol_test.go`

- [x] Detect moderation before invoking the progress callback.
- [x] Retry moderated streams up to five times without exposing a false 99% terminal state.
- [x] Preserve ordered `videoId`, `assetId`, and `videoPostId` candidates and poll them in that order.
- [x] Add focused tests for candidate ordering and successful recovery through a video candidate.

### Task 4: Verify locally

**Files:**
- No source changes expected.

- [x] Run focused provider and transport tests.
- [x] Compile the full Go backend test suite and build; run focused changed-path tests in the host sandbox.
- [x] Rebuild/restart only the local Go canary container, leaving `xianyuandaxian` untouched.
- [x] Run a real neutral-image canary task and verify archived video/poster responses.
