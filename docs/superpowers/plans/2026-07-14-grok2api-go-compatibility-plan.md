# Grok2API Go Compatibility Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the latest Go gateway and management application beside the existing customized Python application, preserve all current URLs and workflows, synchronize Web SSO accounts, and deploy the verified hybrid stack without Redis.

**Architecture:** Vendor `chenyme/main` below `gateway/`, run Python and Go as separate containers, and publish one Nginx edge container. Existing URLs stay on Python; `/gateway` and `/gateway/v1` reach Go. A private shared-secret synchronization protocol reconciles Web SSO account mutations between Python local token storage and Go SQLite.

**Tech Stack:** Python 3.13/FastAPI, Go 1.26/Gin, React 19/Vite, Docker Compose, Nginx, SQLite, in-memory runtime store, Node built-in tests, Python unittest, Go test.

---

## File Structure

- `gateway/`: vendored upstream Go/React source at commit `dd6624c`.
- `gateway/UPSTREAM.md`: source remote, commit, update procedure, and local patch inventory.
- `gateway/frontend/vite.config.ts`: `/gateway/` asset base.
- `gateway/frontend/src/app/router.tsx`: React Router basename.
- `gateway/backend/internal/compat/`: shared sync payloads, fingerprints, and HTTP client.
- `gateway/backend/internal/transport/http/compat/`: private Go reconciliation endpoint.
- `app/services/gateway_sync/`: Python sync models, parsing, reconciliation, and Go client.
- `app/api/internal/gateway_sync.py`: private Python reconciliation API.
- `app/api/internal/__init__.py`: internal router registration.
- `nginx/conf/hybrid.conf`: route ownership and streaming proxy behavior.
- `docker-compose.yml`: edge, Python, Go, WARP, and optional FlareSolverr services.
- `gateway/config.example.compat.yaml`: SQLite + memory single-instance Go configuration.
- `tests/test_gateway_sync.py`: Python sync behavior and secret redaction.
- `tests/test_gateway_routes.py`: FastAPI internal route registration.
- `tests/gateway_edge_routes.test.cjs`: static route ownership assertions.
- `gateway/backend/internal/transport/http/compat/handler_test.go`: Go reconciliation authentication and behavior.
- `gateway/frontend/src/app/router.test.tsx`: base-path contract.

### Task 1: Vendor and identify the upstream snapshot

**Files:**
- Create: `gateway/**`
- Create: `gateway/UPSTREAM.md`

- [ ] **Step 1: Import the upstream tree below `gateway/`**

Run:

```bash
git read-tree --prefix=gateway/ -u chenyme/main
```

Expected: upstream commit `dd6624c` appears only below `gateway/`; existing Python files are unchanged.

- [ ] **Step 2: Record the source and local patch policy**

Create `gateway/UPSTREAM.md` containing:

```markdown
# Vendored Upstream

- Remote: `https://github.com/chenyme/grok2api`
- Commit: `dd6624cbb1d1243415baa93830870b82ebed2fb5`
- Imported: `2026-07-14`

Update with `git read-tree --prefix=gateway/ -u <reviewed-commit>` in a clean integration branch. Local compatibility patches are limited to base-path routing, private account synchronization, configuration wiring, and tests.
```

- [ ] **Step 3: Verify the Python tree did not change**

Run:

```bash
git diff --name-only HEAD -- . ':!gateway' ':!docs'
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add gateway
git commit -m "build: vendor Go gateway upstream"
```

### Task 2: Make the Go frontend work below `/gateway`

**Files:**
- Modify: `gateway/frontend/vite.config.ts`
- Modify: `gateway/frontend/src/app/router.tsx`
- Create: `gateway/frontend/src/app/router.test.tsx`

- [ ] **Step 1: Write the failing base-path test**

The test must assert that Vite exports `base: "/gateway/"` and that `createBrowserRouter` receives `{ basename: "/gateway" }`.

- [ ] **Step 2: Run the test and confirm failure**

Run:

```bash
cd gateway/frontend && corepack pnpm test -- --run src/app/router.test.tsx
```

Expected: failure because the current router has no basename.

- [ ] **Step 3: Add the base path**

Update Vite:

```ts
export default defineConfig({
  base: "/gateway/",
  plugins: [react(), tailwindcss()],
  // existing configuration remains unchanged
});
```

Update the router:

```tsx
export const router = createBrowserRouter(
  [/* existing route objects */],
  { basename: "/gateway" },
);
```

- [ ] **Step 4: Verify frontend checks**

Run:

```bash
cd gateway/frontend && corepack pnpm lint
cd gateway/frontend && corepack pnpm build
```

Expected: both commands exit 0 and generated assets use `/gateway/` URLs.

- [ ] **Step 5: Commit**

```bash
git add gateway/frontend
git commit -m "feat: mount gateway admin below gateway path"
```

### Task 3: Add edge routing and the hybrid Compose stack

**Files:**
- Modify: `docker-compose.yml`
- Create: `nginx/conf/hybrid.conf`
- Create: `gateway/config.example.compat.yaml`
- Create: `tests/gateway_edge_routes.test.cjs`

- [ ] **Step 1: Write failing static route tests**

Assert that the edge config routes `/gateway/`, `/api/admin/v1/`, `/gateway/v1/`, `/gateway/healthz`, and `/gateway/readyz` to Go, while `/`, `/admin/`, `/v1/public/`, `/v1/admin/`, and `/v1/files/` remain on Python.

- [ ] **Step 2: Run the route test and confirm failure**

```bash
node --test tests/gateway_edge_routes.test.cjs
```

Expected: failure because `hybrid.conf` does not exist.

- [ ] **Step 3: Create the edge config**

Use upstreams `grok2api_python:8000` and `grok2api_go:8000`. For `/gateway/v1/`, strip `/gateway` before proxying. Disable buffering for SSE and set a two-hour read timeout on media generation paths.

- [ ] **Step 4: Update Compose**

Compose must publish only `grok2api_edge:8000` as `${GROK2API_PORT:-18000}:8000`. Python, Go, WARP, and FlareSolverr remain internal. Go mounts `./data/gateway/config.yaml` read-only and `./data/gateway:/app/data`; its configuration selects SQLite and memory.

- [ ] **Step 5: Validate configuration**

```bash
docker compose config
node --test tests/gateway_edge_routes.test.cjs
```

Expected: valid Compose output and passing route tests; no Redis service or Redis runtime selection.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml nginx/conf/hybrid.conf gateway/config.example.compat.yaml tests/gateway_edge_routes.test.cjs
git commit -m "feat: add hybrid Python and Go edge stack"
```

### Task 4: Implement Python-side account reconciliation

**Files:**
- Create: `app/services/gateway_sync/__init__.py`
- Create: `app/services/gateway_sync/models.py`
- Create: `app/services/gateway_sync/parser.py`
- Create: `app/services/gateway_sync/service.py`
- Create: `app/services/gateway_sync/client.py`
- Create: `app/api/internal/__init__.py`
- Create: `app/api/internal/gateway_sync.py`
- Modify: `main.py`
- Modify: `app/api/v1/admin_api/token.py`
- Modify: `config.defaults.toml`
- Create: `tests/test_gateway_sync.py`
- Create: `tests/test_gateway_routes.py`

- [ ] **Step 1: Write failing tests**

Cover:

```python
def test_source_key_is_sha256_prefixed(): ...
def test_plain_and_json_web_imports_are_parsed_without_logging_tokens(): ...
async def test_reconcile_adds_updates_and_removes_tokens_idempotently(): ...
async def test_go_origin_does_not_echo_back_to_go(): ...
async def test_invalid_sync_secret_returns_401(): ...
async def test_token_admin_update_enqueues_full_snapshot(): ...
```

- [ ] **Step 2: Run the tests and confirm failure**

```bash
python3 -m unittest tests.test_gateway_sync tests.test_gateway_routes -v
```

- [ ] **Step 3: Implement stable identity and payloads**

Use:

```python
def source_key(token: str) -> str:
    raw = token.removeprefix("sso=").strip()
    return "sso:" + hashlib.sha256(raw.encode("utf-8")).hexdigest()
```

Payloads contain `source_key`, `pool`, `enabled`, `note`, and a token only for import/upsert. Logging uses source-key prefixes and counts only.

- [ ] **Step 4: Implement reconciliation**

The service normalizes the current token pools, applies Go-origin upsert/update/delete operations under the existing storage lock, reloads `TokenManager`, and suppresses outbound sync when `origin == "go"`.

- [ ] **Step 5: Add private routes**

Register `/internal/gateway-sync/reconcile`, `/internal/gateway-sync/import`, `/internal/gateway-sync/update`, and `/internal/gateway-sync/delete`. Require `X-Grok2API-Sync-Secret` with `hmac.compare_digest`. Do not include this prefix in Nginx public locations.

- [ ] **Step 6: Hook the existing token update**

After `/v1/admin/tokens` saves and reloads successfully, enqueue one full Python-origin snapshot to Go. A Go outage must not roll back the local token save; return sync state separately and retain a bounded retry record in `data/gateway-sync-outbox.json`.

- [ ] **Step 7: Verify tests and commit**

```bash
python3 -m unittest tests.test_gateway_sync tests.test_gateway_routes -v
git add app/services/gateway_sync app/api/internal app/api/v1/admin_api/token.py main.py config.defaults.toml tests/test_gateway_sync.py tests/test_gateway_routes.py
git commit -m "feat: synchronize legacy Web SSO accounts"
```

### Task 5: Implement Go-side private reconciliation and Python mirroring

**Files:**
- Create: `gateway/backend/internal/compat/client.go`
- Create: `gateway/backend/internal/compat/types.go`
- Create: `gateway/backend/internal/transport/http/compat/handler.go`
- Create: `gateway/backend/internal/transport/http/compat/handler_test.go`
- Modify: `gateway/backend/internal/infra/config/config.go`
- Modify: `gateway/backend/internal/infra/config/config_test.go`
- Modify: `gateway/backend/internal/application/account/service.go`
- Modify: `gateway/backend/internal/transport/http/account/handler.go`
- Modify: `gateway/backend/internal/transport/http/server.go`
- Modify: `gateway/backend/internal/app/application.go`
- Modify: `gateway/config.example.compat.yaml`

- [ ] **Step 1: Write failing Go tests**

Cover missing/wrong secret, idempotent full reconciliation, removal of absent Web accounts only, preservation of Build accounts, import mirroring, update/delete mirroring, timeout behavior, and redacted logs.

- [ ] **Step 2: Run focused tests and confirm failure**

```bash
cd gateway/backend && GOPROXY=https://proxy.golang.org,direct go test ./internal/transport/http/compat ./internal/transport/http/account ./internal/application/account
```

- [ ] **Step 3: Add compatibility configuration**

```yaml
compatibility:
  syncSecret: ""
  legacyBaseURL: "http://grok2api_python:8000"
  requestTimeout: 10s
```

Validation permits the feature to be disabled only when both URL and secret are empty; partial configuration is rejected.

- [ ] **Step 4: Add Go reconciliation methods**

The account service imports Web tokens using the existing codec, applies enabled/name metadata by `SourceKey`, lists all Web accounts in bounded pages, and removes only Web accounts absent from a full Python snapshot. Build OAuth accounts are never touched.

- [ ] **Step 5: Add private handler**

Register `/internal/compat/accounts/reconcile` before frontend fallback. Authenticate with the same constant-time secret check and never register the path through Nginx.

- [ ] **Step 6: Mirror Go-origin mutations**

After successful Web import, send the raw import documents to Python. After update/delete and batch update/delete, send source keys and changed state. Mirroring failures are logged and exposed as a warning without undoing the successful local operation.

- [ ] **Step 7: Verify all Go checks and commit**

```bash
cd gateway/backend && GOPROXY=https://proxy.golang.org,direct go test ./...
cd gateway/backend && GOPROXY=https://proxy.golang.org,direct go vet ./...
cd gateway/backend && GOPROXY=https://proxy.golang.org,direct go build ./cmd/grok2api
git add gateway/backend gateway/config.example.compat.yaml
git commit -m "feat: reconcile Go and legacy Web accounts"
```

### Task 6: Preserve admin pages and add gateway navigation/status

**Files:**
- Modify: `app/static/common/html/header.html`
- Modify: `app/static/common/js/header.js`
- Modify: `app/static/admin/pages/token.html`
- Modify: `app/static/admin/js/token.js`
- Modify: `app/static/admin/css/token.css`
- Modify: `tests/token_quota_display.test.cjs`
- Create: `tests/gateway_admin_navigation.test.cjs`

- [ ] **Step 1: Write failing static UI tests**

Assert that existing Token/Config/Cache links remain, new gateway links target `/gateway/dashboard`, `/gateway/accounts`, `/gateway/models`, `/gateway/client-keys`, `/gateway/request-audits`, `/gateway/docs`, and `/gateway/settings`, and quota rendering shows `未返回` for missing modes.

- [ ] **Step 2: Run and confirm failure**

```bash
node --test tests/token_quota_display.test.cjs tests/gateway_admin_navigation.test.cjs
```

- [ ] **Step 3: Add navigation and compact peer status**

Keep the existing visual system. Add a single `新版网关` menu group and a compact sync-status line to Token management. Do not replace the token table or duplicate the React accounts UI.

- [ ] **Step 4: Verify and commit**

```bash
node --test tests/token_quota_display.test.cjs tests/gateway_admin_navigation.test.cjs
git add app/static tests/token_quota_display.test.cjs tests/gateway_admin_navigation.test.cjs
git commit -m "feat: link preserved admin to Go gateway"
```

### Task 7: Build and smoke-test the local hybrid stack

**Files:**
- Modify as required by failures discovered in Tasks 1-6 only.

- [ ] **Step 1: Generate local secrets outside Git**

Create `data/gateway/config.yaml` from the compatibility example with fresh JWT, encryption, admin, and sync secrets. Ensure `git status` does not list it.

- [ ] **Step 2: Build**

```bash
docker compose build grok2api_python grok2api_go grok2api_edge
docker compose up -d
```

- [ ] **Step 3: Verify health and routes**

```bash
curl -fsS http://127.0.0.1:18000/health
curl -fsS http://127.0.0.1:18000/gateway/healthz
curl -fsS http://127.0.0.1:18000/gateway/readyz
```

Expected: Python and Go healthy; no Redis container exists.

- [ ] **Step 4: Run automated regression checks**

Run focused Python tests for modified modules, all Node tests, all Go tests, frontend lint/build, and `docker compose config`. Record historical unrelated Python failures separately.

- [ ] **Step 5: Browser verification**

Use Playwright or the in-app browser at desktop and mobile sizes to inspect all preserved public pages, existing admin pages, and every new gateway page. Confirm no broken assets, redirect loops, overflow, or overlapping controls.

- [ ] **Step 6: Real-account smoke tests**

Import the five local Web SSO accounts through the private sync path. Verify account count/quota, then perform one Chat, one Image, one Image Edit, one Video, one NSFW, one parent-post, and one video-extension request. Restart containers and confirm persistence.

### Task 8: Final verification, merge, push, and deploy

**Files:**
- Create: `docs/deployment/grok2api-hybrid.md`

- [ ] **Step 1: Write the operations guide**

Document secret generation, data paths, backup, canary ports, health checks, token migration, Nginx cutover, rollback to `9e1e808`, and explicit confirmation that Redis is not required.

- [ ] **Step 2: Run final verification from a clean checkout**

Run all commands from Task 7 and ensure `git status --short` contains only intentional files.

- [ ] **Step 3: Merge the integration branch into local `main`**

Use a non-destructive merge. Preserve the existing untracked files in the main worktree and do not touch `grok2api-xianyudaxian`.

- [ ] **Step 4: Push**

```bash
git push origin main
```

- [ ] **Step 5: Back up netcup**

Back up `/root/grok2api`, data, logs, browser profiles, production configuration, and the new SQLite/media paths before changing containers.

- [ ] **Step 6: Start a canary stack**

Pull the pushed commit, create production-only secrets/config, copy the current five tokens through the sync importer, and start the hybrid stack on a separate local server port.

- [ ] **Step 7: Verify real server generation**

Confirm preserved pages, new pages, five-account quota state, Chat, Image, and Video generation against the canary URL.

- [ ] **Step 8: Cut over and observe**

Switch the public Nginx target, re-run health and generation checks, inspect logs for credential leakage and repeated `401` loops, and keep the previous container available for rollback.

- [ ] **Step 9: Complete only after evidence**

Record the deployed commit, image IDs, successful checks, backup location, and rollback command in the final report.
