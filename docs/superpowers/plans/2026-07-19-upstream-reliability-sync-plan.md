# Upstream Reliability Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Selectively port the reliability and account-management behavior from `chenyme/main` at `b28133e` into the customized Go/React gateway.

**Architecture:** Upstream commits are reference patches. Backend behavior is adapted into `gateway/backend`; admin UI behavior is integrated into existing `gateway/frontend` pages and components. Customized public workspaces, media cache, video extension logic, SQLite/memory runtime, and WARP deployment remain authoritative.

**Tech Stack:** Go 1.26, Gin, GORM/SQLite, React 19, TypeScript, Node contract tests.

---

### Task 1: Update Build Compatibility And Clipboard Reliability

**Files:**
- Modify: `gateway/backend/internal/infra/config/config.go`
- Modify: `gateway/backend/internal/infra/config/config_test.go`
- Modify: `gateway/backend/internal/infra/provider/cli/adapter.go`
- Modify: `gateway/frontend/src/shared/clipboard.ts`
- Modify: `gateway/frontend/src/features/client-keys/client-keys-page.tsx`
- Modify: `gateway/frontend/test/gateway-path-contract.test.mjs`

- [ ] **Step 1: Write failing tests for `0.2.103` and secure-context clipboard fallback**

```go
func TestDefaultGrokBuildClientVersionMatchesLocalBaseline(t *testing.T) {
	if RecommendedBuildClientVersion != "0.2.103" { t.Fatalf("version=%q", RecommendedBuildClientVersion) }
}
```

- [ ] **Step 2: Verify RED**

Run focused config and frontend contract tests.

- [ ] **Step 3: Port upstream compatibility constants and clipboard behavior**

Use async Clipboard API only when `globalThis.isSecureContext === true`; otherwise immediately use the hidden textarea fallback. Always display the returned client key secret in the secret dialog.

- [ ] **Step 4: Run tests and commit**

Commit: `fix: sync build compatibility and clipboard fallback`

### Task 2: Persist SSO Rejection Reliably Across All Request Types

**Files:**
- Modify: `gateway/backend/internal/application/account/service.go`
- Modify: `gateway/backend/internal/application/account/credential_refresh_test.go`
- Modify: `gateway/backend/internal/application/account/quota_refresh_test.go`
- Modify: `gateway/backend/internal/application/gateway/service.go`
- Modify: `gateway/backend/internal/application/gateway/service_test.go`
- Modify: `gateway/backend/internal/application/gateway/image.go`
- Modify: `gateway/backend/internal/application/gateway/video.go`

- [ ] **Step 1: Write failing cancellation and retry tests**

Tests cancel the client context after a provider `401`, then assert the account becomes `reauthRequired`, selector cache is invalidated, and the next attempt uses another account.

- [ ] **Step 2: Verify RED**

Run focused account and gateway tests.

- [ ] **Step 3: Add bounded finalization helpers**

```go
func (s *Service) markSSOCredentialRejected(ctx context.Context, value account.Credential, reason string) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.MarkReauthRequired(writeCtx, value.ID, reason)
}
```

Gateway adds `isSSOCredentialRejected` and a non-returning helper that persists state, logs write failures, and calls `selector.MarkQuotaStateChanged`.

- [ ] **Step 4: Apply to chat, image, video, quota, conversion, and owned response paths**

Transport errors wrapping `provider.ErrUnauthorized` and HTTP `401` follow the same path. Image/chat requests release the failed lease and continue to the next account where retry semantics permit it.

- [ ] **Step 5: Run tests and commit**

Commit: `fix: persist rejected sso credentials reliably`

### Task 3: Port Responses Remote Compaction And Stream Terminal Validation

**Files:**
- Create: `gateway/backend/internal/application/gateway/responses_compaction.go`
- Create: `gateway/backend/internal/application/gateway/responses_compaction_test.go`
- Create: `gateway/backend/internal/infra/provider/cli/responses_compaction.go`
- Create: `gateway/backend/internal/infra/provider/cli/responses_compaction_forward.go`
- Create: `gateway/backend/internal/infra/provider/cli/responses_compaction_prompt.txt`
- Create: `gateway/backend/internal/infra/provider/cli/responses_reasoning_recovery.go`
- Modify: `gateway/backend/internal/infra/provider/cli/adapter.go`
- Modify: `gateway/backend/internal/infra/provider/cli/responses_history.go`
- Modify: `gateway/backend/internal/infra/provider/cli/responses_tool_state.go`
- Modify: `gateway/backend/internal/transport/http/inference/handler.go`
- Modify: `gateway/backend/internal/transport/http/inference/handler_test.go`

- [ ] **Step 1: Port upstream tests first and verify RED**

Bring over compaction trigger validation, usage normalization, reasoning recovery, and incomplete stream tests from upstream commits `a88036b` and `2e5a30d`, changing import paths only where local structure differs.

- [ ] **Step 2: Implement provider compaction forwarding**

Preserve exactly one final `compaction_trigger`, normalize required usage integers, and synthesize the protocol-correct compact response without retaining request bodies.

- [ ] **Step 3: Add protocol-aware stream terminal inspection**

Responses require `response.completed`, Chat accepts `[DONE]`, Anthropic requires `message_stop`, and Image requires `image_generation.completed`. Failed/incomplete/read-interrupted streams receive distinct internal audit codes.

- [ ] **Step 4: Run tests and commit**

Commit: `fix: stabilize responses compaction streams`

### Task 4: Add Web Account Setup Actions And Identity Synchronization

**Files:**
- Create: `gateway/backend/internal/application/account/web_account_scripts.go`
- Create: `gateway/backend/internal/application/account/web_account_settings.go`
- Create: `gateway/backend/internal/infra/provider/web/account_settings.go`
- Create: `gateway/backend/internal/transport/http/account/web_account_scripts.go`
- Modify: `gateway/backend/internal/domain/account/account.go`
- Modify: `gateway/backend/internal/repository/account.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/account_repository.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/mapping.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/models.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/schema.go`
- Modify: `gateway/backend/internal/infra/provider/provider.go`
- Modify: `gateway/backend/internal/infra/provider/web/adapter.go`
- Modify: `gateway/backend/internal/infra/provider/web/quota.go`
- Modify: `gateway/backend/internal/application/account/service.go`
- Modify: `gateway/backend/internal/transport/http/account/handler.go`
- Create: `gateway/frontend/src/features/accounts/web-account-scripts.tsx`
- Create: `gateway/frontend/src/features/accounts/web-account-settings.tsx`
- Modify: `gateway/frontend/src/features/accounts/accounts-api.ts`
- Modify: `gateway/frontend/src/features/accounts/accounts-page.tsx`

- [ ] **Step 1: Port upstream tests for ToS, birth date, NSFW, and identity reconciliation**

Use upstream commits `eb70ea4`, `4a56afb`, `a60e92e`, and `a5f87e0` as the behavioral source. Verify tests fail before implementation.

- [ ] **Step 2: Add idempotent account fields and migrations**

Persist terms accepted time/version, birth-date set time, NSFW enabled time, and synchronized identity fields without invalidating existing rows.

- [ ] **Step 3: Implement provider and service actions**

Each action checks provider/auth type, refreshes the effective credential, performs the upstream request, and records completion only after success. Batch scripts reuse the existing account task pool and progress stream.

- [ ] **Step 4: Integrate compact admin UI controls**

Add per-account setup status/actions and batch setup dialog using existing account table, menus, dialogs, and translations. Do not replace customized account layout.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: add grok web account setup actions`

### Task 5: Add Selected Token Refresh, Cleanup, And Export Improvements

**Files:**
- Modify: `gateway/backend/internal/application/account/service.go`
- Add/modify account cleanup, export, credential refresh tests
- Modify: `gateway/backend/internal/repository/account.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/account_repository.go`
- Modify: `gateway/backend/internal/transport/http/account/handler.go`
- Modify: `gateway/frontend/src/features/accounts/accounts-api.ts`
- Modify: `gateway/frontend/src/features/accounts/accounts-page.tsx`
- Modify: `gateway/frontend/src/shared/i18n/index.ts`

- [ ] **Step 1: Port failing upstream cleanup and selected-refresh tests**

Verify healthy/probing/reset-pending accounts remain, unsupported or inactive selected accounts are skipped, and provider export preserves refreshed credentials and setup timestamps.

- [ ] **Step 2: Implement repository batch deletion by status**

Delete in batches of 500 using provider, status, and current time filters. Return deleted IDs so sticky state and refresh state are cleared.

- [ ] **Step 3: Add service and HTTP operations**

Expose `POST /accounts/cleanup` and `POST /accounts/batch/refresh-tokens`. Validate provider and bounded unique IDs.

- [ ] **Step 4: Integrate account page actions**

Selected rows gain refresh-token action. Provider toolbar gains cleanup confirmation with status checkboxes. Export continues to download provider-scoped credentials.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: add account refresh and cleanup workflows`

### Task 6: Verify Upstream Sync Without Regressing Custom Features

**Files:**
- Test only

- [ ] **Step 1: Run backend focused and broad verification**

Run focused changed-package tests, `go vet ./...`, and `go build ./cmd/grok2api`. Run network-listening tests outside the restricted sandbox when approval is available.

- [ ] **Step 2: Run frontend verification**

Run all Node contract tests and `pnpm build`.

- [ ] **Step 3: Re-run custom media contracts**

Confirm public React routes, image cache, video playback, video extension result separation, multi-account media retry, and no-Redis configuration remain intact.

- [ ] **Step 4: Review upstream coverage**

Compare local markers against commits `a88036b`, `3412e65`, `eb70ea4`, `4a56afb`, `a60e92e`, `a5f87e0`, `2e5a30d`, and `9714712`; document intentionally excluded README/sponsor/static changes.

- [ ] **Step 5: Deploy and smoke test**

Push `HEAD:main`, update the detached production checkout to `origin/main`, rebuild only `grok2api_go`, force recreate if image IDs differ, wait for health, and verify WARP/data volumes remain untouched.
