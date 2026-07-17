# Fixed Video Extension Workspace Design

## Goal

Keep the Video and NSFW extension workspaces visually stable when moving between the empty, loading, playable-video, extending, and completed-extension states. Loading a video must not change the workspace column width, cause an unexpected page-scale jump, or make the extension workflow appear inside a dialog.

## Current Problem

The empty extension state uses a minimum-height placeholder, while the selected state replaces it with a full-width `aspect-video` player followed by timeline, duration, prompt, and action controls. The active state is therefore substantially taller than the empty state. On narrow viewports the entire workspace is a single column, so this height change moves the history and surrounding content abruptly. During cache-dialog navigation, scrolling can also start before the dialog close animation finishes.

The media element itself must never determine a grid track's width. Its intrinsic dimensions are presentation data, not layout dimensions.

## Chosen Layout

Use one fixed extension workspace shell on desktop and mobile:

1. Header: `Current video and extension` / `Video timeline extension`.
2. Fixed 16:9 media viewport.
3. Timeline and numeric start-time input.
4. Extension duration, prompt, prompt enhancement, and submit action.
5. Reserved extension status/result region.

The empty, loading, poster, and playable states replace only the contents of the fixed media viewport. They do not replace the outer structure.

### Desktop

- Preserve the existing two-column workspace: controls on the left and history/results on the right.
- The controls track keeps its existing bounded width.
- The fixed media viewport uses `width: 100%`, `aspect-ratio: 16 / 9`, `min-width: 0`, and `overflow: hidden`.
- Timeline and form controls remain below the viewport and cannot contribute intrinsic width to the grid track.

### Mobile

- Preserve the same information order in a single column.
- The fixed media viewport spans the available content width and keeps its 16:9 ratio.
- Timeline controls use a bounded two-track grid; the numeric field cannot force horizontal overflow.
- The extension result region is reserved below the action controls so progress and completion do not create a large layout jump.

## Interaction States

### No Video Selected

- Render the fixed 16:9 viewport with the existing instructional empty state.
- Keep timeline and extension inputs hidden or disabled without removing the fixed viewport.
- Do not use a larger minimum-height placeholder than the eventual player.

### Video Selected

- Render the original video inside the same fixed viewport using `object-contain`.
- Preserve the original video as the selected extension source.
- Loading metadata, poster frames, and controls must not change viewport dimensions.

### Extension Running

- Keep the original video visible.
- Show progress in the reserved extension status/result region.
- The in-progress extension may also remain in video history, but it must not appear inside the cache dialog.
- Starting an extension disables duplicate submission until the current task finishes or is stopped.

### Extension Completed

- Keep the original video selected and visible.
- Render the completed extension in the reserved result region and in video history.
- Do not automatically replace the original video.
- Provide an explicit `Continue extending this result` action. Only this action changes the extension source to the completed result.
- Preserve the root/original post ID required for a subsequent extension chain.

### Extension Failed or Stopped

- Remove the failed in-progress result from the reserved result region and history according to the existing failed-task policy.
- Keep the original selected video and its controls intact.
- Re-enable extension submission.

## Cache Dialog Navigation

- Cache browsing may remain a dialog.
- Selecting or extending a cached video first closes the dialog.
- Scrolling to the extension workspace happens only after the dialog close animation completes.
- The selected cached video is then rendered in the fixed viewport.
- Closing the dialog without selecting a video performs no scroll.

## Component Boundaries

### Fixed Media Viewport

Introduce a reusable fixed viewport wrapper or enforce the same stable contract through `VideoPlayer`:

- Stable `aspect-ratio` and width constraints.
- `min-width: 0` and `overflow: hidden`.
- Empty, loading, poster, and player content share the same wrapper.
- The child `<video>` remains `width: 100%`, `height: 100%`, and `object-fit: contain`.

### Extension Result Region

The Video and NSFW pages keep a separate extension-result state instead of replacing the selected source. The state contains the task ID, progress, URL, poster URL, post ID, and root original post ID needed for chaining.

The duplicated Video/NSFW behavior should follow one shared state-update shape. A shared hook is optional only if it reduces meaningful duplication without expanding the change beyond these two pages.

## Accessibility and Usability

- Keep all icon-only controls labeled.
- Maintain at least 44px touch targets on coarse-pointer devices.
- Progress is communicated with text, not color alone.
- The explicit continuation action clearly states that it changes the extension source.
- Respect reduced-motion preferences for scrolling and dialog transitions where the existing component system supports them.

## Verification

### Automated

- Contract tests assert that Video and NSFW use the fixed media viewport contract.
- Tests assert that empty and selected states share the same viewport wrapper.
- Tests assert that extension results use separate state and do not automatically replace the selected source.
- Tests assert that cache navigation waits for the dialog close animation before scrolling.
- Frontend TypeScript, lint, and production builds pass.

### Browser

Verify Video and NSFW at desktop and mobile viewport sizes:

- Empty to selected-video transition keeps the same viewport width and height.
- Poster loading and playback do not resize the workspace.
- Starting an extension does not move the original video or open a dialog.
- Progress and completion remain inside the reserved result region.
- The original video stays selected after completion.
- `Continue extending this result` intentionally changes the source and preserves chain metadata.
- No horizontal scrolling, control overlap, or selected-ring overflow occurs.

## Out of Scope

- Changing the upstream video extension protocol.
- Replacing the cache dialog with a new media-library design.
- Redesigning unrelated media cards or global workspace columns.
- Automatically stitching or replacing the original source without an explicit user action.
