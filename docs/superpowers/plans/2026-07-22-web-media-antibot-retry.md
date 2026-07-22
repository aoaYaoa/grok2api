# Grok Web Media Anti-Bot Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add bounded stream-level anti-bot recovery to Grok Web image editing and video generation.

**Architecture:** Introduce a shared streaming JSON POST helper beside the existing media HTTP helper. It reuses `preflightUpstream`, preserves prefetched stream bytes, refreshes signed Statsig once, and converts a repeated rejection into the existing sanitized 403 response. WebSocket media errors share classification and feedback without replaying accepted generation work.

**Tech Stack:** Go 1.26, `net/http`, existing Grok Web provider adapters, standard `testing` package.

---

### Task 1: Streaming POST Anti-Bot Recovery

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/protocol_test.go`
- Modify: `gateway/backend/internal/infra/provider/web/image.go`

- [ ] **Step 1: Write failing tests**

Add tests that serve an anti-bot first response followed by a valid stream, and two consecutive anti-bot responses. Assert request count, returned status, and preserved first valid frame.

- [ ] **Step 2: Verify the tests fail**

Run: `go test ./internal/infra/provider/web -run 'TestPostStreamingJSON' -count=1`

Expected: FAIL because `postStreamingJSONWithReferer` does not exist.

- [ ] **Step 3: Implement the shared helper**

Split one-request construction from the existing HTTP-status retry loop, then add `postStreamingJSONWithReferer`. Use `preflightUpstream` for 2xx bodies, invalidate Statsig and retry once for `errWebAntiBot`, and return `antiBotProviderResponse` after the final rejection.

- [ ] **Step 4: Integrate image editing**

Replace the image-edit conversation POST with the streaming helper. Preserve streaming and non-streaming body ownership.

- [ ] **Step 5: Verify focused tests pass**

Run: `go test ./internal/infra/provider/web -run 'TestPostStreamingJSON|TestImageEdit' -count=1`

Expected: PASS.

### Task 2: Video and WebSocket Classification

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/protocol_test.go`
- Modify: `gateway/backend/internal/infra/provider/web/video.go`
- Modify: `gateway/backend/internal/infra/provider/web/image.go`

- [ ] **Step 1: Write failing classifier tests**

Add one video stream test and one Imagine WebSocket payload test using `{"error":{"message":"Request rejected by anti-bot rules.","code":7}}`.

- [ ] **Step 2: Verify the tests fail**

Run: `go test ./internal/infra/provider/web -run 'TestVideoStreamClassifiesAntiBot|TestImagineWebSocketClassifiesAntiBot' -count=1`

Expected: FAIL because the current paths return generic media errors.

- [ ] **Step 3: Implement shared classification**

Route video stream error objects and Imagine WebSocket error objects through the package anti-bot classifier while retaining bounded sanitized messages for unrelated failures.

- [ ] **Step 4: Integrate video streaming POST**

Use `postStreamingJSONWithReferer` for video conversation requests so stream-level anti-bot rejection refreshes Statsig before moderation and recovery logic run.

- [ ] **Step 5: Verify the provider package**

Run: `go test ./internal/infra/provider/web -count=1`

Expected: PASS.

### Task 3: Full Verification and Deployment

**Files:**
- No additional source files.

- [ ] **Step 1: Format and inspect**

Run: `gofmt -w internal/infra/provider/web/image.go internal/infra/provider/web/video.go internal/infra/provider/web/protocol_test.go`

Run: `git diff --check`

- [ ] **Step 2: Run backend verification**

Run: `go test ./...`

Run: `go vet ./...`

Run: `go build ./cmd/grok2api`

- [ ] **Step 3: Commit and push**

Commit the implementation and push `HEAD` to `origin/main`.

- [ ] **Step 4: Deploy only `grok2api_go`**

Fetch `origin/main` in `/root/grok2api-go-release`, switch the detached checkout, rebuild only `grok2api_go`, and force-recreate that service without adding Redis or modifying WARP.

- [ ] **Step 5: Verify production**

Confirm `/healthz` returns 200, `grok2api_go` and WARP are healthy, the data volume remains mounted, and the recent logs contain no startup errors.

