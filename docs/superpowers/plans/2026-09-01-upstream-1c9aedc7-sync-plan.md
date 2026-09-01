# Upstream 1c9aedc7 Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronize `chenyme/grok2api` through `1c9aedc7` while preserving local public media workflows and production data.

**Architecture:** Map upstream root `backend/` and `frontend/` deltas beneath the local `gateway/` prefix and resolve overlaps against local Web video, media cache, retry, and deployment contracts. Verify the merged Go and React application before publishing and replace only the application container in production.

**Tech Stack:** Go 1.26, React 19, TypeScript 6, Vite 8, pnpm 11, SQLite, Docker Compose.

## Global Constraints

- Do not modify `xianyuandaxian`, OpenClaw, WARP services, Redis configuration, SQLite volumes, cached media, credentials, or `seed/` data.
- Preserve Chat, Imagine, image edit, Video, NSFW, Voice, cache, playback, extension, five-minute retry, and cross-account task behavior.
- Keep `/video` on `grok_web`; do not route it through Console.
- Deploy only `grok2api_go` with `--no-deps`; never run Compose `down`.

---

### Task 1: Apply and Resolve the Upstream Delta

**Files:**
- Modify: upstream-changed files under `gateway/backend/**` and `gateway/frontend/**`
- Modify: `gateway/config.example.yaml`
- Test: changed package tests supplied by upstream

**Interfaces:**
- Consumes: upstream commits `62d2775c..1c9aedc7`
- Produces: path-mapped changes beneath `gateway/` with local media behavior preserved

- [x] Apply upstream changes with a binary full-index three-way patch.
- [x] Preserve local public pages, Web reference-video payloads, five-minute recovery, and extension behavior while resolving conflicts.
- [x] Confirm public workspace, media cache, Docker, and WARP files are outside the staged upstream delta.

### Task 2: Record and Verify the Sync

**Files:**
- Modify: `gateway/UPSTREAM.md`
- Add: `docs/superpowers/plans/2026-09-01-upstream-1c9aedc7-sync-plan.md`

**Interfaces:**
- Consumes: resolved source tree
- Produces: tested commit recording the exact upstream SHA and import date

- [x] Record upstream commit `1c9aedc709a0d53ddcfa99349d85b925178e2b69` and import date `2026-09-01`.
- [x] Run focused and complete Go tests plus the backend build.
- [x] Run frontend contract tests, unit tests, lint, and production build.
- [x] Check the staged diff for whitespace errors, conflict markers, and protected-path changes.

### Task 3: Publish and Deploy

**Files:**
- Modify: synchronized gateway sources and metadata

**Interfaces:**
- Consumes: verified sync commit
- Produces: pushed branch and healthy production application container

- [ ] Commit and push the active branch to `origin`.
- [ ] Fetch the commit on `netcup`, build only `grok2api_go`, and recreate it with `--no-deps`.
- [ ] Verify container health, `/healthz`, `/video`, persisted data mounts, and unchanged WARP containers.
