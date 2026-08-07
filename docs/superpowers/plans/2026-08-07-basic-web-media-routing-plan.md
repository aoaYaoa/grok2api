# Basic Web Media Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Basic Web video routing and make legacy cached media references work with both Web and Console providers.

**Architecture:** Update the Web catalog as the source of tier capability truth, keep legacy page model selection explicit, and normalize internal image references inside the Console adapter before building upstream payloads. Preserve existing gateway retry and account-rotation behavior.

**Tech Stack:** Go, Gin, GORM/SQLite, React public workspace, Docker Compose.

---

### Task 1: Basic Web video capability

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/catalog.go`
- Test: `gateway/backend/internal/infra/provider/web/protocol_test.go`

- [ ] Add a failing test asserting Basic accounts list and route `grok-imagine-video`.
- [ ] Run the focused Web provider tests and confirm failure.
- [ ] Change the video catalog minimum tier to Basic.
- [ ] Run the focused tests and confirm success.

### Task 2: Legacy Imagine model selection

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/image.go`
- Test: `gateway/backend/internal/transport/http/legacy/image_test.go`

- [ ] Add failing non-Pro and Pro route-selection assertions.
- [ ] Run the legacy transport tests and confirm failure.
- [ ] Select the lite or quality Web model from the Pro flag.
- [ ] Run the focused tests and confirm success.

### Task 3: Console cached image references

**Files:**
- Modify: `gateway/backend/internal/infra/provider/console/media.go`
- Test: `gateway/backend/internal/infra/provider/console/console_test.go`

- [ ] Add failing video and image-edit tests using `grok2api-media://image/<id>`.
- [ ] Run the Console provider tests and confirm failure.
- [ ] Resolve, validate, and encode internal assets as image data URLs.
- [ ] Run the focused tests and confirm success.

### Task 4: Verification and release

- [ ] Run related package tests.
- [ ] Run `go test ./...` from `gateway/backend`.
- [ ] Build the production image.
- [ ] Commit and push `main`.
- [ ] Deploy with the existing server Compose stack.
- [ ] Re-sync models and verify Basic Web video availability plus cached-reference handling.
