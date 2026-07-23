# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `41665caa21279349b4f7a6ea5f4ea512b9414c04`
- Imported: `2026-07-23`

## Selected Updates

The Go and React sources were three-way synchronized from upstream `v3.0.7` through commit `41665caa21279349b4f7a6ea5f4ea512b9414c04`. Local public workspaces, media caching, video extension, five-minute recovery, cross-account retry, WARP routing, SQLite storage, and deployment layout remain preserved.

- High-concurrency account selection, bounded quota refresh state, atomic billing settlement, and hardened recovery paths.
- Configurable egress fallback, proxy batch operations, cache invalidation, and hot-reloadable response-header timeouts.
- Improved spending-limit failover, account quota recovery, Codex model catalog compatibility, and cached-token accounting.
- Provider, persistence, CLI, Console, conversation, and inference compatibility fixes after upstream `v3.0.7`.

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
