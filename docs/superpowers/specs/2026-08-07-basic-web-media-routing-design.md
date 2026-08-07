# Basic Web Media Routing Design

## Goal

Restore Basic Grok Web video access, keep legacy public media routes aligned with the selected UI mode, and make cached image references portable across Web and Console providers.

## Confirmed Failures

- The Web catalog marks `grok-imagine-video` as Super-only, so Basic accounts never publish the video capability even though the upstream product grants a small Basic video allowance.
- The legacy Imagine page always selects the Web quality model, including non-Pro requests, while Basic accounts only publish the lite image model.
- Console media accepts HTTPS and data URLs but rejects the internal `grok2api-media://image/<id>` reference used by the cache and legacy pages.
- Web quota synchronization only exposes chat windows. A Basic video allowance must not be presented as a fabricated chat quota value.

## Design

1. Mark Web video as available from the Basic tier. Model synchronization will then persist `grok-imagine-video` for Basic accounts and normal account rotation can continue after an account-scoped 429 response.
2. Select the legacy Imagine model from the `pro` flag: non-Pro uses `Web/grok-imagine-image-lite`; Pro keeps the Web quality route.
3. Add a Console media-reference resolver that opens internal image assets through `provider.ImageAssetReader`, validates a 20 MiB image payload, and converts it to a data URL before image edit or video generation.
4. Keep quota reporting honest. This change enables Basic video routing but does not invent a remaining-video count when the upstream quota endpoint does not provide one. Upstream 429 responses remain the exhaustion signal and trigger account switching.

## Error Handling

- Missing or unreadable internal assets return a clear local validation error before contacting the upstream provider.
- Oversized or non-image assets are rejected.
- Account-scoped Web 429 responses continue through the existing video account-switch path.

## Verification

- Unit tests cover Basic Web video discovery and tier ordering.
- Legacy Imagine tests cover both non-Pro and Pro model selection.
- Console tests prove internal cache references become image data URLs for video and image edit requests.
- Production verification confirms Basic accounts synchronize the Web video capability and cached reference requests no longer fail locally.
