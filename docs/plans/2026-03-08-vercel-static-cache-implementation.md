# Vercel Static Compatibility And Cache Versioning Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a minimal Vercel-friendly compatibility layer for the current `app/static` layout and centralize HTML asset cache versioning.

**Architecture:** Keep `app/static` as the only static source, enhance page routers to render HTML with a shared asset version token, and add explicit route/static entry fallbacks for deployment environments. Avoid `_public` migration entirely.

**Tech Stack:** FastAPI, Python standard library, unittest, Docker Compose

---

### Task 1: Add failing tests for HTML asset version injection and route fallbacks

**Files:**
- Modify: `tests/test_page_routes.py`

**Step 1: Write failing tests**
- Add tests asserting HTML page responses replace `__ASSET_VERSION__`.
- Add tests asserting `/favicon.ico` or page helper targets return 404 when absent.

**Step 2: Run test to verify it fails**
Run: `python3 -m unittest tests.test_page_routes`
Expected: FAIL because helper/rendering support does not exist yet.

**Step 3: Write minimal implementation**
- Add HTML rendering helper support in page routers.
- Add existence checks for favicon/static entry paths.

**Step 4: Run test to verify it passes**
Run: `python3 -m unittest tests.test_page_routes`
Expected: PASS.

### Task 2: Centralize asset versioning for touched HTML pages

**Files:**
- Create: `app/api/pages/helpers.py`
- Modify: `app/api/pages/public.py`
- Modify: `app/api/pages/admin.py`
- Modify: `app/static/public/pages/video.html`
- Modify: `app/static/public/pages/nsfw.html`
- Modify: `app/static/public/pages/chat.html`
- Modify: `app/static/public/pages/imagine.html`
- Modify: `app/static/public/pages/imagine_workbench.html`
- Modify: `app/static/public/pages/voice.html`
- Modify: `app/static/admin/pages/login.html`
- Modify: `app/static/admin/pages/config.html`
- Modify: `app/static/admin/pages/cache.html`
- Modify: `app/static/admin/pages/token.html`

**Step 1: Write failing test**
- Extend `tests/test_page_routes.py` to expect `__ASSET_VERSION__` replacement in rendered HTML.

**Step 2: Run test to verify it fails**
Run: `python3 -m unittest tests.test_page_routes`
Expected: FAIL with placeholder still present.

**Step 3: Write minimal implementation**
- Introduce one asset version constant.
- Replace hardcoded query strings only in touched entry pages with `__ASSET_VERSION__`.
- Use HTML response helper to replace placeholders server-side.

**Step 4: Run test to verify it passes**
Run: `python3 -m unittest tests.test_page_routes`
Expected: PASS.

### Task 3: Add deployment-friendly root extras

**Files:**
- Modify: `main.py`

**Step 1: Add test coverage if practical**
- If direct route tests are too heavy, rely on focused helper tests and runtime verification.

**Step 2: Implement minimal compatibility**
- Add `/health` endpoint.
- Add `/favicon.ico` response/redirect based on existing static assets.

**Step 3: Verify app still starts**
Run: `python3 -m py_compile main.py app/api/pages/helpers.py app/api/pages/public.py app/api/pages/admin.py`
Expected: PASS.

### Task 4: Regression verification

**Files:**
- Verify existing test files only

**Step 1: Run focused suite**
Run: `python3 -m unittest tests.test_page_routes tests.test_default_nsfw tests.test_app_chat_reasoning tests.test_video_30s_support tests.test_video_auth_fallback tests.test_video_stream_fallback`
Expected: PASS.

**Step 2: Rebuild runtime**
Run: `docker compose up -d --build grok2api`
Expected: container rebuilt and started.

**Step 3: Runtime smoke check**
Run: `docker compose exec -T grok2api python - <<'PY' ... /v1/models ... PY`
Expected: JSON model list prefix returned.
