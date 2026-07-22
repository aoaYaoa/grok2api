# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `11bb5e20e7409ecaa64cf083c1e302fbb6ab30e7`
- Imported: `2026-07-22`

## Selected Updates

The Go and React sources were three-way synchronized from the previous reviewed baseline `ca8b2474bf2ed1a0f96f22044cd3d8031a838e37` to upstream `v3.0.7`. Local public workspaces, media caching, video extension, five-minute recovery, cross-account retry, WARP routing, SQLite storage, and deployment layout remain preserved.

- Claude Code and Codex prompt-cache affinity, including stable session routing and native client-tool cache compatibility.
- Bounded, redacted Grok Web image/video diagnostics and clearer video failure logging without exposing credentials or upstream media URLs.
- Proxy-pool endpoint support with per-proxy cooldown isolation in the existing egress manager.
- Build billing/free-profile inference updates for account routing and quota presentation.
- Provider, persistence, CLI, Console, conversation, and inference compatibility fixes from upstream `v3.0.7`.

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
