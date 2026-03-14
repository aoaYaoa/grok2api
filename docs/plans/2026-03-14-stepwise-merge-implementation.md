# Stepwise Multi-Image Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a stepwise merge flow in Imagine Workbench so 3+ local reference images can be merged reliably with user-selected first pair and preserved intermediate results.

**Architecture:** Frontend orchestrates a multi-step loop. Each step submits exactly 2 reference images to `/v1/public/imagine/workbench/edit`. The first step is user-selected; subsequent steps automatically merge the previous result with the next original reference. Intermediate results are added to history and to the reference list tagged as step outputs. Backend endpoints remain unchanged.

**Tech Stack:** Vanilla JS, HTML, CSS, FastAPI static assets, Node `node:test` for static tests, Python `unittest` optional for backend regression.

---

### Task 1: Add failing static tests for stepwise merge entry and modal

**Files:**
- Modify: `/Users/aay/自有项目/grok2api/tests/imagine_image_action_entries.test.cjs`

**Step 1: Write the failing test**

```javascript
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const workbenchHtml = fs.readFileSync('app/static/public/pages/imagine_workbench.html', 'utf8');
const workbenchJs = fs.readFileSync('app/static/public/js/imagine_workbench.js', 'utf8');

// Stepwise merge UI entry
assert.match(workbenchHtml, /stepwise-merge/);
assert.match(workbenchJs, /startStepwiseMerge/);
```

**Step 2: Run test to verify it fails**

Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL with missing stepwise merge entry

**Step 3: Commit**

```bash
git add tests/imagine_image_action_entries.test.cjs
git commit -m "test: require stepwise merge entry"
```

---

### Task 2: Add Stepwise Merge modal + entry point UI

**Files:**
- Modify: `/Users/aay/自有项目/grok2api/app/static/public/pages/imagine_workbench.html`
- Modify: `/Users/aay/自有项目/grok2api/app/static/public/css/imagine_workbench.css`

**Step 1: Write minimal HTML**

Add a hidden modal shell and a button entry near the reference strip:
```html
<button id="stepwiseMergeBtn" class="stepwise-merge-btn" type="button">分步合成</button>
<div id="stepwiseMergeModal" class="stepwise-merge-modal hidden">
  <div class="stepwise-merge-panel">
    <h3>选择首步两张参考图</h3>
    <div id="stepwiseMergeGrid" class="stepwise-merge-grid"></div>
    <div class="stepwise-merge-actions">
      <button id="stepwiseMergeConfirm" disabled>开始合成</button>
      <button id="stepwiseMergeCancel">取消</button>
    </div>
  </div>
</div>
```

**Step 2: Add CSS skeleton**

```css
.stepwise-merge-btn { /* visible only when refs >= 3 */ }
.stepwise-merge-modal { position: fixed; inset: 0; background: rgba(0,0,0,.45); }
.stepwise-merge-panel { max-width: 720px; margin: 8vh auto; }
.stepwise-merge-grid { display: grid; grid-template-columns: repeat(3, minmax(0,1fr)); gap: 12px; }
.stepwise-merge-card { border: 2px solid transparent; cursor: pointer; }
.stepwise-merge-card.is-selected { border-color: #ff6b6b; }
```

**Step 3: Commit**

```bash
git add app/static/public/pages/imagine_workbench.html app/static/public/css/imagine_workbench.css
git commit -m "feat(workbench): add stepwise merge modal shell"
```

---

### Task 3: Implement stepwise merge state and selection logic

**Files:**
- Modify: `/Users/aay/自有项目/grok2api/app/static/public/js/imagine_workbench.js`

**Step 1: Write failing test assertions (extend from Task 1)**

Add checks for selection handling and state variables:
```javascript
assert.match(workbenchJs, /stepwiseMergeState/);
assert.match(workbenchJs, /openStepwiseMergeModal/);
```

**Step 2: Run test to verify it fails**

Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL

**Step 3: Add state + modal plumbing**

Add state:
```javascript
const stepwiseMergeState = {
  active: false,
  pendingRefs: [],
  selectedIds: new Set(),
  currentStep: 0,
  totalSteps: 0,
  lastResultUrl: '',
};
```

Add functions:
- `openStepwiseMergeModal()`
- `renderStepwiseMergeGrid()`
- `toggleStepwiseSelection(id)`
- `closeStepwiseMergeModal()`

Selection rules: only allow two selections; enable confirm when two chosen.

**Step 4: Commit**

```bash
git add app/static/public/js/imagine_workbench.js tests/imagine_image_action_entries.test.cjs
git commit -m "feat(workbench): stepwise merge state + selection"
```

---

### Task 4: Implement stepwise merge execution loop

**Files:**
- Modify: `/Users/aay/自有项目/grok2api/app/static/public/js/imagine_workbench.js`

**Step 1: Write failing test for entry execution**

```javascript
assert.match(workbenchJs, /runStepwiseMerge/);
```

**Step 2: Run test to verify it fails**

Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: FAIL

**Step 3: Implement `runStepwiseMerge()`**

Pseudocode:
```javascript
async function runStepwiseMerge() {
  const order = state.referenceImages.map(...original order...)
  const [firstA, firstB] = chosen
  const remaining = order.filter(id not in [firstA, firstB])
  let current = await mergeTwo(firstA, firstB, step=1)
  for (next of remaining) {
    current = await mergeTwo(current, next, step++)
  }
}
```

`mergeTwo`:
- build request body with exactly 2 reference_items
- call `requestWorkbenchEditStream`
- on success: add history entry, add as reference image tagged `step_result: true`
- on failure: retry once, then stop

**Step 4: Commit**

```bash
git add app/static/public/js/imagine_workbench.js tests/imagine_image_action_entries.test.cjs
git commit -m "feat(workbench): stepwise merge loop"
```

---

### Task 5: Tag step results and preserve history

**Files:**
- Modify: `/Users/aay/自有项目/grok2api/app/static/public/js/imagine_workbench.js`

**Step 1: Add tagging helper**

```javascript
function markStepResult(entry, stepIndex) {
  entry.stepResult = true;
  entry.stepIndex = stepIndex;
}
```

**Step 2: Update history rendering**

Show step badge like `Step 2/4` on result cards.

**Step 3: Commit**

```bash
git add app/static/public/js/imagine_workbench.js
git commit -m "feat(workbench): tag step results in history"
```

---

### Task 6: Final verification

**Files:**
- Test: `/Users/aay/自有项目/grok2api/tests/imagine_image_action_entries.test.cjs`

**Step 1: Run tests**

Run: `node --test tests/imagine_image_action_entries.test.cjs`
Expected: PASS

**Step 2: (Optional) Manual check**

- Add 3 reference images
- Select two in stepwise merge modal
- Verify step results appear in history and are added to references

**Step 3: Commit**

```bash
git add app/static/public/js/imagine_workbench.js app/static/public/pages/imagine_workbench.html app/static/public/css/imagine_workbench.css tests/imagine_image_action_entries.test.cjs
git commit -m "feat(workbench): stepwise multi-image merge"
```
