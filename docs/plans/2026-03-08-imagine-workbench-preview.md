# Imagine Workbench Preview Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the imagine workbench's current-image echo preview easier to inspect by enlarging the preview area and adding a click-to-open large preview overlay.

**Architecture:** Keep the existing single-source preview flow in `imagine_workbench.js`, but enrich the preview DOM with an explicit preview action layer. The main preview shell becomes taller so vertical images are larger by default, while a lightweight modal overlay provides detailed inspection without changing upload/edit flow.

**Tech Stack:** Static HTML, vanilla JavaScript, CSS, Node.js built-in test runner.

---

### Task 1: Lock the preview UX in tests

**Files:**
- Modify: `tests/imagine_image_action_entries.test.cjs`

**Step 1: Write the failing test**
Add assertions that the imagine workbench page exposes a preview modal container and that the workbench JS/CSS contain click-to-preview hooks and a larger preview height token.

**Step 2: Run test to verify it fails**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL because the preview overlay hooks do not exist yet.

**Step 3: Write minimal implementation**
No production implementation in this task.

**Step 4: Run test to verify it passes**
Run after implementation task.
Expected: PASS.

### Task 2: Add the large preview overlay

**Files:**
- Modify: `app/static/public/pages/imagine_workbench.html`
- Modify: `app/static/public/js/imagine_workbench.js`
- Modify: `app/static/public/css/imagine_workbench.css`

**Step 1: Write the failing test**
Covered by Task 1.

**Step 2: Run test to verify it fails**
Covered by Task 1.

**Step 3: Write minimal implementation**
- Add a hidden preview modal near the bottom of the workbench page.
- In JS, track the current preview URL and open/close the modal from the current preview image.
- In CSS, raise the default preview shell height and style the modal so desktop/mobile both work.

**Step 4: Run test to verify it passes**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS.

### Task 3: Verify shipped assets

**Files:**
- Modify: none if checks pass

**Step 1: Run syntax and regression checks**
Run:
- `node --check app/static/public/js/imagine_workbench.js`
- `node --test tests/imagine_image_action_entries.test.cjs tests/nsfw_video_extend_entries.test.cjs`

**Step 2: Rebuild local container**
Run: `APP_ASSET_VERSION=<new-tag> docker compose up -d --build grok2api`
Expected: container restarts cleanly.

**Step 3: Confirm container has the new assets**
Check the built container for the new modal and preview height rules.

**Step 4: Commit**
Optional after user review.
