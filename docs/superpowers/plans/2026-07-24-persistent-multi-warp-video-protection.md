# Persistent Multi-WARP Video Protection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve independent WARP identities and prevent silently returned fast stock videos from being archived or retried across accounts.

**Architecture:** Keep the existing SQLite-backed egress and media-job flow. WARP containers receive independent persistent `/var/lib/cloudflare-warp` volumes. The Web video adapter classifies a playable result completed in under ten seconds as an egress-level soft fallback, quarantines only the selected egress node, and returns a non-retry-safe error; the gateway therefore fails the current job without switching accounts or spending another quota.

**Tech Stack:** Go, SQLite repository, Docker Compose, Cloudflare WARP container.

---

### Task 1: Lock fast-fallback behavior with failing tests

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/protocol_test.go`
- Modify: `gateway/backend/internal/infra/egress/manager_test.go`
- Modify: `gateway/backend/internal/application/gateway/video_test.go`

- [ ] **Step 1: Test direct and recovered fast results are classified as terminal, account-neutral errors.**

  Add tests that call the video adapter's fast-result guard with a playable result and an elapsed time below ten seconds, asserting HTTP 502, `MediaJobRetrySafe == false`, `AccountHealthNeutral == true`, and no archive call.

- [ ] **Step 2: Test the guard is also used for a recovered result.**

  Use the same guard with source labels `stream` and `recovered`; both must reject before `ArchiveVideo`.

- [ ] **Step 3: Test egress quarantine persists cooldown and invalidates the cached client.**

  Use the existing mutable egress repository and cached client test fixtures. Assert `LastError`, nonzero `CooldownUntil`, reduced health, one repository update, and client eviction.

- [ ] **Step 4: Test gateway retry planning does not defer or switch accounts for the terminal error.**

  Assert the new error's HTTP status is not retry-safe and `videoRetryPlan` returns no retry.

- [ ] **Step 5: Run the focused tests and confirm they fail for the missing guard/quarantine behavior.**

  Run:

  ```bash
  go test ./internal/infra/provider/web ./internal/infra/egress ./internal/application/gateway
  ```

  Expected: FAIL because the new error and quarantine method do not exist yet.

### Task 2: Implement terminal fast-fallback protection

**Files:**
- Modify: `gateway/backend/internal/infra/provider/web/video.go`
- Modify: `gateway/backend/internal/infra/egress/manager.go`
- Modify: `gateway/backend/internal/infra/provider/provider.go` only if a shared error helper is required

- [ ] **Step 1: Add a typed terminal video fallback error.**

  Give it a sanitized message, HTTP 502, `MediaJobRetrySafe() == false`, and `AccountHealthNeutral() == true`. Do not expose the upstream URL or prompt.

- [ ] **Step 2: Add `QuarantineForScope` to the egress manager.**

  For a proxied node, persist a bounded cooldown, reduce health, store a safe reason, invalidate the cached client, and invalidate the node snapshot. For direct egress, only invalidate the direct client; never write a fake node record.

- [ ] **Step 3: Add one guard after direct stream completion and after URL recovery.**

  Measure from immediately before the video request through any result recovery. If a playable URL completes in less than ten seconds, quarantine the lease's Web node, log a bounded diagnostic, and return the terminal error before `ArchiveVideo`.

- [ ] **Step 4: Keep extension behavior consistent.**

  Apply the same guard to extension results without changing extension payloads or replacing the original video.

- [ ] **Step 5: Run the focused tests and confirm they pass.**

  Run:

  ```bash
  go test ./internal/infra/provider/web ./internal/infra/egress ./internal/application/gateway
  ```

### Task 3: Persist three WARP identities

**Files:**
- Modify: `docker-compose.yml`
- Modify: `docker-compose.multi-warp.yml`

- [ ] **Step 1: Mount an independent named volume at `/var/lib/cloudflare-warp` for `warp`, `warp2`, and `warp3`.**

  Keep the existing `warp` service name and add only two additional exits; do not add Redis, change `xianyuandaxian`, or remove the current WARP state.

- [ ] **Step 2: Make the multi-WARP override define only `warp2` and `warp3` with independent volumes.**

  Remove unused `warp4` from the override so the deployment has three total exits.

- [ ] **Step 3: Validate Compose configuration without stopping production.**

  Run `docker compose config` and inspect the rendered volume mounts.

### Task 4: Verify and deploy safely

**Files:**
- Modify: none

- [ ] **Step 1: Run all Go checks.**

  ```bash
  go test ./...
  go vet ./...
  go build ./cmd/grok2api
  ```

- [ ] **Step 2: Commit and push the verified changes.**

  ```bash
  git add docker-compose.yml docker-compose.multi-warp.yml gateway/backend/internal/infra/provider/web/video.go gateway/backend/internal/infra/egress/manager.go gateway/backend/internal/infra/provider/web/protocol_test.go gateway/backend/internal/infra/egress/manager_test.go gateway/backend/internal/application/gateway/video_test.go docs/superpowers/plans/2026-07-24-persistent-multi-warp-video-protection.md
  git commit -m "fix: quarantine fast video fallback and persist warp exits"
  git push origin main
  ```

- [ ] **Step 3: Preserve the current production WARP registration before creating volumes.**

  On `ssh netcup`, copy the existing `/var/lib/cloudflare-warp` state into the new `warp` volume, then create only `warp2` and `warp3`; never run `docker compose down` and never remove the current WARP container before state is preserved.

- [ ] **Step 4: Add two egress nodes through the supported admin API/repository path.**

  Configure distinct proxy URLs for `warp2` and `warp3`, leave account bindings unset so existing affinity distributes accounts, and verify three distinct egress IPs.

- [ ] **Step 5: Deploy only `grok2api_go` with `--no-deps`.**

  Preserve SQLite/media volumes and the existing WARP services. Verify `/healthz`, container health, and the production commit.

- [ ] **Step 6: Run at most one controlled video task per exit.**

  Stop immediately after the first normal result that takes at least ten seconds and has matching media; do not automatically retry a rejected fast fallback and do not exceed three generation attempts total.
