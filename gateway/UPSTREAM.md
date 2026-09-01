# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `1c9aedc709a0d53ddcfa99349d85b925178e2b69`
- Version: `v3.1.5`
- Imported: `2026-09-01`

## Selected Updates

The Go and React sources were three-way synchronized through upstream commit `1c9aedc709a0d53ddcfa99349d85b925178e2b69`. Local public workspaces, media caching, video extension, five-minute recovery, cross-account retry, WARP routing, SQLite storage, and deployment layout remain preserved.

- Configurable Basic Web video-duration cap applied before upstream submission, while preserving local reference-video and extension workflows.
- Replay-safe quality retries, bounded credential-material failure handling, and clearer pinned-account selection diagnostics.
- Cross-account reasoning recovery, semantic stream-idle detection, root tool-schema normalization, and protocol-safe output-loop failures.
- Compact audit billing display with shared USD tick formatting and improved provider/filter labels.

- Quality Guard pagination, lower status-poll overhead, and runtime-selected account lease quarantine.
- Non-streaming idle-response cooldowns plus improved CLI and Console network-failure classification.
- Quality Guard management-page query controls and Grok2API `v3.1.5` metadata.

- Shared weekly Web Imagine limits with authoritative refresh preservation and product availability fencing.
- Stale-page Statsig recovery for Web media, scoped access-cookie authentication fallback, and transient Build failure isolation.
- Provider-prefixed public model names, SuperGrok Plus billing recognition, and bounded content/reasoning output-loop guards.

- Missing-thinking and empty-stream quality retries with durable account penalties and protocol-safe terminal events.
- Build refresh-token import and persistence recovery, Grok TUI compaction compatibility, and reasoning-effort audit fields.
- Shared proxy profiles with bounded automatic assignment, trusted client-IP recording, and account quota/status UI improvements.
- Current `mediaGenInput` video generation payload, Web text-only routing for image-video requests, bounded request diagnostics, and configurable empty-stream cooldowns.

- Current Imagine `mediaGenInput` editing, browser Clearance reacquisition, Basic media quota routing, and Console video admission limits.
- Clash subscription parsing, VLESS Reality transport, configurable quality probes, media audit accounting, and soft cooldown for interrupted streams.

- Semantic CLI stream idle timeout now distinguishes tool progress from meaningful assistant output, preventing stalled streams from staying alive indefinitely.

- Grok2API v3.1.2 voice APIs, model 4.6 catalog updates, Imagine quota tracking, and Console/Web media reliability improvements.
- Official video create, reference, edit, and extend operations alongside the preserved legacy Web extension workflow.
- Degraded-account monitoring, tunnel-proxy egress support, and refreshed management and Creative Console interfaces.

- Grok2API v3.1.0 Console media and quota support, DPoP protocol updates, and Grok Build 0.2.119/Composer compatibility.
- Session-sticky duplicate model targets, account-isolated connection pools, Quality Guard, and immediate egress recovery.
- Large account-pool query hardening, PostgreSQL URL configuration, model endpoint grouping, and expanded account filters.

- High-concurrency account selection, bounded quota refresh state, atomic billing settlement, and hardened recovery paths.
- Configurable egress fallback, proxy batch operations, cache invalidation, and hot-reloadable response-header timeouts.
- Improved spending-limit failover, account quota recovery, Codex model catalog compatibility, and cached-token accounting.
- Provider, persistence, CLI, Console, conversation, and inference compatibility fixes after upstream `v3.0.7`.
- Configurable Grok Build 403 account invalidation with hot-reloadable error-code matching.
- BOM-tolerant JSON and JSONL account imports across Build, Web, and Console providers.
- Grok Web agreement and linked-account filters, responsive settings tabs, and abortable audit detail loading.
- Grok Build `0.2.111`, nullable tool-schema normalization, OAuth compatibility, and generated tool declarations.
- Blocked-account detection across Web session, quota, and chat 403 responses with SSO invalidation.
- Route-aware account selection and SQLite support for multiple public model names targeting one upstream model.
- Build quota-state reset operations, updated account egress binding controls, and Creative Console message regenerate/edit/delete actions.
- Client-visible Web stream phase tracking that suppresses late reasoning after visible output begins.
- Linked-account preview and deletion controls with media-job safeguards and provider association filters.
- Unlimited account-routing attempts with request-level stop conditions and configurable management controls.
- Build safety, quota, and account-scoped failure classification improvements.
- Client-key-gated reasoning-effort model aliases for supported Build and Console models.
- Grok2API v3.0.10 account export with provider selection, stable cursor pagination, and deferred credential hydration.
- Dual-stack egress probes, selectable probe providers, cancellation-aware proxy health, and safer clearance refresh handling.
- Lower-I/O layered account selection with preserved diagnostics, stale-credential detection, and post-success failure health updates.
- Response timing metrics, token throughput, detailed billing breakdowns, and matching dashboard and request-audit presentation.
- Grok Build free quota estimate correction, provider-safe concurrent account batch updates, and streamed integer tool-argument normalization for Responses clients.
- Grok2API v3.0.11 client-key account-pool scopes, per-request Build proxy rotation, batch clearance controls, detailed credential-refresh diagnostics, Web media upload diagnostics, and mobile admin layout fixes.

## Updating

Configure the upstream remote, fetch it, and select a full commit SHA after reviewing the candidate revision:

```sh
UPSTREAM_URL=https://github.com/chenyme/grok2api
if git remote get-url chenyme >/dev/null 2>&1; then
  git remote set-url chenyme "$UPSTREAM_URL"
else
  git remote add chenyme "$UPSTREAM_URL"
fi

git fetch chenyme main
UPSTREAM_COMMIT="REPLACE_WITH_FULL_REVIEWED_COMMIT_SHA"
git cat-file -e "${UPSTREAM_COMMIT}^{commit}"
git show --stat --oneline "$UPSTREAM_COMMIT"

git rm -r gateway
git read-tree --prefix=gateway/ -u "$UPSTREAM_COMMIT"
git restore --source=HEAD -- gateway/UPSTREAM.md
git add gateway/UPSTREAM.md
```

Before committing the update, change `Commit` above to the exact value of `UPSTREAM_COMMIT` and `Imported` to the import date, then stage those metadata changes in the same commit as the vendored tree. Reapply and review the local public workspace, media, routing, and deployment compatibility patches after each import.
