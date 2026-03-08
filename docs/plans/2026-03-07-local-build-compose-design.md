# Local Build For grok2api

## Goal
Stop depending on `ghcr.io/chenyme/grok2api:latest` for the `grok2api` service so local fixes survive container restarts and rebuilds.

## Chosen Approach
Change only the `grok2api` service in `docker-compose.yml` from remote `image` to local `build` using the repository `Dockerfile`.

## Why
- Keeps the running container aligned with the checked-out source.
- Makes future `docker compose up -d --build` pick up local fixes.
- Avoids changing `warp` and `flaresolverr`, which are unrelated.

## Operational Notes
- Keep a local image tag for cache reuse.
- Rebuild only `grok2api` after the compose change.
- Rollback is trivial: restore the previous `image:` line.
