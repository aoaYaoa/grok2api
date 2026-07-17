# Fixed Video Extension Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stabilize the Video and NSFW extension workspaces across desktop and mobile while preserving the original source video and showing extension progress/results in a dedicated fixed-size region.

**Architecture:** Keep the existing page-level workspace split and cache dialog. Add a stable 16:9 media viewport contract to the extension player, separate extension-result state from the active source state, and delay cache navigation scrolling until the Radix dialog close animation ends. Apply the same state model to Video and NSFW without changing backend APIs.

**Tech Stack:** React 19, TypeScript, Tailwind CSS, Radix Dialog, Node `node:test`, Vite, in-app browser validation.

---

### Task 1: Lock the extension behavior with frontend contract tests

**Files:**
- Modify: `gateway/frontend/test/public-app-contract.test.mjs`

- [x] **Step 1: Add failing assertions for stable extension layout and state ownership**

Add a test that reads both page sources and asserts each page contains:

```js
assert.match(source, /aspect-video w-full/);
assert.match(source, /extensionResult/);
assert.match(source, /延长结果/);
assert.match(source, /以此结果继续延长/);
assert.match(source, /onAnimationEnd=\{finishCacheDialogClose\}/);
assert.doesNotMatch(source, /setActive\(completedExtension\)/);
assert.doesNotMatch(source, /setActiveVideo\(completedExtension\)/);
```

Also assert both pages preserve `originalPostID` when creating an extension task and expose `延长生成中` while the result has no URL.

- [x] **Step 2: Run the focused test and verify the expected failure**

Run:

```bash
node --test test/public-app-contract.test.mjs
```

Expected: the new assertions fail because the current pages only add `延长视频` to history, do not render a separate result region, and scroll immediately from cache callbacks.

### Task 2: Implement fixed media and independent extension result state in Video

**Files:**
- Create: `gateway/frontend/src/public/components/video-extension-result.tsx`
- Modify: `gateway/frontend/src/public/pages/video-page.tsx`

- [x] **Step 1: Add separate extension result state and stable cache-scroll coordination**

Add:

```tsx
const [extensionResult, setExtensionResult] = useState<VideoItem | null>(null);
const pendingExtensionScroll = useRef(false);
```

Add helpers that only flush a pending scroll after the dialog content reports `data-state === "closed"`; ordinary history selections may scroll immediately.

- [x] **Step 2: Track extension progress without replacing the source video**

Extend `watch` with `isExtension` and `extensionRootPostID` arguments. Create one `initialItem` with `originalPostID`, insert it into `videos`, and also set `extensionResult` when it is an extension. On each stream update, update both history and `extensionResult`; on failure remove the history task and clear the extension result.

- [x] **Step 3: Render the result inside a reserved extension region**

Keep the existing selected source video and controls unchanged. Under the submit button, render:

- `延长生成中` plus progress while the extension has no URL.
- A bounded thumbnail, `延长结果` label, and open action after completion.
- An explicit `以此结果继续延长` button that intentionally calls a function to make the result the next source.

Do not call `setActive` merely because an extension completed.

- [x] **Step 4: Wait for cache dialog close before scrolling**

Pass `onAnimationEnd={finishCacheDialogClose}` to the cache `DialogContent`. Cache item callbacks must call `setCacheOpen(false)` and mark the scroll as pending; the animation handler performs the scroll after the closed state. History-card callbacks continue to scroll directly.

### Task 3: Apply the same behavior to NSFW

**Files:**
- Modify: `gateway/frontend/src/public/pages/nsfw-page.tsx`

- [x] **Step 1: Mirror extension-result state and progress tracking**

Apply the same `extensionResult`, `initialItem`, `isExtension`, `extensionRootPostID`, failure cleanup, and progress/result rendering used by Video. Preserve `activeVideo` until the user explicitly selects `以此结果继续延长`.

- [x] **Step 2: Mirror fixed media and cache-dialog navigation**

Keep the existing `VideoPlayer` class contract, add the close-animation scroll handler, and pass the deferred-scroll flag from cached-video callbacks.

### Task 4: Run frontend tests and production build

**Files:**
- No additional source files.

- [x] **Step 1: Run contract tests**

Run:

```bash
node --test test/public-app-contract.test.mjs test/public-api-contract.test.mjs
```

Expected: all public route, media, video, and extension contract tests pass.

- [x] **Step 2: Run TypeScript and Vite production build**

Run:

```bash
pnpm build
```

Expected: TypeScript compilation, admin build, and public build all complete successfully.

### Task 5: Verify desktop and mobile behavior in the browser

**Files:**
- No source changes expected.

- [x] **Step 1: Verify empty and selected states at desktop width**

At approximately `1280x720`, confirm the extension panel keeps its bounded column width and the media viewport remains 16:9 before and after selecting a cached video.

- [x] **Step 2: Verify narrow layout**

At approximately `850x640`, confirm the single-column layout has no horizontal overflow, the media viewport remains 16:9, and controls do not change the outer panel width.

- [x] **Step 3: Verify extension state flow**

Confirm the cache dialog closes before scrolling, the original video remains visible, progress appears in the extension result region, the completed extension appears in the same region and history, and only `以此结果继续延长` changes the source.

### Task 6: Review, commit, push, and deploy

**Files:**
- Modify: `docs/superpowers/plans/2026-07-17-fixed-video-extension-workspace-plan.md` to mark completed steps.

- [x] **Step 1: Run final diff and status checks**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors and only the intended frontend, test, plan, and design files changed.

- [x] **Step 2: Commit and push**

Create one commit:

```bash
git add docs/superpowers/plans/2026-07-17-fixed-video-extension-workspace-plan.md gateway/frontend/src/public/pages/video-page.tsx gateway/frontend/src/public/pages/nsfw-page.tsx gateway/frontend/test/public-app-contract.test.mjs
git commit -m "fix: stabilize video extension workspace"
git push origin HEAD:main
```

- [x] **Step 3: Deploy without touching xianyuandaxian**

On netcup, fetch `origin/main`, check out the pushed commit in `/root/grok2api-go-release`, rebuild and force-recreate only `grok2api_go`, then verify the Go container is healthy. Preserve the existing data volume, domain, WARP, SQLite, and `runtimeStore.driver: memory`.

- [x] **Step 4: Verify production smoke paths**

Check `https://grok.uonoe.com/video`, `/healthz`, and one existing cached video response. Confirm recent startup logs contain no errors and the production checkout is at the pushed commit.
