# Chat Hidden Admin Entry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the visible admin entry from public media pages and allow admins to enter `/admin/login` by typing a hidden command in Chat.

**Architecture:** Keep admin auth and routes unchanged. Remove the public header admin link, add a tiny browser-safe helper that recognizes `#admin`, and intercept Chat sends locally before any network request is made.

**Tech Stack:** Static HTML, vanilla browser JavaScript, Node built-in test runner

---

### Task 1: Add regression tests

**Files:**
- Create: `tests/chat_admin_shortcut.test.cjs`
- Test: `app/static/common/html/public-header.html`
- Test: `app/static/public/js/chat-admin-shortcut.js`

**Step 1: Write the failing test**
Check that the public header no longer exposes `管理后台`, and that `#admin` is recognized as a local hidden command.

**Step 2: Run test to verify it fails**
Run: `node --test tests/chat_admin_shortcut.test.cjs`
Expected: FAIL because the helper file does not exist yet and the header still contains the admin link.

### Task 2: Implement hidden command helper and chat interception

**Files:**
- Create: `app/static/public/js/chat-admin-shortcut.js`
- Modify: `app/static/public/js/chat.js`
- Modify: `app/static/public/pages/chat.html`

**Step 1: Write minimal implementation**
Create a helper exposing `isHiddenAdminCommand()` and `maybeHandleHiddenAdminCommand()`; in `chat.js`, intercept sends before message creation/networking.

**Step 2: Run test to verify it passes**
Run: `node --test tests/chat_admin_shortcut.test.cjs`
Expected: PASS for hidden command behavior.

### Task 3: Remove visible public admin buttons

**Files:**
- Modify: `app/static/common/html/public-header.html`
- Modify: `app/static/common/js/public-header.js`
- Modify: `app/static/public/pages/imagine.html`
- Modify: `app/static/public/pages/imagine_workbench.html`
- Modify: `app/static/public/pages/video.html`
- Modify: `app/static/public/pages/chat.html`
- Modify: `app/static/public/pages/voice.html`
- Modify: `app/static/public/pages/nsfw.html`

**Step 1: Update shared header and cache-busting versions**
Remove the desktop/mobile admin links from the shared public header and bump the shared header asset versions so browsers fetch the new header.

**Step 2: Run test to verify it passes**
Run: `node --test tests/chat_admin_shortcut.test.cjs`
Expected: PASS for header visibility rule.
