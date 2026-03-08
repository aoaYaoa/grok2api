# Vercel Static Compatibility And Cache Versioning Design

## Context
The local project serves UI assets from `app/static`, while upstream Vercel-oriented work assumes `_public/static`. Directly porting the upstream branch would conflict with local customizations. We need a minimal compatibility layer that preserves the current layout, avoids wide refactors, and reduces stale-browser-cache issues.

## Approaches

### A. Compatibility layer on current layout
Keep `app/static` as the source of truth. Add stable page/static entry handling for deployment environments, plus centralize cache version strings used by HTML pages.

Pros:
- Minimal conflict with local changes
- No large asset migration
- Easier rollback and verification

Cons:
- Does not fully converge with upstream `_public` layout
- Keeps two deployment styles conceptually different

### B. Migrate to upstream `_public/static`
Reorganize current assets to match the upstream Vercel branch.

Pros:
- Closer to upstream
- Easier future cherry-picks from that branch

Cons:
- High merge risk with local custom pages/scripts
- Broad file moves and path rewrites

### C. Cache-only cleanup
Only unify version strings, no route compatibility work.

Pros:
- Smallest change

Cons:
- Does not address deployment-specific missing page/static issues

## Decision
Choose approach A.

## Design

### Routing and static compatibility
- Keep `app/static` mounted at `/static`.
- Keep page routers under `app/api/pages` as the source of HTML entry routes.
- Add explicit existence checks and stable responses for page-like assets such as `manifest.webmanifest`, `sw.js`, and favicon.
- Add a lightweight `/health` endpoint for deployment probes.
- Avoid any `_public` directory migration.

### Cache versioning
- Introduce a single server-side asset version constant derived from package/app version or a local constant.
- Page routers will inject the version into HTML responses by replacing `__ASSET_VERSION__` placeholders.
- Update the main public/admin HTML entry pages to use `?v=__ASSET_VERSION__` instead of scattered hardcoded numbers where touched.
- This keeps the browser cache-busting mechanism centralized.

### Error handling
- Missing page/static entry files return `404`, never `500` due to blind `FileResponse` paths.
- Favicon/manifest/service worker should also return `404` if absent.

### Testing
- Route helper tests for missing/existing files.
- HTML rendering tests to confirm `__ASSET_VERSION__` is replaced.
- Re-run video/nsfw regression tests because touched pages overlap with media UI.

## Success Criteria
- Existing app/static layout remains unchanged.
- Pages still load locally.
- Missing entry files return `404`.
- Asset cache version can be changed in one place.
- Docker rebuild starts cleanly and `/v1/models` remains reachable.
