# Public React Workspaces Design

## Goal

Replace every actively served public HTML page with a React route while preserving the existing Chat, Imagine, Workbench, Video, NSFW, and Voice workflows. The Go gateway remains the only production runtime and continues to use the in-memory runtime store.

## Architecture

The existing `gateway/frontend` package produces two independent Vite builds from shared dependencies and UI primitives:

- Admin build: existing `/gateway/` application in `dist/admin`.
- Public build: new root application in `dist/public` with routes `/login`, `/chat`, `/imagine`, `/imagine-workbench`, `/video`, `/nsfw`, and `/voice`.

The Go server serves each build as an SPA with explicit route ownership. Public APIs stay under `/v1/public/*`. Legacy admin redirects remain, but runtime public routes no longer read `app/static/public/pages/*.html` or execute legacy page scripts. PWA icons and the manifest move into the React public assets.

## Shared Public Foundation

The public application provides:

- Public-key storage and verification compatible with `grok2api_public_key`.
- A shared fetch client that adds the bearer token, decodes API errors, and redirects expired sessions to `/login`.
- SSE helpers with cancellation and normalized status, image, video, progress, result, and error events.
- File-to-data-URL utilities, download helpers, parent-post parsing, persistent drafts, responsive navigation, dark mode, and toasts.
- Route-level lazy loading and fixed media aspect ratios to control initial bundle size and layout shift.

## Workspaces

### Login

Verifies `/v1/public/verify`, stores the key, supports password visibility, and links to `/gateway/login` for administrators.

### Chat

Loads `/v1/public/models`, supports streaming and non-streaming chat completions, system prompts, reasoning effort, temperature, top-p, multiple file attachments, conversation clearing, cancellation, copy, and markdown-safe text rendering.

### Imagine

Supports continuous SSE generation, 1-3 concurrent tasks, aspect ratio, NSFW and quality controls, auto-scroll, auto-download, image selection, batch download, lightbox navigation, per-image editing, edit progress, and edit history. The UI labels the transport as SSE because it is the stable Go streaming contract; compatibility WebSocket access is implemented as an adapter over the same image task stream.

### Image Workbench

Supports up to eight reference images, drag/drop and paste, parent-post lookup, ordered references, prompt mentions, edit submission, streaming progress, current-chain updates, preview/download, reset, and local history.

### Video

Supports text-to-video and image-to-video, up to eight references, parent-post resolution, aspect ratio, 6/10/15 second duration, 480p/720p, 1-4 concurrent jobs, SSE progress, cancellation, local/cache history, rename, download, active workspace selection, and video continuation. Continuation is implemented through the Go media gateway rather than returning 501.

### NSFW

Combines candidate image generation, image selection/editing, local image upload, parallel video generation, cancellation, download, and continuation in one workflow. It reuses the same tested image and video clients as the dedicated workspaces.

### Voice

Uses the npm `livekit-client` package instead of a CDN global. Go exposes `/v1/public/voice/token` by selecting an eligible Grok Web credential and requesting `https://grok.com/rest/livekit/tokens`. Go also exposes a same-origin WebSocket signal relay for mobile fallback. The page preserves voice/personality/speed controls, microphone publishing, remote audio playback, connection fallback, diagnostics, log copy, and disconnect.

## Visual System

The public application matches the React admin without becoming a marketing page. It uses neutral surfaces, green for healthy/active state, blue for media actions, amber for warnings, and red only for destructive/error states. Cards are limited to repeated media items and framed tools, use at most 8px radius, and are never nested. Desktop uses a persistent compact navigation rail; mobile uses a top bar and a five-item primary navigation with an overflow menu for secondary workspaces.

Controls use Lucide icons, visible labels, 44px minimum touch targets, stable toolbar dimensions, explicit loading/cancel/error states, keyboard-operable dialogs, and responsive grids. Generated media uses `loading="lazy"`, declared aspect ratios, and non-overlapping action rows.

## Error Handling

Unauthorized responses clear the stored public key and return to login. API failures display the upstream detail and leave inputs/history intact for retry. Streaming helpers close transports on completion, route changes, and user cancellation. Voice connection attempts log each endpoint and relay strategy without exposing tokens. Unsupported browser capabilities show a recovery action instead of a blank panel.

## Testing

- Node contract tests verify public routes, build entries, API paths, auth behavior, and absence of legacy script execution.
- Go tests verify public SPA routing, image WebSocket compatibility, voice token normalization/auth, voice signal relay URL construction, and video continuation request handling.
- Existing Go, Node, lint, and TypeScript builds must remain green.
- Docker canary and Playwright verify all public routes on desktop and mobile, then production smoke tests verify real Chat, Image, and Video generation.

