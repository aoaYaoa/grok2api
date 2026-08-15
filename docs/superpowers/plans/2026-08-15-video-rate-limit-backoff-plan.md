# Video Rate-Limit Backoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop Web video 429 responses from rapidly cycling through the entire account pool and defer the job for recovery.

**Architecture:** The Web media adapter marks recognizable rate-limit responses as request-scoped. The gateway's existing durable media-job lease mechanism defers those jobs for five minutes, releasing the account lease while retaining the job and billing reservation for the recovery worker. Other account-specific failures continue through the existing failover path.

**Tech Stack:** Go, `net/http`, SQLite media-job repository, existing provider error interfaces, Go tests.

## Global Constraints

- Preserve cache image resolution and reference-image request construction.
- Do not add Redis or alter WARP/OpenClaw services.
- Do not modify `xianyuandaxian`.
- Do not delete tasks, media, SQLite data, volumes, or caches.
- Keep public errors sanitized and avoid logging upstream response bodies or credentials.

### Task 1: Add the failing provider classification test

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/video_test.go`
- Modify: `gateway/backend/internal/infra/provider/web/video.go`

**Interfaces:**
- Consumes: `newWebMediaUpstreamError`, `provider.IsRequestScopedError`.
- Produces: `RequestScopedFailure() bool` on the Web media upstream error for rate-limit responses.

- [ ] **Step 1: Write the failing test**

Add a test that creates a Web media upstream error from `{"code":8,"message":"Too many requests"}` with status 429 and asserts `provider.IsRequestScopedError(err)` is true. Add a neighboring test for a generic 429 body that remains account-failover eligible unless the provider positively identifies rate limiting.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./gateway/backend/internal/infra/provider/web -run 'TestWebMedia.*RequestScoped|TestWebMedia.*RateLimit' -count=1
```

Expected: the new rate-limit test fails because `webMediaUpstreamError` does not yet implement `RequestScopedFailure()`.

- [ ] **Step 3: Implement the minimal provider classification**

Add `RequestScopedFailure() bool` to `webMediaUpstreamError`. Return true only for HTTP 429 when the bounded sanitized summary contains `too many requests`, `rate limit`, or `rate limited`. Do not retain or inspect raw body data outside construction.

- [ ] **Step 4: Run the focused test and verify it passes**

Run the same focused command. Expected: PASS.

- [ ] **Step 5: Commit the provider change**

```bash
git add gateway/backend/internal/infra/provider/web/video.go gateway/backend/internal/infra/provider/web/video_test.go
git commit -m "fix(video): classify shared web rate limits"
```

### Task 2: Add the failing gateway deferral test

**Files:**
- Modify: `gateway/backend/internal/application/gateway/video_test.go`
- Modify: `gateway/backend/internal/application/gateway/video.go`

**Interfaces:**
- Consumes: `provider.IsRequestScopedError`, `Service.deferVideoJobFor`, existing media-job lease persistence.
- Produces: a 429 branch that defers the current job and releases its account lease without selecting another account.

- [ ] **Step 1: Write the failing test**

Extend the existing `videoCreateFailoverAdapter` fixture with a request-scoped 429 error option or use a provider error implementing both `HTTPStatusCode() int` and `RequestScopedFailure() bool`. Create a job with two eligible accounts, run it once, and assert:

```go
attempts := adapter.Attempts()
if len(attempts) != 1 || attempts[0] != first.ID { t.Fatalf(...) }
stored.Status == media.StatusInProgress
stored.LeaseUntil != nil && stored.LeaseUntil.After(time.Now().UTC())
```

Also assert the job is not terminally failed and the second account was not attempted.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./gateway/backend/internal/application/gateway -run 'TestVideo.*RateLimit|TestVideo.*429' -count=1
```

Expected: FAIL because the current runner marks the first 429 as retryable create failure and selects another account.

- [ ] **Step 3: Implement the minimal gateway behavior**

In `runVideoJob`, after the provider returns an error and before account-health mutation/failover, detect `provider.IsRequestScopedError(err)` with HTTP 429. Release the active lease, call `s.deferVideoJobFor(parent, job, 5*time.Minute)`, log a structured deferral event with the retry delay, and return. Leave the existing behavior for other request-scoped errors and account-specific statuses unchanged.

- [ ] **Step 4: Run the focused test and verify it passes**

Run the same focused command. Expected: PASS, with the existing forbidden failover test still passing.

- [ ] **Step 5: Commit the gateway change**

```bash
git add gateway/backend/internal/application/gateway/video.go gateway/backend/internal/application/gateway/video_test.go
git commit -m "fix(video): defer shared rate limits before account failover"
```

### Task 3: Full verification and deployment

**Files:**
- No additional source files.

- [ ] **Step 1: Run focused and full verification**

```bash
go test ./gateway/backend/internal/infra/provider/web ./gateway/backend/internal/application/gateway -count=1
go test ./...
go build ./cmd/grok2api
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Review the diff and commit history**

Confirm only the provider classification, gateway deferral, focused tests, and required design/plan documents changed. Confirm no WARP or unrelated project files changed.

- [ ] **Step 3: Deploy the Go service**

Use the existing production deployment procedure for `/root/grok2api-go-release`, preserving SQLite/media volumes and leaving WARP containers untouched.

- [ ] **Step 4: Verify production health**

Run:

```bash
ssh netcup 'curl -fsS http://127.0.0.1:8000/healthz'
ssh netcup 'docker ps --format "{{.Names}}\t{{.Status}}"'
```

Expected: `{"ok":true}`, `grok2api_go` healthy, and existing WARP containers unchanged.

- [ ] **Step 5: Run the cache-image regression manually**

From the Video page, choose one cached image, add it as the reference, submit one task, and inspect logs. Expected: the cache image returns 200, the video request includes a post ID, and a 429 causes one attempt followed by a five-minute deferral rather than rapid account cycling.
