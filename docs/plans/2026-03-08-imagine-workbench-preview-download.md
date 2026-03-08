# Imagine Workbench Preview Download Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a download button to the imagine workbench preview lightbox so the currently previewed image can be saved directly.

**Architecture:** Reuse the existing preview lightbox state. Add a download anchor in the lightbox toolbar and populate its `href` and `download` attributes whenever `openPreviewLightbox()` opens a preview.

**Tech Stack:** Static HTML, vanilla JavaScript, CSS, Node.js built-in test runner.

---

### Task 1: Lock the lightbox download UI in tests

**Files:**
- Modify: `tests/imagine_image_action_entries.test.cjs`

**Step 1: Write the failing test**
Assert that the preview lightbox contains a download control and that the workbench JS wires that control from `openPreviewLightbox()`.

**Step 2: Run test to verify it fails**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL because the download control does not exist yet.

**Step 3: Write minimal implementation**
No production code in this task.

**Step 4: Run test to verify it passes**
Run after implementation.
Expected: PASS.

### Task 2: Add the lightbox download action

**Files:**
- Modify: `app/static/public/pages/imagine_workbench.html`
- Modify: `app/static/public/js/imagine_workbench.js`
- Modify: `app/static/public/css/imagine_workbench.css`

**Step 1: Write the failing test**
Covered by Task 1.

**Step 2: Run test to verify it fails**
Covered by Task 1.

**Step 3: Write minimal implementation**
- Add a download anchor next to the close button.
- Style the lightbox action row so buttons align cleanly.
- Populate `href` and `download` when opening the preview.

**Step 4: Run test to verify it passes**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS.
