# Video Extend Entry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add explicit extend actions for the public `video` and `nsfw` workstations while reusing the existing video extension backend flow.

**Architecture:** Keep `video` on the existing workspace extend flow and add pre-bind helpers for quick actions. Add a small extend controller to `nsfw.js` that tracks the active generated video and submits extension jobs through the existing public video start endpoint.

**Tech Stack:** Static HTML, vanilla JS, CSS, Node `node:test`, Python `unittest`

---

### Task 1: Lock the UX with failing static tests

**Files:**
- Create: `tests/nsfw_video_extend_entries.test.cjs`

**Step 1: Write the failing test**
Check that `nsfw.html` exposes a main extend button and mobile proxy button, and that `video.js` / `nsfw.js` expose quick-extend selectors.

**Step 2: Run test to verify it fails**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: FAIL because selectors/buttons do not exist yet.

### Task 2: Add quick extend to `video`

**Files:**
- Modify: `app/static/public/js/video.js`

**Step 1: Add quick-action markup**
Add `延长` buttons to cache rows and stage/history cards.

**Step 2: Add shared pre-bind helper**
Make a helper that selects a video, binds it into the edit workspace, and optionally opens the panel before calling `runExtendVideo()`.

**Step 3: Wire event delegation**
Handle cache-row extend and card extend clicks without breaking existing download/edit behavior.

### Task 3: Add extend flow to `nsfw`

**Files:**
- Modify: `app/static/public/pages/nsfw.html`
- Modify: `app/static/public/css/nsfw.css`
- Modify: `app/static/public/js/nsfw.js`

**Step 1: Add main extend controls**
Add a desktop action button and mobile FAB proxy entry.

**Step 2: Track current video selection**
Remember per-card URL/postId/origin parent ID, highlight the selected card, and auto-select newly completed videos.

**Step 3: Add extension request helper**
Create a single helper for main/card extend that builds the payload, creates the task, opens SSE, and updates button states.

### Task 4: Verify end-to-end regressions

**Files:**
- Modify: `tests/nsfw_video_extend_entries.test.cjs`

**Step 1: Run focused tests**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS.

**Step 2: Run regression suite**
Run: `python3 -m unittest tests.test_page_routes tests.test_default_nsfw tests.test_app_chat_reasoning tests.test_video_30s_support tests.test_video_auth_fallback tests.test_video_stream_fallback`
Expected: PASS.

**Step 3: Syntax verification**
Run: `python3 -m py_compile app/api/v1/public_api/video.py`
Expected: PASS.
