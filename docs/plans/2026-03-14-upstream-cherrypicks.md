# Upstream Cherry-picks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Cherry-pick selected low-conflict fixes from `chenyme` and multi-reference video support from `xianyudaxian` without pulling the `_public` refactor.

**Architecture:** Work on `codex/merge-upstreams-20260314` only. Apply cherry-picks in dependency order, resolve conflicts commit-by-commit, then run focused tests. Restore local stash afterward.

**Tech Stack:** Git, Python unittest, Node test runner

---

### Task 1: Confirm clean working tree and current branch

**Files:**
- None

**Step 1: Check status**

Run: `git status --short`
Expected: clean

**Step 2: Confirm branch**

Run: `git branch --show-current`
Expected: `codex/merge-upstreams-20260314`

---

### Task 2: Cherry-pick xianyudaxian multi-reference base UI commit

**Files:**
- Modify: `app/static/public/css/video.css`
- Modify: `app/static/public/js/video.js`
- Modify: `app/static/public/pages/video.html`
- Modify: `app/static/common/css/toast.css`

**Step 1: Cherry-pick**

Run: `git cherry-pick 0dd0b40`
Expected: commit applied

---

### Task 3: Cherry-pick xianyudaxian interaction fix

**Files:**
- Modify: `app/api/v1/public_api/imagine.py`
- Modify: `app/api/v1/public_api/video.py`
- Modify: `app/services/grok/services/image_edit.py`
- Modify: `app/services/grok/services/video.py`
- Modify: `app/static/common/js/prompt-enhancer.js`
- Modify: `app/static/public/css/imagine_workbench.css`
- Modify: `app/static/public/css/video.css`
- Modify: `app/static/public/js/imagine_workbench.js`
- Modify: `app/static/public/js/video.js`
- Modify: `app/static/public/pages/imagine_workbench.html`
- Modify: `app/static/public/pages/video.html`

**Step 1: Cherry-pick**

Run: `git cherry-pick df8c592`
Expected: commit applied

---

### Task 4: Cherry-pick payload logging

**Files:**
- Modify: `app/services/grok/services/image_edit.py`
- Modify: `app/services/grok/services/video.py`

**Step 1: Cherry-pick**

Run: `git cherry-pick 76d2d62`
Expected: commit applied

---

### Task 5: Cherry-pick multi-reference request fix

**Files:**
- Modify: `app/services/grok/services/image_edit.py`
- Modify: `app/services/grok/services/video.py`
- Modify: `app/static/public/js/video.js`

**Step 1: Cherry-pick**

Run: `git cherry-pick bf86ff9`
Expected: commit applied

---

### Task 6: Cherry-pick core multi-reference API support

**Files:**
- Modify: `app/api/v1/chat.py`
- Modify: `app/services/grok/services/video.py`

**Step 1: Cherry-pick**

Run: `git cherry-pick 699bc08`
Expected: commit applied

---

### Task 7: Cherry-pick chenyme low-conflict model/log fix

**Files:**
- Modify: `app/services/grok/services/model.py`
- Modify: `app/services/reverse/app_chat.py`

**Step 1: Cherry-pick**

Run: `git cherry-pick d322c46`
Expected: commit applied

---

### Task 8: Cherry-pick chenyme CF cookies config update

**Files:**
- Modify: `app/services/reverse/assets_download.py`
- Modify: `app/services/reverse/assets_list.py`
- Modify: `app/services/reverse/assets_upload.py`
- Modify: `app/services/reverse/utils/headers.py`
- Modify: `config.defaults.toml`
- Modify: `readme.md`

**Step 1: Cherry-pick**

Run: `git cherry-pick f180c95`
Expected: commit applied

---

### Task 9: Cherry-pick chenyme video generation fixes

**Files:**
- Modify: `app/services/grok/services/video.py`
- Modify: `app/api/v1/chat.py`
- Modify: `app/static/public/js/video.js`
- Modify: `config.defaults.toml`

**Step 1: Cherry-pick**

Run: `git cherry-pick 56bbd4b`
Expected: commit applied

---

### Task 10: Run Python validation tests

**Files:**
- Test: `tests/test_video_extension_payload.py`
- Test: `tests/test_video_extension_runtime_errors.py`

**Step 1: Run tests**

Run: `python3 -m unittest tests.test_video_extension_payload -v`
Expected: PASS

**Step 2: Run tests**

Run: `python3 -m unittest tests.test_video_extension_runtime_errors -v`
Expected: PASS

---

### Task 11: Run JS UI tests (if UI files changed)

**Files:**
- Test: `tests/nsfw_video_extend_entries.test.cjs`
- Test: `tests/imagine_image_action_entries.test.cjs`

**Step 1: Run test**

Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS

**Step 2: Run test**

Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS

---

### Task 12: Restore local stash (concurrency defaults)

**Files:**
- Modify: `config.defaults.toml`
- Create: `tests/test_concurrency_defaults.py`
- Create: `docs/plans/2026-03-10-concurrency-defaults-plan.md`

**Step 1: Pop stash**

Run: `git stash pop`
Expected: apply cleanly

**Step 2: Run tests**

Run: `python3 -m unittest tests.test_concurrency_defaults -v`
Expected: PASS

