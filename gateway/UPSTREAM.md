# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `ca8b2474bf2ed1a0f96f22044cd3d8031a838e37`
- Imported: `2026-07-21`

## Selected Updates

The Go and React sources were three-way synchronized from the previous reviewed baseline `d3dc3d8f10570f57bdaa3774583a53d508a31435` to upstream `v3.0.6`. Local public workspaces, media caching, video extension, cross-account retry, WARP routing, SQLite storage, and deployment layout remain preserved.

- Grok Build client baseline `0.2.106` and provider fallback improvements.
- Grok Console provider, account identity/settings synchronization, and updated model catalog support.
- FlareSolverr-compatible egress support without enabling an additional production container by default.
- Account auto-clean, media audits, prompt cache reliability, stream recovery, and unlimited client-key limit support.
- Updated admin dashboard, media management, account tooling, and version update checks.

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
