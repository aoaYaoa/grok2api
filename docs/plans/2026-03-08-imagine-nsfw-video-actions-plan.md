# Imagine NSFW Video Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Imagine waterfall image action buttons, remove NSFW extend prompt persistence, and make Video/NSFW extend workflows use timeline-selected start positions.

**Architecture:** Keep the existing frontend modules and align their state/input handling instead of introducing new backend contracts. The change is limited to client-side source selection, prompt persistence behavior, and extend request payloads driven by the existing timeline controls.

**Tech Stack:** Vanilla JavaScript, static HTML/CSS, Node built-in test runner.

---

### Task 1: Lock broken Imagine waterfall image card actions

**Files:**
- Modify: `app/static/public/js/imagine.js`
- Test: `tests/imagine_image_action_entries.test.cjs`

**Step 1: Write the failing test**
Add a static test asserting the waterfall action path contains the expected card source resolution and action hooks.

**Step 2: Run test to verify it fails**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL on missing or incorrect waterfall action source wiring.

**Step 3: Write minimal implementation**
Update the waterfall action flow so card-level continue/outpaint resolves from the item’s actual source/image state and applies the returned state back to the card.

**Step 4: Run test to verify it passes**
Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS.

### Task 2: Remove NSFW extend prompt persistence and add extend timeline contract

**Files:**
- Modify: `app/static/public/js/nsfw.js`
- Modify: `app/static/public/pages/nsfw.html`
- Modify: `app/static/public/css/nsfw.css`
- Test: `tests/nsfw_video_extend_entries.test.cjs`

**Step 1: Write the failing test**
Add static assertions that NSFW no longer stores extend prompt drafts and exposes timeline-based extend controls.

**Step 2: Run test to verify it fails**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: FAIL on old prompt persistence and missing timeline extend selectors.

**Step 3: Write minimal implementation**
Remove localStorage draft persistence for NSFW extend prompts and wire extend actions to a timeline-selected start position UI.

**Step 4: Run test to verify it passes**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS.

### Task 3: Bind Video workstation extend to timeline-selected start time

**Files:**
- Modify: `app/static/public/js/video.js`
- Modify: `app/static/public/pages/video.html`
- Modify: `app/static/public/css/video.css`
- Test: `tests/nsfw_video_extend_entries.test.cjs`

**Step 1: Write the failing test**
Add a static test asserting video extend uses the timeline-selected start time and exposes a visible selector/meta display.

**Step 2: Run test to verify it fails**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: FAIL on missing timeline extend contract.

**Step 3: Write minimal implementation**
Keep the current edit timeline but surface the selected start point in the UI and ensure extend requests are built from that value.

**Step 4: Run test to verify it passes**
Run: `node --test tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS.

### Task 4: Final verification

**Files:**
- Test: `tests/imagine_image_action_entries.test.cjs`
- Test: `tests/nsfw_video_extend_entries.test.cjs`

**Step 1: Run targeted static tests**
Run: `node --test tests/imagine_image_action_entries.test.cjs tests/nsfw_video_extend_entries.test.cjs`
Expected: PASS.

**Step 2: Run focused syntax verification**
Run: `node --check app/static/public/js/imagine.js && node --check app/static/public/js/nsfw.js && node --check app/static/public/js/video.js`
Expected: PASS.
