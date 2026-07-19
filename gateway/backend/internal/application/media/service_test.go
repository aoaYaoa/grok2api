package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	localmedia "github.com/chenyme/grok2api/backend/internal/infra/media"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestServicePersistsAndReopensImage(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(relational.NewMediaAssetRepository(database), relational.NewMediaJobRepository(database), objects, nil, Config{
		PublicBaseURL: "https://api.example", MaxImageBytes: 32 << 20, MaxTotalBytes: 1 << 30,
		CleanupThresholdPercent: 80, CleanupInterval: 10 * time.Minute,
	})
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	asset, err := service.SaveImage(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if asset.MIMEType != "image/png" || asset.SizeBytes != int64(len(raw)) || len(asset.SHA256) != 64 {
		t.Fatalf("asset = %#v", asset)
	}
	if got := service.PublicImageURL(asset.ID); got != "https://api.example/v1/media/images/"+asset.ID {
		t.Fatalf("public URL = %q", got)
	}
	stored, body, err := service.OpenImage(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || stored.ID != asset.ID || !bytes.Equal(data, raw) {
		t.Fatalf("stored=%#v size=%d err=%v", stored, len(data), err)
	}
	if _, err := service.SaveImage(ctx, []byte("not an image")); err == nil {
		t.Fatal("invalid image content was accepted")
	}
}

func TestServiceDeleteImageRemovesObjectMetadataAndAccountingOnce(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	assets := relational.NewMediaAssetRepository(database)
	service := NewService(assets, relational.NewMediaJobRepository(database), objects, nil, Config{
		MaxImageBytes: 32 << 20, MaxTotalBytes: 1 << 30, CleanupThresholdPercent: 80, CleanupInterval: time.Minute,
	})
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	asset, err := service.SaveImage(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := service.DeleteImage(ctx, "  "+asset.ID+"  ")
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	if service.totalBytes.Load() != 0 {
		t.Fatalf("total bytes after delete = %d", service.totalBytes.Load())
	}
	if _, err := assets.GetMediaAsset(ctx, asset.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("metadata still exists: %v", err)
	}
	if _, err := objects.Open(ctx, asset.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("object still exists: %v", err)
	}

	deleted, err = service.DeleteImage(ctx, asset.ID)
	if err != nil || deleted {
		t.Fatalf("repeat delete = %v, %v", deleted, err)
	}
	if service.totalBytes.Load() != 0 {
		t.Fatalf("total bytes after repeat delete = %d", service.totalBytes.Load())
	}
}

func TestServiceDeleteImageToleratesMissingObject(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-delete-race.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	baseAssets := relational.NewMediaAssetRepository(database)
	service := NewService(baseAssets, nil, objects, nil, Config{MaxTotalBytes: 1 << 30, CleanupThresholdPercent: 80, CleanupInterval: time.Minute})
	asset := testDeleteAsset("img_delete_missing", "images/mi/img_delete_missing.png")
	if err := baseAssets.CreateMediaAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	deleted, err := service.DeleteImage(ctx, asset.ID)
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	if service.totalBytes.Load() != 0 {
		t.Fatalf("total bytes = %d", service.totalBytes.Load())
	}
	if _, err := baseAssets.GetMediaAsset(ctx, asset.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("metadata still exists: %v", err)
	}
}

func TestServiceDeleteImageRejectsNonImageAssets(t *testing.T) {
	asset := testDeleteAsset("video_asset_0001", "videos/video_asset_0001.mp4")
	asset.Kind = "video"
	assets := &nonImageAssetRepository{asset: asset}
	service := NewService(assets, nil, nil, nil, Config{})

	deleted, err := service.DeleteImage(context.Background(), asset.ID)
	if !errors.Is(err, ErrAssetNotFound) || deleted {
		t.Fatalf("delete non-image = %v, %v", deleted, err)
	}
	if assets.deleteCalls != 0 {
		t.Fatalf("non-image metadata delete calls = %d", assets.deleteCalls)
	}
}

func TestServiceDeleteImageSkipsAssetsReferencedByActiveJobs(t *testing.T) {
	ctx := context.Background()
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	asset := testDeleteAsset("img_delete_active_reference", "")
	asset.SizeBytes = int64(len(raw))
	asset.StorageKey, err = objects.SaveImage(ctx, asset.ID, asset.MIMEType, raw)
	if err != nil {
		t.Fatal(err)
	}
	assets := &activeReferencedAssetRepository{asset: asset}
	service := NewService(assets, nil, objects, nil, Config{})

	deleted, err := service.DeleteImage(ctx, asset.ID)
	if err != nil || deleted {
		t.Fatalf("delete referenced image = %v, %v", deleted, err)
	}
	if assets.deleteCalls != 0 || service.totalBytes.Load() != asset.SizeBytes {
		t.Fatalf("metadata delete calls = %d, total bytes = %d", assets.deleteCalls, service.totalBytes.Load())
	}
	body, err := objects.Open(ctx, asset.StorageKey)
	if err != nil {
		t.Fatalf("referenced object was deleted: %v", err)
	}
	_ = body.Close()
}

func TestServiceDeleteImageDeletesMetadataBeforeObject(t *testing.T) {
	asset := testDeleteAsset("img_delete_ordered", "images/order.png")
	order := make([]string, 0, 2)
	assets := &orderedDeleteAssetRepository{asset: asset, order: &order}
	objects := &orderedDeleteObjectStorage{order: &order}
	service := NewService(assets, nil, objects, nil, Config{})

	deleted, err := service.DeleteImage(context.Background(), asset.ID)
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	if len(order) != 2 || order[0] != "metadata" || order[1] != "object" {
		t.Fatalf("delete order = %#v", order)
	}
}

func TestServiceDeleteImageRestoresMetadataWhenObjectDeleteFails(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-delete-restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	baseObjects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	objects := &failingObjectStorage{MediaObjectStorage: baseObjects, failures: 1}
	assets := relational.NewMediaAssetRepository(database)
	service := NewService(assets, nil, objects, nil, Config{MaxImageBytes: 32 << 20, MaxTotalBytes: 1 << 30, CleanupThresholdPercent: 80, CleanupInterval: time.Minute})
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	asset, err := service.SaveImage(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := service.DeleteImage(ctx, asset.ID)
	if err == nil || deleted {
		t.Fatalf("failed delete = %v, %v", deleted, err)
	}
	if _, err := assets.GetMediaAsset(ctx, asset.ID); err != nil {
		t.Fatalf("metadata was not restored: %v", err)
	}
	body, err := objects.Open(ctx, asset.StorageKey)
	if err != nil {
		t.Fatalf("object was not retained for retry: %v", err)
	}
	_ = body.Close()
	if service.totalBytes.Load() != asset.SizeBytes {
		t.Fatalf("total bytes after failed delete = %d", service.totalBytes.Load())
	}

	deleted, err = service.DeleteImage(ctx, asset.ID)
	if err != nil || !deleted {
		t.Fatalf("retry delete = %v, %v", deleted, err)
	}
	if _, err := assets.GetMediaAsset(ctx, asset.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("metadata remained after retry: %v", err)
	}
	if _, err := objects.Open(ctx, asset.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("object remained after retry: %v", err)
	}
	if service.totalBytes.Load() != 0 {
		t.Fatalf("total bytes after retry = %d", service.totalBytes.Load())
	}
}

func TestMediaObjectDeleteErrorJoinsRestoreFailure(t *testing.T) {
	deleteErr := errors.New("delete failed")
	restoreErr := errors.New("restore failed")
	err := mediaObjectDeleteError(deleteErr, restoreErr)
	if !errors.Is(err, deleteErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("joined error = %v", err)
	}
}

func TestServiceDeleteImageInitializesExactRemainingTotalBytes(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-delete-uninitialized.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	assets := relational.NewMediaAssetRepository(database)
	values := []mediadomain.Asset{
		testDeleteAsset("img_delete_uninitialized_0001", ""),
		testDeleteAsset("img_delete_uninitialized_0002", ""),
	}
	for index := range values {
		values[index].SizeBytes = int64(len(raw))
		values[index].StorageKey, err = objects.SaveImage(ctx, values[index].ID, values[index].MIMEType, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := assets.CreateMediaAsset(ctx, values[index]); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(assets, nil, objects, nil, Config{})

	deleted, err := service.DeleteImage(ctx, values[0].ID)
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	if service.totalBytes.Load() != values[1].SizeBytes {
		t.Fatalf("total bytes = %d", service.totalBytes.Load())
	}
}

func TestServiceConcurrentSaveAndDeleteKeepExactTotalBytes(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-save-delete-concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	assets := relational.NewMediaAssetRepository(database)
	values := []mediadomain.Asset{
		testDeleteAsset("img_concurrent_delete_0001", ""),
		testDeleteAsset("img_concurrent_remain_0002", ""),
	}
	for index := range values {
		values[index].SizeBytes = int64(len(raw))
		values[index].StorageKey, err = objects.SaveImage(ctx, values[index].ID, values[index].MIMEType, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := assets.CreateMediaAsset(ctx, values[index]); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(assets, nil, objects, nil, Config{MaxImageBytes: 32 << 20, MaxTotalBytes: 1 << 30, CleanupThresholdPercent: 80, CleanupInterval: time.Minute})

	start := make(chan struct{})
	saveResult := make(chan error, 1)
	deleteResult := make(chan error, 1)
	go func() {
		<-start
		_, saveErr := service.SaveImage(ctx, raw)
		saveResult <- saveErr
	}()
	go func() {
		<-start
		deleted, deleteErr := service.DeleteImage(ctx, values[0].ID)
		if deleteErr == nil && !deleted {
			deleteErr = errors.New("concurrent delete was skipped")
		}
		deleteResult <- deleteErr
	}()
	close(start)
	if err := <-saveResult; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteResult; err != nil {
		t.Fatal(err)
	}
	total, err := assets.TotalMediaAssetBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(len(raw)*2) || service.totalBytes.Load() != total {
		t.Fatalf("repository total = %d, service total = %d", total, service.totalBytes.Load())
	}
}

func TestCleanupRestoresMetadataWhenObjectDeleteFails(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-cleanup-restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	baseObjects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	objects := &failingObjectStorage{MediaObjectStorage: baseObjects, failures: 1}
	assets := relational.NewMediaAssetRepository(database)
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	id := "img_cleanup_restore_0001"
	key, err := objects.SaveImage(ctx, id, "image/png", raw)
	if err != nil {
		t.Fatal(err)
	}
	asset := mediadomain.Asset{ID: id, Kind: "image", StorageKey: key, MIMEType: "image/png", SizeBytes: int64(len(raw)), SHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC()}
	if err := assets.CreateMediaAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	service := NewService(assets, nil, objects, nil, Config{MaxTotalBytes: int64(len(raw)), CleanupThresholdPercent: 50, CleanupInterval: time.Minute})

	deleted, err := service.Cleanup(ctx)
	if err == nil || deleted != 0 {
		t.Fatalf("failed cleanup = %d, %v", deleted, err)
	}
	if _, err := assets.GetMediaAsset(ctx, asset.ID); err != nil {
		t.Fatalf("cleanup metadata was not restored: %v", err)
	}
	body, err := objects.Open(ctx, asset.StorageKey)
	if err != nil {
		t.Fatalf("cleanup object was not retained: %v", err)
	}
	_ = body.Close()
	if service.totalBytes.Load() != asset.SizeBytes {
		t.Fatalf("cleanup total bytes after failure = %d", service.totalBytes.Load())
	}

	deleted, err = service.Cleanup(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup retry = %d, %v", deleted, err)
	}
	if _, err := assets.GetMediaAsset(ctx, asset.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cleanup metadata remained after retry: %v", err)
	}
	if _, err := objects.Open(ctx, asset.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup object remained after retry: %v", err)
	}
}

func TestCleanupRefreshesTotalBytesAfterExternalMutation(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-cleanup-refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	assets := relational.NewMediaAssetRepository(database)
	first := testDeleteAsset("img_cleanup_refresh_0001", "images/re/first.png")
	second := testDeleteAsset("img_cleanup_refresh_0002", "images/re/second.png")
	first.SizeBytes, second.SizeBytes = 128, 256
	if err := assets.CreateMediaAsset(ctx, first); err != nil {
		t.Fatal(err)
	}
	service := NewService(assets, nil, objects, nil, Config{MaxTotalBytes: 1000, CleanupThresholdPercent: 80, CleanupInterval: time.Minute})
	if deleted, err := service.Cleanup(ctx); err != nil || deleted != 0 {
		t.Fatalf("initial cleanup = %d, %v", deleted, err)
	}
	if service.totalBytes.Load() != first.SizeBytes {
		t.Fatalf("initial total bytes = %d", service.totalBytes.Load())
	}
	if err := assets.CreateMediaAsset(ctx, second); err != nil {
		t.Fatal(err)
	}
	if deleted, err := service.Cleanup(ctx); err != nil || deleted != 0 {
		t.Fatalf("refreshed cleanup = %d, %v", deleted, err)
	}
	if service.totalBytes.Load() != first.SizeBytes+second.SizeBytes {
		t.Fatalf("refreshed total bytes = %d", service.totalBytes.Load())
	}
}

type nonImageAssetRepository struct {
	repository.MediaAssetRepository
	asset       mediadomain.Asset
	deleteCalls int
}

type activeReferencedAssetRepository struct {
	repository.MediaAssetRepository
	asset       mediadomain.Asset
	deleteCalls int
}

type orderedDeleteAssetRepository struct {
	repository.MediaAssetRepository
	asset mediadomain.Asset
	order *[]string
}

type orderedDeleteObjectStorage struct {
	order *[]string
}

type failingObjectStorage struct {
	repository.MediaObjectStorage
	failures int
}

func (s *failingObjectStorage) Delete(ctx context.Context, storageKey string) error {
	if s.failures > 0 {
		s.failures--
		return errors.New("object deletion failed")
	}
	return s.MediaObjectStorage.Delete(ctx, storageKey)
}

func (*orderedDeleteObjectStorage) SaveImage(context.Context, string, string, []byte) (string, error) {
	return "", nil
}

func (*orderedDeleteObjectStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *orderedDeleteObjectStorage) Delete(context.Context, string) error {
	*s.order = append(*s.order, "object")
	return nil
}

func (r *orderedDeleteAssetRepository) GetMediaAsset(context.Context, string) (mediadomain.Asset, error) {
	return r.asset, nil
}

func (r *orderedDeleteAssetRepository) TotalMediaAssetBytes(context.Context) (int64, error) {
	return r.asset.SizeBytes, nil
}

func (r *orderedDeleteAssetRepository) DeleteMediaAssetIfUnused(context.Context, string) (bool, error) {
	*r.order = append(*r.order, "metadata")
	return true, nil
}

func (r *activeReferencedAssetRepository) GetMediaAsset(context.Context, string) (mediadomain.Asset, error) {
	return r.asset, nil
}

func (r *activeReferencedAssetRepository) TotalMediaAssetBytes(context.Context) (int64, error) {
	return r.asset.SizeBytes, nil
}

func (r *activeReferencedAssetRepository) DeleteMediaAssetIfUnused(context.Context, string) (bool, error) {
	return false, nil
}

func (r *nonImageAssetRepository) GetMediaAsset(context.Context, string) (mediadomain.Asset, error) {
	return r.asset, nil
}

func (r *nonImageAssetRepository) DeleteMediaAssetIfUnused(context.Context, string) (bool, error) {
	r.deleteCalls++
	return true, nil
}

func testDeleteAsset(id, storageKey string) mediadomain.Asset {
	return mediadomain.Asset{ID: id, Kind: "image", StorageKey: storageKey, MIMEType: "image/png", SizeBytes: 128, SHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC()}
}

func TestCleanupDeletesOldestAssetsAtThreshold(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-cleanup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	assets := relational.NewMediaAssetRepository(database)
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	now := time.Now().UTC()
	ids := []string{"img_cleanup_0000000000000001", "img_cleanup_0000000000000002", "img_cleanup_0000000000000003", "img_cleanup_0000000000000004"}
	for index, id := range ids {
		key, err := objects.SaveImage(ctx, id, "image/png", raw)
		if err != nil {
			t.Fatal(err)
		}
		createdAt := now.Add(time.Duration(index-4) * time.Hour)
		if index == len(ids)-1 {
			createdAt = now
		}
		if err := assets.CreateMediaAsset(ctx, mediadomain.Asset{
			ID: id, Kind: "image", StorageKey: key, MIMEType: "image/png", SizeBytes: int64(len(raw)),
			SHA256: strings.Repeat("a", 64), CreatedAt: createdAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(assets, relational.NewMediaJobRepository(database), objects, nil, Config{
		PublicBaseURL: "https://api.example", MaxImageBytes: 32 << 20,
		MaxTotalBytes: int64(len(raw) * 2), CleanupThresholdPercent: 50,
		CleanupInterval: 10 * time.Minute,
	})
	deleted, err := service.Cleanup(ctx)
	if err != nil || deleted != 3 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	total, err := assets.TotalMediaAssetBytes(ctx)
	if err != nil || total != int64(len(raw)) {
		t.Fatalf("remaining bytes=%d err=%v", total, err)
	}
	if _, _, err := service.OpenImage(ctx, ids[0]); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("oldest asset still exists: %v", err)
	}
	if _, body, err := service.OpenImage(ctx, ids[3]); err != nil {
		t.Fatalf("recent asset was deleted: %v", err)
	} else {
		_ = body.Close()
	}
}

func TestCleanupToleratesMissingObjectAfterAtomicMetadataDelete(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media-missing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	assets := relational.NewMediaAssetRepository(database)
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	id := "img_missing_0000000000000001"
	key, err := objects.SaveImage(ctx, id, "image/png", raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.CreateMediaAsset(ctx, mediadomain.Asset{ID: id, Kind: "image", StorageKey: key, MIMEType: "image/png", SizeBytes: int64(len(raw)), SHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := objects.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	service := NewService(assets, relational.NewMediaJobRepository(database), objects, nil, Config{PublicBaseURL: "https://api.example", MaxImageBytes: 32 << 20, MaxTotalBytes: int64(len(raw)), CleanupThresholdPercent: 50, CleanupInterval: 10 * time.Minute})
	deleted, err := service.Cleanup(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup deleted = %d, error = %v", deleted, err)
	}
	if _, err := assets.GetMediaAsset(ctx, id); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("metadata still exists: %v", err)
	}
}

func TestPublicImageURLUsesHotReloadedBase(t *testing.T) {
	service := NewService(nil, nil, nil, nil, Config{PublicBaseURL: "https://config.example/base/"})
	if got := service.PublicImageURL("img_demo"); got != "https://config.example/base/v1/media/images/img_demo" {
		t.Fatalf("configured URL = %q", got)
	}
	updated := service.runtimeConfig()
	updated.PublicBaseURL = "https://runtime.example/api/"
	service.UpdateConfig(updated)
	if got := service.PublicImageURL("img_demo"); got != "https://runtime.example/api/v1/media/images/img_demo" {
		t.Fatalf("hot-reloaded URL = %q", got)
	}
}
