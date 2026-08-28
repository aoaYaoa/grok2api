# Upstream 62d2775c Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronize `chenyme/grok2api` from recorded marker `d6f6e9f5f8e5d643879a4b12741f64a2e4c8d7a7` through `62d2775c` while preserving local public media workflows and production data.

**Architecture:** Generate the exact upstream tree delta and apply it beneath the vendored `gateway/` prefix with Git three-way patching. Resolve overlaps against local Web video retry, React public pages, SQLite storage, media cache, and multi-WARP deployment contracts.

**Tech Stack:** Go 1.26, React 19, TypeScript 6, Vite 8, pnpm 11, SQLite, Docker Compose.

## Global Constraints

- Do not modify `xianyuandaxian`, OpenClaw, WARP services, Redis configuration, SQLite volumes, cached media, credentials, or `seed/` data.
- Preserve Chat, Imagine, image edit, Video, NSFW, Voice, cache, playback, extension, five-minute retry, and cross-account task behavior.
- Keep `/video` on `grok_web`; do not restore the reverted Console routing.
- Deploy only `grok2api_go` with `--no-deps`; never run Compose `down`.

---

### Task 1: Apply the Upstream Delta

**Files:**
- Modify: upstream-changed files under `gateway/backend/**` and `gateway/frontend/**`
- Modify: `gateway/UPSTREAM.md`

**Interfaces:**
- Consumes: upstream commits `d6f6e9f5..62d2775c`
- Produces: path-mapped staged changes beneath `gateway/`

- [ ] Generate `/tmp/grok2api-upstream-20260828.patch` with `git diff --binary --full-index d6f6e9f5 62d2775c`.
- [ ] Apply it using `git apply --3way --directory=gateway`.
- [ ] Inventory unmerged paths with `git diff --name-only --diff-filter=U` and conflict markers with `rg`.

### Task 2: Preserve Local Contracts While Resolving Conflicts

**Files:**
- Modify: conflicting files under `gateway/backend/internal/**`
- Modify: conflicting files under `gateway/frontend/src/**`
- Test: matching upstream and local `*_test.go`, `*.test.ts`, and `*.test.mjs` files

**Interfaces:**
- Consumes: upstream weekly Imagine limits, Statsig recovery, auth cookie fallback, Build cooldown protection, model-prefix handling, and stream loop guards
- Produces: merged behavior retaining local Web video, media persistence, public React pages, and five-minute deferred retry

- [ ] Resolve each conflict by retaining local public/deployment behavior and importing upstream reliability behavior.
- [ ] Confirm `gateway/frontend/src/public/**`, legacy media handlers, and Docker/WARP files are not removed or replaced.
- [ ] Confirm `/video` continues to submit `grok-imagine-video` through `grok_web`.

### Task 3: Verify and Record the Sync

**Files:**
- Modify: `gateway/UPSTREAM.md`

**Interfaces:**
- Consumes: resolved source tree
- Produces: tested commit recording exact upstream SHA and import date

- [ ] Update `gateway/UPSTREAM.md` to commit `62d2775c` and import date `2026-08-28`.
- [ ] Run focused tests for changed Go packages.
- [ ] Run `go test ./...` and `go build ./cmd/grok2api` from `gateway/backend`.
- [ ] Run `pnpm test`, `pnpm lint`, and `pnpm build` from `gateway/frontend`.
- [ ] Run Quality Guard unit tests when its files changed, then `git diff --check`.

### Task 4: Publish and Deploy

**Files:**
- Add: this plan
- Modify: synchronized gateway source and metadata

**Interfaces:**
- Consumes: verified sync commit
- Produces: pushed branch and healthy production application container

- [ ] Commit as `sync upstream 62d2775c` and push the active branch to `origin`.
- [ ] On `netcup`, fetch the active branch, detach at the new commit, build only `grok2api_go`, and recreate it with `--no-deps`.
- [ ] Verify container health, `/healthz`, `/video`, persisted `/app/data`, and existing WARP containers.
