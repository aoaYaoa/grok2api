# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `86ae605717087c2df479dc8a268219d3ad8fe731`
- Version: `v3.1.2`
- Imported: `2026-08-14`

## Selected Updates

The Go and React sources were three-way synchronized through upstream commit `86ae605717087c2df479dc8a268219d3ad8fe731`. Local public workspaces, media caching, video extension, five-minute recovery, cross-account retry, WARP routing, SQLite storage, and deployment layout remain preserved.

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
