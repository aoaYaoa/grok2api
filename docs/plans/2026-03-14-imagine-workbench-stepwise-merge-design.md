# Imagine Workbench Stepwise Multi-Image Merge Design

Date: 2026-03-14

## Goal
Enable stable multi-image edits when users provide 3+ local reference images by orchestrating a stepwise merge flow, while preserving intermediate results for review and reuse.

## Scope
- Frontend: add a stepwise merge flow in Imagine Workbench.
- Backend: no new endpoints; reuse `/v1/public/imagine/workbench/edit` with at most 2 reference images per step.
- Retain existing single-step behavior for 1-2 images.

## Architecture
- Frontend orchestrates the stepwise merge workflow.
- Backend remains unchanged in interface; each step is a normal image edit with 2 references.
- Intermediate results are stored in history and also added as reference items for continued merging.

## UX Flow
- When reference image count >= 3, show a “Stepwise Merge” entry point.
- Step 1: user selects two images as the initial merge pair.
- Step N: automatically merge (previous step result + next image in original order).
- Each step produces an output card in the history section and adds it to reference list with a “step result” marker.
- Provide a stop button between steps.

## Data Flow
1. Collect reference list in original order.
2. User selects two images for step 1.
3. POST `/v1/public/imagine/workbench/edit` with exactly two reference items.
4. On success:
   - Store result in history with step index.
   - Add result as reference item (tagged).
5. Continue with (step result + next image) until finished.

## Error Handling
- Each step retries once on failure.
- If retry fails: stop the workflow, show “Step N failed” message, retain completed results.
- Do not remove any previously generated outputs.

## Logging
- Log step index, selected first pair IDs, and resulting image URL for each step.

## Testing
- Frontend: add a static test ensuring stepwise merge entry appears when >=3 references.
- Service-level: no new backend tests required beyond existing multi-reference behavior.

## Out of Scope
- Server-side job orchestration or new API endpoints.
- Automatic pairing heuristics beyond the user-selected first pair.
