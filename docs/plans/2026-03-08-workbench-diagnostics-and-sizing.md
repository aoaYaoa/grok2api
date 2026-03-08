# Workbench Diagnostics And Sizing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve image workbench reliability diagnostics and stabilize the workbench UI so generated images do not jump in size and history cards expose visible prompt-enhance controls.

**Architecture:** Keep existing workbench request flow, but enrich empty-result handling with explicit upstream state so SSE errors are actionable. Preserve current prompt-enhancer integration, while adding visible style hooks for history cards and constraining the preview/gallery containers to a fixed visual height before and after generation.

**Tech Stack:** Vanilla JS, CSS, FastAPI, Python async services, Node static tests, Python unittest.

---

### Task 1: Lock backend empty-result diagnostics with a failing test

**Files:**
- Create: `tests/test_image_edit_collect_diagnostics.py`
- Modify: `app/services/grok/services/image_edit.py`

**Step 1: Write the failing test**
Assert that when image collect connects but returns no URLs, the service raises an upstream error with explicit detail describing the connected-but-empty state.

**Step 2: Run test to verify it fails**
Run: `python3 -m unittest tests.test_image_edit_collect_diagnostics`
Expected: FAIL because current error detail is only `empty_result`.

**Step 3: Write minimal implementation**
Track whether the upstream connected and whether any image candidates were seen, then include that state in the raised `UpstreamException.details` and log line.

**Step 4: Run test to verify it passes**
Run: `python3 -m unittest tests.test_image_edit_collect_diagnostics`
Expected: PASS.

### Task 2: Lock workbench preview sizing and history enhancer visibility with failing static tests

**Files:**
- Modify: `tests/imagine_image_action_entries.test.cjs`
- Modify: `app/static/public/css/imagine_workbench.css`
- Modify: `app/static/public/js/imagine_workbench.js`

**Step 1: Write the failing tests**
Add static assertions for fixed preview/gallery sizing selectors and a visible history prompt-enhancer hook/class.

**Step 2: Run test to verify it fails**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL because the new selectors/hooks do not exist yet.

**Step 3: Write minimal implementation**
Constrain preview/gallery height and history thumbnail sizing in CSS. Add a clear class/hook to history prompt-enhancer containers so the enhance control is consistently visible and styleable.

**Step 4: Run test to verify it passes**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS.

### Task 3: Run regressions and rebuild local service

**Files:**
- Modify: `app/services/grok/services/image_edit.py`
- Modify: `app/static/public/css/imagine_workbench.css`
- Modify: `app/static/public/js/imagine_workbench.js`

**Step 1: Run verification**
Run:
- `python3 -m py_compile app/services/grok/services/image_edit.py`
- `python3 -m unittest tests.test_image_edit_collect_diagnostics tests.test_image_edit_multi_reference_routing tests.test_image_edit_parent_post_source_normalization tests.test_page_routes tests.test_default_nsfw tests.test_app_chat_reasoning tests.test_video_30s_support tests.test_video_auth_fallback tests.test_video_stream_fallback`
- `node --test tests/imagine_image_action_entries.test.cjs tests/nsfw_video_extend_entries.test.cjs`

**Step 2: Rebuild local container**
Run: `APP_ASSET_VERSION=extend-smoke-20260308m docker compose up -d --build grok2api`
Expected: local workbench serves the new diagnostics and stable sizing.
