# Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `dd6624cbb1d1243415baa93830870b82ebed2fb5`
- Imported: `2026-07-14`

## Updating

Fetch the desired upstream revision, remove the current vendored tree, and import it at the same prefix:

```sh
git fetch chenyme main
git rm -r gateway
git read-tree --prefix=gateway/ -u chenyme/main
git restore --source=HEAD -- gateway/UPSTREAM.md
git add gateway/UPSTREAM.md
```

Reapply and review local patches after each import. Aside from this provenance file, local patches are limited to base-path routing, private account sync, config wiring, and tests; all other changes should be contributed upstream.
