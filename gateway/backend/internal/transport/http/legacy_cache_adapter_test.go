package httpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	cachehttp "github.com/chenyme/grok2api/backend/internal/transport/http/cache"
	legacyhttp "github.com/chenyme/grok2api/backend/internal/transport/http/legacy"
)

type fakeLegacyImageMediaLibrary struct {
	assets   []mediadomain.Asset
	listErr  error
	listCall int
}

func (f *fakeLegacyImageMediaLibrary) AdminListImages(ctx context.Context, page, pageSize int, _ string) ([]mediadomain.Asset, int64, error) {
	f.listCall++
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	start := min((page-1)*pageSize, len(f.assets))
	end := min(start+pageSize, len(f.assets))
	return f.assets[start:end], int64(len(f.assets)), nil
}

func (f *fakeLegacyImageMediaLibrary) PublicImageURL(id string) string {
	return "https://grok.example/v1/media/images/" + id
}

func TestLegacyVideoCacheAdapterRenamesByMigratedFilename(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "generated-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "video", name), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &legacyVideoCacheAdapter{handler: handler}
	item, err := adapter.RenameVideo(name, "Renamed local")
	if err != nil {
		t.Fatal(err)
	}
	if item.PostID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || item.DisplayName != "Renamed local" || item.ViewURL == "" {
		t.Fatalf("item=%#v", item)
	}
}

func TestLegacyImageCacheAdapterListsCachedImages(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "cached-reference.jpg"
	if err := os.WriteFile(filepath.Join(root, "image", name), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := &legacyImageCacheAdapter{handler: handler}
	items, err := adapter.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != name || items[0].ViewURL != "/v1/files/image/"+name || items[0].Source != "legacy" || items[0].CacheKey != name {
		t.Fatalf("items=%#v", items)
	}
}

func TestLegacyVideoCacheAdapterSetsDeletionIdentity(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "generated-123e4567-e89b-12d3-a456-426614174000-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "video", name), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := &legacyVideoCacheAdapter{handler: handler}
	items, err := adapter.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "legacy" || items[0].CacheKey != name {
		t.Fatalf("items=%#v", items)
	}
}

func TestLegacyCacheAdaptersDeleteLegacyItems(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	imageName := "cached.jpg"
	videoName := "generated-123e4567-e89b-12d3-a456-426614174000-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "image", imageName), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", videoName), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	imageAdapter := &legacyImageCacheAdapter{handler: handler}
	videoAdapter := &legacyVideoCacheAdapter{handler: handler}
	imageResult, err := imageAdapter.DeleteImages(context.Background(), []legacyhttp.CacheDeleteTarget{{Source: "legacy", CacheKey: imageName}})
	if err != nil || imageResult.Deleted != 1 || imageResult.Skipped != 0 || imageResult.Failed != 0 || len(imageResult.DeletedKeys) != 1 || imageResult.DeletedKeys[0] != imageName {
		t.Fatalf("image result=%#v err=%v", imageResult, err)
	}
	videoResult, err := videoAdapter.DeleteVideos([]legacyhttp.CacheDeleteTarget{{Source: "legacy", CacheKey: videoName}})
	if err != nil || videoResult.Deleted != 1 || videoResult.Skipped != 0 || videoResult.Failed != 0 || len(videoResult.DeletedKeys) != 1 || videoResult.DeletedKeys[0] != videoName {
		t.Fatalf("video result=%#v err=%v", videoResult, err)
	}
	if _, err := os.Stat(filepath.Join(root, "image", imageName)); !os.IsNotExist(err) {
		t.Fatalf("image was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "video", videoName)); !os.IsNotExist(err) {
		t.Fatalf("video was not deleted: %v", err)
	}
}

func TestLegacyCacheAdaptersReportMissingAndFailedItems(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "cached.jpg"
	if err := os.WriteFile(filepath.Join(root, "image", name), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &legacyImageCacheAdapter{handler: handler}
	result, err := adapter.DeleteImages(context.Background(), []legacyhttp.CacheDeleteTarget{
		{Source: "legacy", CacheKey: name},
		{Source: "legacy", CacheKey: "missing.jpg"},
		{Source: "legacy", CacheKey: "foo/bar.jpg"},
	})
	if err != nil || result.Deleted != 1 || result.Skipped != 1 || result.Failed != 1 || len(result.DeletedKeys) != 1 || result.DeletedKeys[0] != name {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestLegacyCacheAdaptersRejectNonLegacyItems(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	imageName := "cached.jpg"
	videoName := "generated-123e4567-e89b-12d3-a456-426614174000-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "image", imageName), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", videoName), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	imageAdapter := &legacyImageCacheAdapter{handler: handler}
	if _, err := imageAdapter.DeleteImages(context.Background(), []legacyhttp.CacheDeleteTarget{{Source: "mediaAsset", CacheKey: imageName}}); err == nil {
		t.Fatal("non-legacy image item was accepted")
	}
	videoAdapter := &legacyVideoCacheAdapter{handler: handler}
	if _, err := videoAdapter.DeleteVideos([]legacyhttp.CacheDeleteTarget{{Source: "mediaJob", CacheKey: videoName}}); err == nil {
		t.Fatal("non-legacy video item was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "image", imageName)); err != nil {
		t.Fatalf("image was deleted after rejected request: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "video", videoName)); err != nil {
		t.Fatalf("video was deleted after rejected request: %v", err)
	}
}

func TestLegacyVideoCacheAdapterRejectsDeletionWithoutHandler(t *testing.T) {
	adapter := &legacyVideoCacheAdapter{}
	if _, err := adapter.DeleteVideos([]legacyhttp.CacheDeleteTarget{{Source: "legacy", CacheKey: "cached.mp4"}}); err == nil {
		t.Fatal("video deletion without handler did not return an error")
	}
}

func TestLegacyImageCacheAdapterMergesMediaLibraryImages(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	legacyName := "cached-reference.jpg"
	legacyPath := filepath.Join(root, "image", legacyName)
	if err := os.WriteFile(legacyPath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTime := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(legacyPath, legacyTime, legacyTime); err != nil {
		t.Fatal(err)
	}
	mediaTime := legacyTime.Add(time.Hour)
	library := &fakeLegacyImageMediaLibrary{assets: []mediadomain.Asset{{
		ID: "img_edited_result", Kind: "image", MIMEType: "image/png", SizeBytes: 321, CreatedAt: mediaTime,
	}}}

	adapter := newLegacyImageCacheAdapter(handler, library)
	items, err := adapter.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].Name != "img_edited_result.png" || items[0].ViewURL != "https://grok.example/v1/media/images/img_edited_result" || items[0].ModifiedAtMS != mediaTime.UnixMilli() {
		t.Fatalf("media item=%#v", items[0])
	}
	if items[1].Name != legacyName {
		t.Fatalf("legacy item=%#v", items[1])
	}
}

func TestLegacyImageCacheAdapterKeepsLegacyImagesWhenMediaListingFails(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "legacy-still-available.jpg"
	if err := os.WriteFile(filepath.Join(root, "image", name), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := newLegacyImageCacheAdapter(handler, &fakeLegacyImageMediaLibrary{listErr: errors.New("media database unavailable")})
	items, err := adapter.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != name {
		t.Fatalf("items=%#v", items)
	}
}

func TestLegacyImageCacheAdapterUsesRequestContextForMediaListing(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image", "legacy.jpg"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	media := &fakeLegacyImageMediaLibrary{assets: []mediadomain.Asset{{ID: "img_cancelled", CreatedAt: time.Now().UTC()}}}
	adapter := newLegacyImageCacheAdapter(handler, media)
	if _, err := adapter.ListImages(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestLegacyImageCacheAdapterPaginatesMediaOnlyLibrary(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	assets := make([]mediadomain.Asset, 101)
	for index := range assets {
		assets[index] = mediadomain.Asset{
			ID:   fmt.Sprintf("img_page_%03d", index),
			Kind: "image", MIMEType: "image/jpeg", SizeBytes: int64(index + 1), CreatedAt: createdAt.Add(-time.Duration(index) * time.Minute),
		}
	}
	media := &fakeLegacyImageMediaLibrary{assets: assets}
	cache := newLegacyImageCacheAdapter(nil, media)
	items, err := cache.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(assets) || media.listCall != 2 {
		t.Fatalf("items=%d calls=%d", len(items), media.listCall)
	}
	if items[0].Name != assets[0].ID+".jpg" || items[len(items)-1].Name != assets[len(assets)-1].ID+".jpg" {
		t.Fatalf("first=%#v last=%#v", items[0], items[len(items)-1])
	}
}
