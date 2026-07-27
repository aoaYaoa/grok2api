# Upstream 5131698 Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate `chenyme/grok2api` changes from `d2a8b4f7` through `5131698b` into the customized `gateway/` tree while preserving local React public workspaces, media caching, cross-account media recovery, SQLite/memory deployment, and multi-WARP routing.

**Architecture:** Generate a path-mapped three-way patch for upstream `backend/` and `frontend/` sources and apply it beneath `gateway/`. Resolve overlapping gateway, account-management, and i18n changes by combining upstream linked-account, model-alias, routing-attempt, and failure-classification behavior with the local quota recovery, account eligibility, public media, and deployment contracts.

**Tech Stack:** Go 1.26, React 19, TypeScript 6, Vite 8, SQLite, Docker Compose, WARP

---

### Task 1: Apply The Upstream Delta

**Files:**
- Modify: `gateway/backend/**`
- Modify: `gateway/frontend/src/features/**`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

- [ ] **Step 1: Generate the exact upstream patch**

Run:

```bash
git diff --binary --output=/tmp/grok2api-upstream-5131698.patch d2a8b4f7 5131698b -- VERSION config.example.yaml backend frontend
```

Expected: the patch contains only the 58 upstream application paths changed after the recorded import.

- [ ] **Step 2: Apply the patch below `gateway/`**

Run:

```bash
git apply --3way --index --directory=gateway /tmp/grok2api-upstream-5131698.patch
```

Expected: non-overlapping files are staged and overlapping customized files are left as explicit conflicts.

- [ ] **Step 3: Protect local-only surfaces**

Run:

```bash
git diff --name-only -- docker-compose.yml docker-compose.multi-warp.yml gateway/frontend/src/public gateway/backend/internal/infra/provider/web/video.go
```

Expected: no changes to deployment topology, public React workspaces, or the customized web video provider.

### Task 2: Integrate Backend Reliability And Routing

**Files:**
- Modify: `gateway/backend/internal/application/account/**`
- Modify: `gateway/backend/internal/application/gateway/**`
- Modify: `gateway/backend/internal/domain/model/reasoning.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/**`
- Modify: `gateway/backend/internal/infra/provider/**`
- Modify: `gateway/backend/internal/infra/runtime/**`
- Modify: `gateway/backend/internal/transport/http/**`

- [ ] **Step 1: Merge linked-account management**

Integrate association filters, preview-before-delete, optional linked-provider deletion, and skip-on-media safeguards. Keep local account quota visibility, unavailable-account filtering, media ownership, and SQLite schema upgrades.

- [ ] **Step 2: Merge routing and failure classification**

Integrate unlimited routing attempts and Build safety/quota classification. Preserve model-route isolation, upstream reset timestamps, `Retry-After`, billing-period recovery, paid/free quota differences, sticky WARP affinity, and cross-account media inheritance.

- [ ] **Step 3: Merge reasoning aliases**

Integrate client-key-gated thinking-level aliases, reasoning effort mapping, Codex model listing, and Console/CLI normalization without changing public media model mappings.

- [ ] **Step 4: Run focused backend tests**

Run:

```bash
go test ./internal/application/account ./internal/application/clientkey ./internal/application/gateway ./internal/infra/persistence/relational ./internal/infra/provider/... ./internal/infra/runtime/... ./internal/transport/http/account ./internal/transport/http/clientkey ./internal/transport/http/inference
```

Expected: all selected packages pass.

### Task 3: Integrate Management UI

**Files:**
- Modify: `gateway/frontend/src/features/accounts/accounts-api.ts`
- Modify: `gateway/frontend/src/features/accounts/accounts-page.tsx`
- Modify: `gateway/frontend/src/features/client-keys/**`
- Modify: `gateway/frontend/src/features/settings/**`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

- [ ] **Step 1: Add linked-account controls**

Integrate association filters, deletion preview, linked-provider cleanup options, and media-blocked explanations using the existing account table, theme tokens, responsive dialogs, and local quota/status presentation.

- [ ] **Step 2: Add routing and alias settings**

Expose unlimited attempts and reasoning-effort aliases through existing client-key and settings controls without replacing customized navigation or public workspaces.

- [ ] **Step 3: Verify public React contracts**

Run:

```bash
node --test test/public-app-contract.test.mjs
```

Expected: all 20 public route, media, cache, layout, and theme contracts pass.

### Task 4: Record And Verify

**Files:**
- Modify: `gateway/UPSTREAM.md`

- [ ] **Step 1: Record the imported revision**

Set the upstream commit to `5131698b` and import date to `2026-07-27`, summarizing linked-account management, unlimited routing attempts, Build failure classification, and reasoning aliases.

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

Confirm the server Git SHA, application and WARP health, `/healthz`, public `/video` response, SQLite/memory/local-media startup topology, quota refresh state, and absence of startup errors. Do not run paid image or video generation.
