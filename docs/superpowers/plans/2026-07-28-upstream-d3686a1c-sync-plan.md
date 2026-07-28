# Upstream d3686a1c Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate `chenyme/grok2api` changes from `5131698b` through `d3686a1c` into the customized `gateway/` tree while preserving local React public workspaces, media caches, cross-account recovery, SQLite/memory deployment, and multi-WARP routing.

**Architecture:** Apply the exact upstream delta as a path-mapped three-way patch beneath `gateway/`. Resolve overlaps by combining upstream v3.0.10 account export, egress, selector, failure-health, timing, and billing behavior with the local media ownership, quota recovery, public workspace, and deployment contracts.

**Tech Stack:** Go 1.26, React 19, TypeScript 6, Vite 8, SQLite, Docker Compose, WARP

---

### Task 1: Apply The Upstream Delta

**Files:**
- Modify: `gateway/VERSION`
- Modify: `gateway/backend/**`
- Modify: `gateway/frontend/src/features/**`
- Modify: `gateway/frontend/src/shared/**`

- [ ] **Step 1: Generate the exact upstream patch**

Run:

```bash
git diff --binary --output=/tmp/grok2api-upstream-d3686a1c.patch 5131698b d3686a1c -- VERSION config.example.yaml backend frontend
```

Expected: the patch contains only upstream application files changed after the recorded `5131698b` import.

- [ ] **Step 2: Apply the patch below `gateway/`**

Run:

```bash
git apply --3way --index --directory=gateway /tmp/grok2api-upstream-d3686a1c.patch
```

Expected: non-overlapping changes are staged and customized overlaps remain as explicit conflicts.

- [ ] **Step 3: Protect local-only surfaces**

Run:

```bash
git diff --name-only -- docker-compose.yml docker-compose.multi-warp.yml gateway/frontend/src/public gateway/backend/internal/infra/provider/web/video.go
```

Expected: no changes to deployment topology, public React workspaces, or the customized Grok Web video provider.

### Task 2: Integrate Backend Reliability And Routing

**Files:**
- Modify: `gateway/backend/internal/application/account/**`
- Modify: `gateway/backend/internal/application/egress/**`
- Modify: `gateway/backend/internal/application/gateway/**`
- Modify: `gateway/backend/internal/domain/**`
- Modify: `gateway/backend/internal/infra/egress/**`
- Modify: `gateway/backend/internal/infra/persistence/relational/**`
- Modify: `gateway/backend/internal/infra/runtime/**`
- Modify: `gateway/backend/internal/transport/http/**`

- [ ] **Step 1: Merge account export and deferred hydration**

Integrate selective/cursor-based credential export and deferred credential hydration. Preserve local linked-account cleanup, quota/status display, media-job protection, and SQLite compatibility.

- [ ] **Step 2: Merge egress dual-stack management**

Integrate IPv4/IPv6 probe separation, selectable probe providers, cancellation-aware cooling, clearance handling, proxy cleanup, and widened clearance storage. Preserve sticky WARP affinity and the existing multi-WARP deployment topology.

- [ ] **Step 3: Merge selector and failure-health updates**

Integrate layered selection diagnostics, reduced database I/O, `MarkFailureAfterSuccess`, response timing, and unlimited routing compatibility. Preserve paid/free quota recovery, five-minute media recovery, cross-account image/video inheritance, and request stop conditions.

- [ ] **Step 4: Merge performance and billing details**

Integrate response performance metrics, request audit timing fields, and billing breakdowns without changing existing media accounting or public inference response contracts.

- [ ] **Step 5: Run focused backend tests**

Run:

```bash
go test ./internal/application/account ./internal/application/egress ./internal/application/gateway ./internal/domain/audit ./internal/infra/egress ./internal/infra/persistence/relational ./internal/infra/runtime/... ./internal/transport/http/account ./internal/transport/http/audit ./internal/transport/http/dashboard ./internal/transport/http/egress ./internal/transport/http/inference
```

Expected: all selected packages pass.

### Task 3: Integrate Management UI

**Files:**
- Modify: `gateway/frontend/src/features/accounts/**`
- Modify: `gateway/frontend/src/features/audits/**`
- Modify: `gateway/frontend/src/features/dashboard/**`
- Modify: `gateway/frontend/src/features/settings/**`
- Modify: `gateway/frontend/src/shared/api/client.ts`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

- [ ] **Step 1: Add export and audit controls**

Integrate selectable account export, cursor pagination, response timing, and billing breakdown presentation using the existing account table, dialogs, theme tokens, and responsive layout.

- [ ] **Step 2: Add dual-stack egress controls**

Integrate probe-family and provider controls, updated operations, and error feedback without replacing the customized navigation, public workspaces, or local WARP settings.

- [ ] **Step 3: Verify public React contracts**

Run:

```bash
node --test test/public-app-contract.test.mjs
```

Expected: all public route, media, cache, layout, and theme contracts pass.

### Task 4: Record And Verify

**Files:**
- Modify: `gateway/UPSTREAM.md`

- [ ] **Step 1: Record the imported revision**

Set the upstream commit to `d3686a1cfb63038a594baafb5170dcadffacf126`, import date to `2026-07-28`, and summarize v3.0.10, dual-stack egress, account export, selector performance, failure-health, timing, and billing changes.

- [ ] **Step 2: Run complete backend verification**

Run from `gateway/backend`:

```bash
go test ./...
go vet ./...
go build ./cmd/grok2api
```

Expected: every command exits successfully.

- [ ] **Step 3: Run complete frontend verification**

Run from `gateway/frontend`:

```bash
pnpm lint
pnpm build
node --test test/public-app-contract.test.mjs
```

Expected: lint reports no errors, both bundles build, and all public contracts pass.

- [ ] **Step 4: Validate deployment and diff hygiene**

Run:

```bash
docker compose -f docker-compose.yml -f docker-compose.multi-warp.yml config --quiet
git diff --check
git diff --cached --check
```

Expected: Compose renders and both diff checks are clean.

### Task 5: Publish And Deploy

**Files:**
- No additional source files.

- [ ] **Step 1: Commit, merge, and push**

Commit the isolated branch, fast-forward local `main`, rerun representative tests on merged `main`, and push `origin/main`.

- [ ] **Step 2: Deploy only `grok2api_go`**

Fetch the exact pushed SHA in `/root/grok2api-go-release`, rebuild `grok2api_go`, and recreate only that service. Do not run `docker compose down`, add Redis, recreate WARP, or modify SQLite/media volumes.

- [ ] **Step 3: Verify production**

Confirm the server Git SHA, application and WARP health, `/healthz`, public `/video` response, SQLite/memory/local-media startup topology, and absence of startup errors. Do not run paid image or video generation.
