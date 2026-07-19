# Public Cache Management And Reference Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add permanent multi-select deletion for public image/video caches, allow cached images to become Video and NSFW references, and remove the manual parent-post ID feature.

**Architecture:** Cache list items carry an additive source/key identity. Legacy files, media assets, and owned media jobs are deleted through source-specific backend services behind authenticated public endpoints. React pages use shared cache-selection state and focused image/video cache dialogs so management and reference-picking behavior stay consistent.

**Tech Stack:** Go 1.26, Gin, GORM/SQLite, React 19, TypeScript, Radix dialogs, Lucide icons, Node contract tests.

---

### Task 1: Add Cache Deletion Identity And Legacy File Deletion

**Files:**
- Modify: `gateway/backend/internal/transport/http/cache/handler.go`
- Modify: `gateway/backend/internal/transport/http/legacy_cache_adapter.go`
- Modify: `gateway/backend/internal/transport/http/legacy_cache_adapter_test.go`

- [ ] **Step 1: Write failing adapter tests**

Add tests asserting list items expose `Source: "legacy"` and `CacheKey: filename`, and deleting a legacy image/video removes only the validated file. Include a video metadata assertion.

```go
func TestLegacyImageCacheAdapterDeletesValidatedLegacyItem(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil { t.Fatal(err) }
	name := "cached-image.jpg"
	if err := os.WriteFile(filepath.Join(root, "image", name), []byte("image"), 0o600); err != nil { t.Fatal(err) }
	adapter := &legacyImageCacheAdapter{handler: handler}
	deleted, err := adapter.DeleteImages(context.Background(), []legacyhttp.CacheDeleteTarget{{Source: "legacy", CacheKey: name}})
	if err != nil || deleted.Deleted != 1 { t.Fatalf("result=%#v err=%v", deleted, err) }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd gateway/backend && GOCACHE=/tmp/grok2api-go-cache go test ./internal/transport/http -run 'TestLegacy(Image|Video)CacheAdapterDeletes' -count=1`

Expected: compile failure because delete target/result types and adapter methods do not exist.

- [ ] **Step 3: Export validated cache deletion**

Add a public handler method that delegates to the existing safe path validation and removes stale video metadata only when no remaining video resolves to the same post ID.

```go
func (h *Handler) DeleteItem(mediaType, name string) (bool, error) {
	deleted, err := h.delete(mediaType, name)
	if err != nil || !deleted || mediaType != "video" { return deleted, err }
	postID := extractPostID(name)
	if postID != "" { _ = h.removeUnusedVideoMetadata(postID) }
	return true, nil
}
```

- [ ] **Step 4: Add cache identity types and adapter methods**

```go
type CacheDeleteTarget struct { Source string `json:"source"`; CacheKey string `json:"cache_key"` }
type CacheDeleteResult struct { Deleted int `json:"deleted"`; Skipped int `json:"skipped"`; Failed int `json:"failed"`; DeletedKeys []string `json:"deleted_keys"` }
```

Legacy list conversion sets `Source: "legacy"` and `CacheKey: item.Name`. Deletion accepts only the exact `legacy` source.

- [ ] **Step 5: Run focused tests and commit**

Run: `cd gateway/backend && GOCACHE=/tmp/grok2api-go-cache go test ./internal/transport/http ./internal/transport/http/cache -count=1`

Commit: `feat: add legacy cache deletion primitives`

### Task 2: Add Media Asset And Owned Media Job Deletion

**Files:**
- Modify: `gateway/backend/internal/repository/media.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/media_repository.go`
- Modify: `gateway/backend/internal/infra/persistence/relational/media_repository_test.go`
- Modify: `gateway/backend/internal/application/media/service.go`
- Modify: `gateway/backend/internal/application/media/service_test.go`
- Modify: `gateway/backend/internal/application/gateway/video.go`
- Modify: `gateway/backend/internal/application/gateway/video_test.go`

- [ ] **Step 1: Write failing repository and service tests**

```go
func TestDeleteMediaJobRequiresOwnerAndTerminalStatus(t *testing.T) {
	repo := NewMediaJobRepository(openTestDatabase(t))
	// Create one completed job for key 7 and one running job for key 7.
	deleted, err := repo.DeleteMediaJob(context.Background(), "completed", 7)
	if err != nil || !deleted { t.Fatalf("deleted=%v err=%v", deleted, err) }
	if deleted, _ := repo.DeleteMediaJob(context.Background(), "running", 7); deleted { t.Fatal("running job deleted") }
	if deleted, _ := repo.DeleteMediaJob(context.Background(), "other-owner", 7); deleted { t.Fatal("foreign job deleted") }
}
```

- [ ] **Step 2: Verify RED**

Run: `cd gateway/backend && GOCACHE=/tmp/grok2api-go-cache go test ./internal/infra/persistence/relational ./internal/application/media ./internal/application/gateway -run 'TestDelete(MediaJob|ImageAsset)' -count=1`

- [ ] **Step 3: Add repository contracts**

```go
type MediaJobRepository interface {
	// existing methods
	DeleteMediaJob(ctx context.Context, id string, clientKeyID uint64) (bool, error)
}
```

The SQL delete condition is `id = ? AND client_key_id = ? AND status IN ?` with completed/failed statuses.

- [ ] **Step 4: Add media image deletion**

```go
func (s *Service) DeleteImage(ctx context.Context, id string) (bool, error) {
	asset, err := s.assets.GetMediaAsset(ctx, strings.TrimSpace(id))
	if errors.Is(err, repository.ErrNotFound) { return false, nil }
	if err != nil || asset.Kind != "image" { return false, err }
	if err := s.objects.Delete(ctx, asset.StorageKey); err != nil && !errors.Is(err, os.ErrNotExist) { return false, err }
	if err := s.assets.DeleteMediaAsset(ctx, asset.ID); err != nil && !errors.Is(err, repository.ErrNotFound) { return false, err }
	s.totalBytes.Add(-asset.SizeBytes)
	return true, nil
}
```

- [ ] **Step 5: Add owned video history deletion to gateway service**

```go
func (s *Service) DeleteVideo(ctx context.Context, id string, key clientkey.Key) (bool, error) {
	return s.mediaJobs.DeleteMediaJob(ctx, strings.TrimSpace(id), key.ID)
}
```

- [ ] **Step 6: Run tests and commit**

Commit: `feat: delete owned cached media records`

### Task 3: Add Authenticated Public Cache Delete Endpoints

**Files:**
- Modify: `gateway/backend/internal/transport/http/legacy/image.go`
- Modify: `gateway/backend/internal/transport/http/legacy/image_test.go`
- Modify: `gateway/backend/internal/transport/http/legacy/video.go`
- Modify: `gateway/backend/internal/transport/http/legacy/video_test.go`
- Modify: `gateway/backend/internal/transport/http/legacy_cache_adapter.go`
- Modify: `gateway/backend/internal/transport/http/server.go`

- [ ] **Step 1: Write failing route tests**

Test `POST /v1/public/imagine/cache/delete` and `POST /v1/public/video/cache/delete` with authenticated requests, bounded target arrays, partial results, and unauthenticated rejection.

```json
{"items":[{"source":"mediaAsset","cache_key":"img_test"}]}
```

- [ ] **Step 2: Verify RED**

Run: `cd gateway/backend && GOCACHE=/tmp/grok2api-go-cache go test ./internal/transport/http/legacy -run 'Test(Image|Video)CacheDelete' -count=1`

- [ ] **Step 3: Extend list DTOs**

Image and video list responses add `source` and `cache_key`. Database video conversion uses `Source: "mediaJob"` and `CacheKey: job.ID`; media images use `Source: "mediaAsset"` and `CacheKey: asset.ID`.

- [ ] **Step 4: Implement bounded deletion handlers**

Reject empty requests and more than 200 targets. Return HTTP 200 for partial results and sanitized HTTP 500 only when no source operation can complete.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: expose public cache deletion endpoints`

### Task 4: Add Frontend Cache APIs And Shared Selection State

**Files:**
- Modify: `gateway/frontend/src/public/api/contracts.mjs`
- Modify: `gateway/frontend/src/public/features/image/image-api.ts`
- Modify: `gateway/frontend/src/public/features/video/video-api.ts`
- Create: `gateway/frontend/src/public/features/cache/cache-selection.ts`
- Test: `gateway/frontend/test/public-api-contract.test.mjs`

- [ ] **Step 1: Write failing runtime tests**

```js
test("cache selection removes only confirmed deleted keys", async () => {
  const state = cacheSelection.createCacheSelection(["a", "b"]);
  assert.deepEqual(cacheSelection.removeDeletedSelection(state, ["a"]), new Set(["b"]));
});
```

- [ ] **Step 2: Verify RED**

Run: `cd gateway/frontend && node --test test/public-api-contract.test.mjs`

- [ ] **Step 3: Add typed deletion identity and APIs**

```ts
export type CacheIdentity = { source: "legacy" | "mediaAsset" | "mediaJob"; cacheKey: string };
export type CacheDeleteResult = { deleted: number; skipped: number; failed: number; deleted_keys: string[] };
```

Add `deleteCachedImages` and `deleteCachedVideos` using the new endpoints.

- [ ] **Step 4: Add immutable selection helpers**

Helpers toggle one key, select visible keys, clear selection, and remove only confirmed deleted keys.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: add public cache selection APIs`

### Task 5: Build Image And Video Cache Dialog Components

**Files:**
- Create: `gateway/frontend/src/public/components/image-cache-dialog.tsx`
- Create: `gateway/frontend/src/public/components/video-cache-dialog.tsx`
- Modify: `gateway/frontend/src/public/components/image-grid.tsx`
- Modify: `gateway/frontend/src/public/components/video-grid.tsx`
- Modify: `gateway/frontend/test/public-app-contract.test.mjs`

- [ ] **Step 1: Write failing component contracts**

Require inset selection controls, selected-count toolbar, delete confirmation, picker-mode confirmation, and sticky mobile toolbar classes.

- [ ] **Step 2: Verify RED**

Run: `cd gateway/frontend && node --test test/public-app-contract.test.mjs`

- [ ] **Step 3: Implement focused dialogs**

Both dialogs accept `items`, `loading`, `mode`, `selected`, `onSelectedChange`, `onDelete`, and page-specific action callbacks. Picker mode shows `添加所选`; manage mode shows `删除所选`.

- [ ] **Step 4: Keep card geometry stable**

Selection buttons are `absolute left-2 top-2 size-8`, use `ring-inset`, and do not alter card dimensions. Toolbars use `sticky top-0 z-10` inside the scroll container.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: add reusable public cache dialogs`

### Task 6: Remove Manual ID UI And Integrate Image Cache Management

**Files:**
- Modify: `gateway/frontend/src/public/pages/workbench-page.tsx`
- Modify: `gateway/frontend/src/public/pages/imagine-page.tsx`
- Modify: `gateway/frontend/src/public/features/image/image-api.ts`
- Modify: `gateway/frontend/src/public/api/contracts.mjs`
- Modify: `gateway/frontend/src/public/lib/media.ts`
- Modify: `gateway/backend/internal/transport/http/legacy/image.go`
- Modify: `gateway/backend/internal/transport/http/legacy/image_test.go`
- Modify: `gateway/frontend/test/public-api-contract.test.mjs`
- Modify: `gateway/frontend/test/public-app-contract.test.mjs`

- [ ] **Step 1: Write failing absence tests**

Assert Workbench no longer contains `parentInput`, `addParent`, `resolveParentPost`, or `parentPostId`; Video is covered in Task 7. Assert the endpoint contract and backend route are absent.

- [ ] **Step 2: Verify RED**

Run frontend and focused legacy tests; expected failures identify the still-present route and controls.

- [ ] **Step 3: Remove manual ID feature**

Delete only the visible resolver flow and route. Preserve internal `parentPostID`, `parentPostIDPattern`, `imaginePublicImageURL`, and edit fallback behavior.

- [ ] **Step 4: Replace inline cache markup with ImageCacheDialog**

Workbench uses picker mode to append references; Imagine uses manage mode plus preview/edit callbacks. Both call `deleteCachedImages` and remove confirmed deleted entries from local state.

- [ ] **Step 5: Run tests and commit**

Commit: `feat: manage image cache and remove id input`

### Task 7: Integrate Video And NSFW Reference Picking And Video Deletion

**Files:**
- Modify: `gateway/frontend/src/public/pages/video-page.tsx`
- Modify: `gateway/frontend/src/public/pages/nsfw-page.tsx`
- Modify: `gateway/frontend/test/public-app-contract.test.mjs`

- [ ] **Step 1: Write failing workflow contracts**

Assert Video opens image cache in picker mode, appends selected cached images with duplicate and eight-item guards, and contains no manual parent ID controls. Assert NSFW selects one cached image and clears `localImage`. Assert both video cache dialogs expose deletion.

- [ ] **Step 2: Verify RED**

Run: `cd gateway/frontend && node --test test/public-app-contract.test.mjs`

- [ ] **Step 3: Implement Video picker flow**

Add cached image state, image cache dialog state, selected cache keys, and `addCachedReferences`. Use each image's `sourceURL` as `UploadAsset.data`.

- [ ] **Step 4: Implement NSFW picker flow**

Picker selection calls `setSelected(image)`, `setLocalImage(null)`, closes the dialog, and keeps the existing video-generation reference path unchanged.

- [ ] **Step 5: Integrate video cache management**

Video and NSFW call `deleteCachedVideos`, retain failed selections, and clear active/extension state if the active cached video was deleted.

- [ ] **Step 6: Run tests and commit**

Commit: `feat: use cached images as video references`

### Task 8: Verify Cache Management End To End

**Files:**
- Test only

- [ ] **Step 1: Run backend verification**

Run focused cache/media/legacy tests, `go vet ./...`, and `go build ./cmd/grok2api` with `GOCACHE=/tmp/grok2api-go-cache`.

- [ ] **Step 2: Run frontend verification**

Run `node --test test/*.mjs` and `pnpm build`.

- [ ] **Step 3: Review diff and commit verification fixes**

Run `git diff --check` and inspect all public cache routes and dialog call sites.

- [ ] **Step 4: Production smoke test after deployment**

Verify image/video list counts, delete one disposable image and video, confirm repeated deletion is skipped, and confirm Video/NSFW references can start a request without ID input.

