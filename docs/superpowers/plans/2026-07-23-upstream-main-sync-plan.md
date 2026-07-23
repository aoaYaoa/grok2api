# Upstream Main Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port upstream changes after v3.0.7 into the customized `gateway/` application without replacing its React public workspaces, media cache, Web anti-bot fixes, SQLite runtime, or WARP deployment.

**Architecture:** Treat `upstream/main` as a reference tree whose `backend`, `frontend`, and `config.example.yaml` paths map below `gateway/`. Use upstream `v3.0.7` as the three-way merge base, resolve local divergences in place, and keep deployment topology unchanged.

**Tech Stack:** Go, React, TypeScript, SQLite, Docker Compose, WARP

---

### Task 1: Generate The Path-Mapped Merge

**Files:**
- Modify: `gateway/backend/**`
- Modify: `gateway/frontend/**`
- Modify: `gateway/config.example.yaml`

- [ ] Merge every upstream file changed after `v3.0.7` against the corresponding current `gateway/` file.
- [ ] Copy newly added upstream backend and frontend files into the mapped directories.
- [ ] Record every conflict and verify no root-level duplicate application is introduced.

### Task 2: Preserve Customized Runtime Behavior

**Files:**
- Modify: `gateway/backend/internal/app/**`
- Modify: `gateway/backend/internal/application/**`
- Modify: `gateway/backend/internal/infra/**`

- [ ] Integrate upstream high-concurrency selection, invalidation, billing recovery, egress fallback, timeout, and usage fixes.
- [ ] Preserve local Web Statsig challenge signing, stream anti-bot retry, video inheritance, media cache, and memory/SQLite defaults.
- [ ] Keep Redis optional and do not add services to Docker Compose.

### Task 3: Adapt The Management UI

**Files:**
- Modify: `gateway/frontend/src/features/accounts/**`
- Modify: `gateway/frontend/src/features/settings/**`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

- [ ] Add upstream egress operations and fallback controls using the existing component and theme system.
- [ ] Preserve customized account quota presentation and all public React workspaces.

### Task 4: Verify The Integrated Tree

**Files:**
- Test: `gateway/backend/**`
- Test: `gateway/frontend/**`

- [ ] Run focused tests for all conflicted packages.
- [ ] Run `go test ./...`, `go vet ./...`, and `go build ./cmd/grok2api` from `gateway/backend`.
- [ ] Run the frontend test, typecheck, and production build commands.
- [ ] Run `git diff --check` and inspect the final change list for duplicate root applications or deployment changes.

### Task 5: Record, Push, And Deploy

**Files:**
- Modify: `gateway/UPSTREAM.md`
- Modify: `gateway/VERSION`

- [ ] Record upstream commit `41665caa21279349b4f7a6ea5f4ea512b9414c04` and the synchronization date.
- [ ] Commit the verified changes and push `HEAD:main`.
- [ ] Deploy only `grok2api_go`, retaining the existing WARP container and data volume.
- [ ] Verify health, readiness, production Chat, image edit, and recent anti-bot logs.
