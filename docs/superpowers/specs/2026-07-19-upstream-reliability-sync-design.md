# Upstream Reliability Sync Design

## Goal

Selectively integrate the reliability and account-management changes from `chenyme/main` at `b28133e` without replacing the customized Go gateway, React public workspaces, media cache, video extension flow, multi-account routing, or deployment topology.

## Scope

The synchronization includes:

- Grok Build recommended client version and user agent update to `0.2.103`.
- Responses remote compaction forwarding, usage normalization, reasoning recovery, and terminal stream validation.
- Reliable SSO rejection persistence across chat, image, video, quota synchronization, conversion, and owned response forwarding.
- Grok Web account setup actions for terms acceptance, birth date, and NSFW enablement.
- Account identity synchronization and provider-link reconciliation improvements.
- Selected-account token refresh and cleanup of cooldown, disabled, and reauthentication-required accounts.
- Clipboard fallback and complete client-key secret display fixes.

Sponsor assets, upstream README replacement, legacy static pages, and direct replacement of customized frontend files are outside this synchronization.

## Integration Strategy

Upstream commits are treated as reference implementations rather than cherry-picked wholesale. Backend behavior is ported into the corresponding `gateway/backend` packages. Admin UI behavior is adapted to existing components, theme tokens, responsive layouts, and account-page workflows in `gateway/frontend`.

Each feature is introduced with focused regression tests. Existing public React workspaces and legacy compatibility adapters remain the authority where upstream paths conflict with customized behavior.

## Reliability Rules

- A confirmed SSO `401` is persisted with a context that survives client cancellation and has a bounded timeout.
- The invalid account is removed from the in-process selector snapshot immediately so the next request chooses another account.
- Remote compaction streams are successful only after the protocol-specific terminal event is observed.
- An interrupted or incomplete upstream stream is recorded with a stable internal error code and does not appear as a successful response.
- Batch account operations validate provider ownership and account IDs before mutation.
- Cleanup never deletes healthy, probing, or reset-pending accounts.

## Account UI

The existing accounts page gains compact menu actions for selected token refresh, provider-scoped cleanup, Web setup actions, and credential export. Destructive cleanup requires confirmation and reports deleted, skipped, and failed counts. New controls follow the existing neutral theme and do not replace the customized account table layout.

## Data Compatibility

Schema additions use idempotent migrations. Existing accounts, provider links, media jobs, runtime settings, SQLite deployment, memory runtime store, and WARP configuration remain valid. Redis is not introduced.

## Verification

- Focused tests for SSO rejection, selector invalidation, compaction stream completion, account setup actions, selected token refresh, and cleanup.
- Full backend build and `go vet`.
- Frontend contract tests and both Vite builds.
- Production smoke tests for health, account list, one non-destructive token refresh path, and current public media routes.

