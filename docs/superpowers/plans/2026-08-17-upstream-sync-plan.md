# Upstream v3.1.2 Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronize `chenyme/grok2api` through `f42ba1765fa520c6a587387daf8b22612168b397` while preserving the local React public workspaces, media cache, video extension/recovery behavior, SQLite deployment, and multi-WARP routing.

**Architecture:** Apply the cumulative upstream tree delta from the recorded marker `5b0c5cd3` into the vendored `gateway/` prefix with Git three-way patching. Resolve conflicts by subsystem against the final upstream tree, then reapply local public-workspace and media guarantees where upstream behavior differs.

**Tech Stack:** Go 1.26, React 19, TypeScript 6, Vite 8, pnpm 11, SQLite, Docker Compose.

## Global Constraints

- Do not modify `xianyuandaxian`.
- Do not add Redis or change the SQLite runtime.
- Preserve local public Chat, Imagine, image edit, Video, NSFW, Voice, cache, media playback, video extension, five-minute retry, and cross-account task behavior.
- Preserve Docker volumes, media files, tasks, credentials, and all project WARP containers during deployment.
- Import upstream commit `f42ba1765fa520c6a587387daf8b22612168b397` exactly and record it in `gateway/UPSTREAM.md`.

---

### Task 1: Apply the cumulative upstream delta

**Files:**
- Modify: `gateway/` files changed by `git diff 5b0c5cd3 f42ba1765fa520c6a587387daf8b22612168b397`
- Modify: `gateway/UPSTREAM.md`

**Interfaces:**
- Consumes: upstream marker `5b0c5cd3` and target `f42ba1765fa520c6a587387daf8b22612168b397`
- Produces: a working tree containing the final upstream delta under `gateway/`

- [ ] **Step 1: Generate the full-index binary patch**

Run:

```bash
git diff --binary --full-index 5b0c5cd3 f42ba1765fa520c6a587387daf8b22612168b397 > /tmp/grok2api-upstream-20260817.patch
```

- [ ] **Step 2: Apply it with three-way conflict support**

Run:

```bash
git apply --3way --directory=gateway /tmp/grok2api-upstream-20260817.patch
```

- [ ] **Step 3: List and classify every conflict**

Run:

```bash
git diff --name-only --diff-filter=U
```

Expected: conflicts are limited to files changed both locally and upstream; all added upstream files are present under `gateway/`.

### Task 2: Resolve backend reliability and media conflicts

**Files:**
- Modify: `gateway/backend/internal/application/gateway/*.go`
- Modify: `gateway/backend/internal/infra/provider/web/*.go`
- Modify: `gateway/backend/internal/infra/provider/console/*.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/*.go`
- Test: matching `*_test.go` files in the same packages

**Interfaces:**
- Consumes: upstream Imagine protocol, Basic-tier media, Console admission, clearance, audit, and cooldown changes
- Produces: merged backend behavior retaining local media persistence and retry contracts

- [ ] **Step 1: Resolve application gateway conflicts against upstream tests**
- [ ] **Step 2: Preserve local Web video 429 five-minute deferral and stored-media playback**
- [ ] **Step 3: Integrate current `mediaGenInput`, Basic image edit/720p video, Console limits, and clearance reacquisition**
- [ ] **Step 4: Run focused package tests**

Run:

```bash
go test ./internal/application/gateway ./internal/infra/provider/web ./internal/infra/provider/console ./internal/infra/persistence/relational
```

Expected: PASS with zero failures.

### Task 3: Resolve egress, audit, CLI, and management UI conflicts

**Files:**
- Modify: `gateway/backend/internal/application/egress/**`
- Modify: `gateway/backend/internal/pkg/tunnelproxy/**`
- Modify: `gateway/backend/internal/infra/provider/cli/**`
- Modify: `gateway/frontend/src/features/accounts/**`
- Modify: `gateway/frontend/src/features/audits/**`
- Modify: `gateway/frontend/src/features/quality-guard/**`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

**Interfaces:**
- Consumes: upstream Clash/VLESS, probe profile, reasoning summary, audit accounting, and quota presentation changes
- Produces: updated admin features without replacing local public page routes or styling

- [ ] **Step 1: Integrate egress subscription and VLESS Reality support**
- [ ] **Step 2: Integrate probe profiles and quality-guard recovery**
- [ ] **Step 3: Integrate CLI reasoning and media audit accounting**
- [ ] **Step 4: Preserve local public frontend files and verify admin-only UI changes**
- [ ] **Step 5: Run frontend lint and build**

Run:

```bash
pnpm lint
pnpm build
```

Expected: both commands exit 0; admin and public bundles are generated.

### Task 4: Record, verify, integrate, and deploy

**Files:**
- Modify: `gateway/UPSTREAM.md`
- Add: `docs/superpowers/plans/2026-08-17-upstream-sync-plan.md`

**Interfaces:**
- Consumes: merged and tested source tree
- Produces: pushed `main` and a healthy production `grok2api_go` container

- [ ] **Step 1: Update upstream metadata**

Set `Commit` to `f42ba1765fa520c6a587387daf8b22612168b397` and `Imported` to `2026-08-17`.

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
go build ./cmd/grok2api
pnpm lint
pnpm build
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 3: Commit and push the synchronized branch to `main`**

Run:

```bash
git add -A
git commit -m "sync upstream f42ba176"
git push origin HEAD:main
```

- [ ] **Step 4: Rebuild only the application service in production**

Run on `netcup` from `/root/grok2api-go-release`:

```bash
git fetch origin
git checkout --detach origin/main
docker compose -f docker-compose.yml -f docker-compose.multi-warp.yml build grok2api_go
docker compose -f docker-compose.yml -f docker-compose.multi-warp.yml up -d --force-recreate --no-deps grok2api_go
```

- [ ] **Step 5: Verify production health and retained WARP services**

Run:

```bash
curl -fsS http://127.0.0.1:8000/healthz
docker ps --filter name=grok2api-go-release --format '{{.Names}}\t{{.Status}}'
```

Expected: application and all three project WARP containers report healthy.
