# Parent Post Upload Fallback Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When image history continue/outpaint requests fail on the parentPostId edit path, automatically fall back to single-image upload edit so the workbench can still continue editing upload-generated images.

**Architecture:** Keep the existing parentPostId-first behavior. If the parent-post edit path returns no images after the upstream chat attempt, emit a fallback progress event, then retry once through the regular upload-based image edit path using the normalized imagine-public source image as the single reference.

**Tech Stack:** FastAPI, Python async services, Grok image edit service, Python unittest.

---

### Task 1: Lock fallback behavior with a failing test

**Files:**
- Modify: `tests/test_image_edit_parent_post_source_normalization.py`
- Verify: `app/services/grok/services/image_edit.py`

**Step 1: Write the failing test**
Add a unit test asserting that `edit_with_parent_post()` calls `edit()` with one normalized image reference when the parent-post collect phase returns no images.

**Step 2: Run test to verify it fails**
Run: `python3 -m unittest tests.test_image_edit_parent_post_source_normalization`
Expected: FAIL because fallback is not implemented yet.

**Step 3: Write minimal implementation**
In `edit_with_parent_post()`, after `_collect_images(...)`, detect the empty result case, log it, emit a fallback progress event, and call `self.edit(...)` with `images=[image_ref]`.

**Step 4: Run test to verify it passes**
Run: `python3 -m unittest tests.test_image_edit_parent_post_source_normalization`
Expected: PASS.

### Task 2: Re-run targeted regressions and rebuild local service

**Files:**
- Modify: `app/services/grok/services/image_edit.py`

**Step 1: Run verification**
Run:
- `python3 -m py_compile app/services/grok/services/image_edit.py`
- `python3 -m unittest tests.test_image_edit_parent_post_source_normalization tests.test_page_routes tests.test_default_nsfw tests.test_app_chat_reasoning tests.test_video_30s_support tests.test_video_auth_fallback tests.test_video_stream_fallback`
- `node --test tests/imagine_image_action_entries.test.cjs tests/nsfw_video_extend_entries.test.cjs`

**Step 2: Rebuild local container**
Run: `APP_ASSET_VERSION=extend-smoke-20260308k docker compose up -d --build grok2api`
Expected: local service restarts with fallback fix.
