# Grok2API

Grok2API is a Go gateway with React public and admin workspaces for Grok Build, Grok Web, and Grok Console accounts.

> [!NOTE]
> This project is for learning and research. Follow Grok's terms of service and local laws.

> [!NOTE]
> 开源项目欢迎大家支持二开和 PR，但请保留原作者标识和前端标识，尊重他人劳动成果～！

## Runtime

- Go API gateway and account scheduler
- React admin and public workspaces
- SQLite data and local media storage
- In-memory runtime state by default; Redis is optional and not required
- Docker image built from `gateway/Dockerfile`

## Quick Start

```bash
mkdir -p gateway-config
cp gateway/config.example.compat.yaml gateway-config/config.yaml
docker compose up -d --build
```

The default local address is `http://127.0.0.1:18002`.

Configuration, provider behavior, API routes, and development instructions are documented in [gateway/README.md](gateway/README.md).

## Verification

```bash
cd gateway/backend && go test ./...
cd ../frontend && node --test test/*.test.mjs && pnpm lint && pnpm build
cd ../.. && node --test tests/*.test.cjs
```

The production container contains only the Go binary and compiled React assets. The retired FastAPI and legacy static-page implementation is not part of the runtime.
