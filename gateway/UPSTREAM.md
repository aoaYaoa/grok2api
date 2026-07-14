# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `dd6624cbb1d1243415baa93830870b82ebed2fb5`
- Imported: `2026-07-14`

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

Before committing the update, change both `Commit` above to the exact value of `UPSTREAM_COMMIT` and `Imported` to the import date, then stage those metadata changes in the same commit as the vendored tree. Reapply and review local patches after each import. Aside from this provenance file, local patches are limited to base-path routing, private account sync, config wiring, and tests; all other changes should be contributed upstream.
