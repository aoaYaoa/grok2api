# Multi-Reference Merge Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Force the image workbench to use pure multi-reference edit mode when 2 or more reference images are present so dual-subject merge prompts are not overridden by parentPostId chaining.

**Architecture:** Keep the existing workbench and history flows intact, but change request body construction for the main workbench edit action. When `references.length >= 2`, omit `parent_post_id` and `source_image_url` entirely so the backend stays on multi-reference edit mode. Preserve parent-chain behavior for history continue/outpaint actions.

**Tech Stack:** Vanilla JS, FastAPI public imagine API, Node static tests, Python unittest regression suite.

---

### Task 1: Lock the routing rule with a failing test

**Files:**
- Modify: `tests/imagine_image_action_entries.test.cjs`
- Modify: `app/static/public/js/imagine_workbench.js`

**Step 1: Write the failing test**
Add a static assertion that the workbench JS contains an explicit multi-reference routing gate that suppresses `parent_post_id` when two or more references are present.

**Step 2: Run test to verify it fails**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL because the routing gate is missing.

**Step 3: Write minimal implementation**
In `buildWorkbenchEditBody()`, compute `useReferenceMergeMode = references.length >= 2`. Only attach `parent_post_id` and `source_image_url` when `!useReferenceMergeMode`.

**Step 4: Run test to verify it passes**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS.

### Task 2: Keep UI mode metadata coherent

**Files:**
- Modify: `app/static/public/js/imagine_workbench.js`

**Step 1: Add a focused failing expectation if needed**
Assert that the workbench edit call uses upload-style mode when multi-reference merge mode is active.

**Step 2: Implement minimal mode adjustment**
When `references.length >= 2`, pass `mode: 'upload'` from `runEdit()` so history does not misleadingly mark the edit as `parent_post`.

**Step 3: Re-run static tests**
Run: `node --test tests/imagine_image_action_entries.test.cjs tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS.

### Task 3: Verify regressions and rebuild local container

**Files:**
- Modify: `app/static/public/js/imagine_workbench.js`
- Verify: `app/static/public/js/imagine.js`

**Step 1: Run syntax and regression checks**
Run:
- `node --check app/static/public/js/imagine_workbench.js`
- `python3 -m unittest tests.test_page_routes tests.test_default_nsfw tests.test_app_chat_reasoning tests.test_video_30s_support tests.test_video_auth_fallback tests.test_video_stream_fallback`

**Step 2: Rebuild local service**
Run: `APP_ASSET_VERSION=extend-smoke-20260308i docker compose up -d --build grok2api`
Expected: container restarted with updated assets.

**Step 3: Verify served asset**
Check container-served JS contains the multi-reference routing gate.
