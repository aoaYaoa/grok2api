package relational

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

func TestMediaJobRepositoryListMediaJobsPaginatesAndFilters(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)

	accountValue, _, err := NewAccountRepository(database).UpsertByIdentity(ctx, accountdomain.Credential{
		Provider:             accountdomain.ProviderWeb,
		AuthType:             accountdomain.AuthTypeSSO,
		WebTier:              accountdomain.WebTierBasic,
		Name:                 "media-list-account",
		SourceKey:            "media-list-account",
		EncryptedAccessToken: testEncryptedToken,
		AuthStatus:           accountdomain.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := clientKeyModel{Name: "media-list-key", Prefix: "media-list-key", SecretHash: testSecretHash, EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	if err := database.db.WithContext(ctx).Create(&key).Error; err != nil {
		t.Fatal(err)
	}

	jobRepo := NewMediaJobRepository(database)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	jobs := []mediadomain.Job{
		testMediaJob("media_job_completed_old", accountValue.ID, key.ID, mediadomain.StatusCompleted, now.Add(-4*time.Hour)),
		testMediaJob("media_job_queued_mid", accountValue.ID, key.ID, mediadomain.StatusQueued, now.Add(-3*time.Hour)),
		testMediaJob("media_job_failed_newer", accountValue.ID, key.ID, mediadomain.StatusFailed, now.Add(-2*time.Hour)),
		testMediaJob("media_job_completed_new", accountValue.ID, key.ID, mediadomain.StatusCompleted, now.Add(-time.Hour)),
	}
	jobs[0].Prompt = "A quiet harbor"
	jobs[1].Prompt = "Northern lights"
	jobs[2].Prompt = "Desert sunrise"
	jobs[3].Prompt = "City skyline"
	for _, job := range jobs {
		if err := jobRepo.CreateMediaJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}

	firstPage, total, err := jobRepo.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page: repository.PageQuery{Offset: 0, Limit: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("total = %d", total)
	}
	assertMediaJobIDs(t, firstPage, "media_job_completed_new", "media_job_failed_newer")

	secondPage, total, err := jobRepo.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page: repository.PageQuery{Offset: 2, Limit: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("second page total = %d", total)
	}
	assertMediaJobIDs(t, secondPage, "media_job_queued_mid", "media_job_completed_old")

	completed, total, err := jobRepo.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page:   repository.PageQuery{Offset: 0, Limit: 10},
		Filter: repository.MediaJobListFilter{Status: string(mediadomain.StatusCompleted)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("completed total = %d", total)
	}
	assertMediaJobIDs(t, completed, "media_job_completed_new", "media_job_completed_old")
	if completed[0].ClientKeyID != key.ID {
		t.Fatalf("completed job client key = %d, want %d", completed[0].ClientKeyID, key.ID)
	}

	searched, total, err := jobRepo.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page: repository.PageQuery{Offset: 0, Limit: 1, Search: "northern"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("searched total = %d", total)
	}
	assertMediaJobIDs(t, searched, "media_job_queued_mid")

	sorted, total, err := jobRepo.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page: repository.PageQuery{
			Offset: 0,
			Limit:  4,
			Sort:   repository.SortQuery{Field: "prompt", Direction: repository.SortAscending},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("sorted total = %d", total)
	}
	assertMediaJobIDs(t, sorted, "media_job_completed_old", "media_job_completed_new", "media_job_failed_newer", "media_job_queued_mid")

	stats, err := jobRepo.SummarizeMediaJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalJobs != 4 || stats.Completed != 2 || stats.Failed != 1 || stats.InProgress != 0 || stats.Queued != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestMediaJobRepositoryDeletesOnlyOwnedTerminalJobs(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)

	accountValue, _, err := NewAccountRepository(database).UpsertByIdentity(ctx, accountdomain.Credential{
		Provider:             accountdomain.ProviderWeb,
		AuthType:             accountdomain.AuthTypeSSO,
		WebTier:              accountdomain.WebTierBasic,
		Name:                 "media-delete-account",
		SourceKey:            "media-delete-account",
		EncryptedAccessToken: testEncryptedToken,
		AuthStatus:           accountdomain.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := clientKeyModel{Name: "media-delete-key", Prefix: "media-delete-key", SecretHash: testSecretHash, EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	foreignKey := clientKeyModel{Name: "media-delete-foreign-key", Prefix: "media-delete-foreign-key", SecretHash: strings.Repeat("b", 64), EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	if err := database.db.WithContext(ctx).Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.db.WithContext(ctx).Create(&foreignKey).Error; err != nil {
		t.Fatal(err)
	}

	jobRepo := NewMediaJobRepository(database)
	now := time.Now().UTC()
	jobs := []mediadomain.Job{
		testMediaJob("media_delete_completed_recorded", accountValue.ID, key.ID, mediadomain.StatusCompleted, now),
		testMediaJob("media_delete_failed_recorded", accountValue.ID, key.ID, mediadomain.StatusFailed, now),
		testMediaJob("media_delete_completed_unrecorded", accountValue.ID, key.ID, mediadomain.StatusCompleted, now),
		testMediaJob("media_delete_queued", accountValue.ID, key.ID, mediadomain.StatusQueued, now),
		testMediaJob("media_delete_running", accountValue.ID, key.ID, mediadomain.StatusInProgress, now),
		testMediaJob("media_delete_foreign", accountValue.ID, foreignKey.ID, mediadomain.StatusCompleted, now),
	}
	jobs[0].UsageRecordedAt = &now
	jobs[1].UsageRecordedAt = &now
	jobs[5].UsageRecordedAt = &now
	for _, job := range jobs {
		if err := jobRepo.CreateMediaJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []string{"media_delete_completed_recorded", "media_delete_failed_recorded"} {
		deleted, err := jobRepo.DeleteOwnedTerminalMediaJob(ctx, id, key.ID)
		if err != nil || !deleted {
			t.Fatalf("delete %q = %v, %v", id, deleted, err)
		}
	}
	for _, id := range []string{"media_delete_completed_unrecorded", "media_delete_queued", "media_delete_running", "media_delete_foreign", "media_delete_completed_recorded"} {
		deleted, err := jobRepo.DeleteOwnedTerminalMediaJob(ctx, id, key.ID)
		if err != nil || deleted {
			t.Fatalf("delete %q = %v, %v", id, deleted, err)
		}
	}
}

func TestMediaAssetRepositoryOldestExcludesAssetsUsedByActiveVideoJobs(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accountValue, _, err := NewAccountRepository(database).UpsertByIdentity(ctx, accountdomain.Credential{
		Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, WebTier: accountdomain.WebTierBasic,
		Name: "media-reference-account", SourceKey: "media-reference-account", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := clientKeyModel{Name: "media-reference-key", Prefix: "media-reference-key", SecretHash: testSecretHash, EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	if err := database.db.WithContext(ctx).Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	assetRepo := NewMediaAssetRepository(database)
	for index, id := range []string{"img_active_reference_0001", "img_unused_reference_0002"} {
		if err := assetRepo.CreateMediaAsset(ctx, mediadomain.Asset{ID: id, Kind: "image", StorageKey: "images/" + id + ".png", MIMEType: "image/png", SizeBytes: 100, SHA256: strings.Repeat("a", 64), CreatedAt: now.Add(time.Duration(index) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	job := testMediaJob("media_job_active_reference", accountValue.ID, key.ID, mediadomain.StatusQueued, now)
	job.InputJSON = `{"image_urls":["grok2api-media://image/img_active_reference_0001"]}`
	if err := NewMediaJobRepository(database).CreateMediaJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	values, err := assetRepo.ListOldestMediaAssets(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].ID != "img_unused_reference_0002" {
		t.Fatalf("cleanup candidates = %#v", values)
	}
	deleted, err := assetRepo.DeleteMediaAssetIfUnused(ctx, "img_active_reference_0001")
	if err != nil || deleted {
		t.Fatalf("delete active asset = %v, %v", deleted, err)
	}
	if _, err := assetRepo.GetMediaAsset(ctx, "img_active_reference_0001"); err != nil {
		t.Fatalf("active asset was deleted: %v", err)
	}
	deleted, err = assetRepo.DeleteMediaAssetIfUnused(ctx, "img_unused_reference_0002")
	if err != nil || !deleted {
		t.Fatalf("delete unused asset = %v, %v", deleted, err)
	}
}

func TestMediaJobRepositoryValidatesInternalAssetReferences(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accountValue, _, err := NewAccountRepository(database).UpsertByIdentity(ctx, accountdomain.Credential{
		Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, WebTier: accountdomain.WebTierBasic,
		Name: "media-contract-account", SourceKey: "media-contract-account", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := clientKeyModel{Name: "media-contract-key", Prefix: "media-contract-key", SecretHash: testSecretHash, EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	if err := database.db.WithContext(ctx).Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	assetRepo := NewMediaAssetRepository(database)
	jobRepo := NewMediaJobRepository(database)
	asset := testMediaAsset("img_contract_asset_0001", "media/contract-asset.png", time.Now().UTC())
	if err := assetRepo.CreateMediaAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}

	for _, value := range []struct {
		id        string
		inputJSON string
		wantErr   error
	}{
		{id: "media_job_scheme_reference", inputJSON: `{"image_urls":["grok2api-media://image/img_contract_asset_0001"]}`},
		{id: "media_job_path_reference", inputJSON: `{"image_urls":["/v1/media/images/img_contract_asset_0001"]}`},
		{id: "media_job_configured_path_reference", inputJSON: `{"image_urls":["/api/v2/v1/media/images/img_contract_asset_0001"]}`},
		{id: "media_job_external_reference", inputJSON: `{"image_urls":["https://example.com/source.png"]}`},
		{id: "media_job_external_matching_route", inputJSON: `{"image_urls":["https://cdn.example/api/v1/media/images/img_contract_missing_external"]}`},
		{id: "media_job_missing_scheme", inputJSON: `{"image_urls":["grok2api-media://image/img_contract_missing_0002"]}`, wantErr: repository.ErrNotFound},
		{id: "media_job_missing_path", inputJSON: `{"image_urls":["/v1/media/images/img_contract_missing_0003"]}`, wantErr: repository.ErrNotFound},
		{id: "media_job_missing_configured_path", inputJSON: `{"image_urls":["/api/v2/v1/media/images/img_contract_missing_0004"]}`, wantErr: repository.ErrNotFound},
		{id: "media_job_nested_internal_path", inputJSON: `{"image_urls":["/api/v1/media/images/img_contract_asset_0001/extra"]}`, wantErr: repository.ErrNotFound},
	} {
		job := testMediaJob(value.id, accountValue.ID, key.ID, mediadomain.StatusQueued, time.Now().UTC())
		job.InputJSON = value.inputJSON
		err := jobRepo.CreateMediaJob(ctx, job)
		if !errors.Is(err, value.wantErr) {
			t.Fatalf("create %q error = %v, want %v", value.id, err, value.wantErr)
		}
	}
}

func TestMediaRepositoriesConcurrentCreateAndDeletePreserveAssetReference(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accountValue, _, err := NewAccountRepository(database).UpsertByIdentity(ctx, accountdomain.Credential{
		Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, WebTier: accountdomain.WebTierBasic,
		Name: "media-interleave-account", SourceKey: "media-interleave-account", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := clientKeyModel{Name: "media-interleave-key", Prefix: "media-interleave-key", SecretHash: testSecretHash, EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4}
	if err := database.db.WithContext(ctx).Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	assetRepo := NewMediaAssetRepository(database)
	jobRepo := NewMediaJobRepository(database)
	asset := testMediaAsset("img_interleave_asset_0001", "media/interleave-asset.png", time.Now().UTC())
	if err := assetRepo.CreateMediaAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	job := testMediaJob("media_job_interleave", accountValue.ID, key.ID, mediadomain.StatusQueued, time.Now().UTC())
	job.InputJSON = `{"image_urls":["grok2api-media://image/img_interleave_asset_0001"]}`

	start := make(chan struct{})
	deleteResult := make(chan bool, 1)
	deleteError := make(chan error, 1)
	createError := make(chan error, 1)
	go func() {
		<-start
		deleted, deleteErr := assetRepo.DeleteMediaAssetIfUnused(ctx, asset.ID)
		deleteResult <- deleted
		deleteError <- deleteErr
	}()
	go func() {
		<-start
		createError <- jobRepo.CreateMediaJob(ctx, job)
	}()
	close(start)
	deleted, deleteErr, createErr := <-deleteResult, <-deleteError, <-createError
	if deleteErr != nil {
		t.Fatal(deleteErr)
	}
	if createErr == nil {
		if deleted {
			t.Fatal("job was created while its referenced asset was deleted")
		}
		if _, err := assetRepo.GetMediaAsset(ctx, asset.ID); err != nil {
			t.Fatalf("created job lost its asset: %v", err)
		}
		return
	}
	if !errors.Is(createErr, repository.ErrNotFound) || !deleted {
		t.Fatalf("create error = %v, deleted = %v", createErr, deleted)
	}
	if _, err := jobRepo.GetMediaJob(ctx, job.ID, key.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("failed job creation committed: %v", err)
	}
}

func TestMediaAssetRepositoryListMediaAssetsPaginatesAndCounts(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	assetRepo := NewMediaAssetRepository(database)

	stats, err := assetRepo.SummarizeMediaAssets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalImages != 0 || stats.TotalBytes != 0 {
		t.Fatalf("initial stats = %#v", stats)
	}

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	assets := []mediadomain.Asset{
		testMediaAsset("media_asset_0001", "media/asset-0001.png", now.Add(-3*time.Hour)),
		testMediaAsset("media_asset_0002", "media/asset-0002.png", now.Add(-2*time.Hour)),
		testMediaAsset("media_asset_0003", "media/asset-0003.png", now.Add(-time.Hour)),
	}
	for _, asset := range assets {
		if err := assetRepo.CreateMediaAsset(ctx, asset); err != nil {
			t.Fatal(err)
		}
	}

	firstPage, total, err := assetRepo.ListMediaAssets(ctx, repository.MediaAssetListQuery{
		Page: repository.PageQuery{Offset: 0, Limit: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total = %d", total)
	}
	assertMediaAssetIDs(t, firstPage, "media_asset_0003", "media_asset_0002")

	secondPage, total, err := assetRepo.ListMediaAssets(ctx, repository.MediaAssetListQuery{
		Page: repository.PageQuery{Offset: 2, Limit: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("second page total = %d", total)
	}
	assertMediaAssetIDs(t, secondPage, "media_asset_0001")

	searched, total, err := assetRepo.ListMediaAssets(ctx, repository.MediaAssetListQuery{
		Page: repository.PageQuery{Offset: 0, Limit: 1, Search: "0001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("searched total = %d", total)
	}
	assertMediaAssetIDs(t, searched, "media_asset_0001")

	stats, err = assetRepo.SummarizeMediaAssets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalImages != 3 || stats.TotalBytes != 3*1024 {
		t.Fatalf("stats = %#v", stats)
	}
}

func testMediaJob(id string, accountID, clientKeyID uint64, status mediadomain.Status, createdAt time.Time) mediadomain.Job {
	job := mediadomain.Job{
		ID:            id,
		RequestID:     "request-" + id,
		ClientKeyID:   clientKeyID,
		ClientKeyName: "media-list-key",
		AccountID:     accountID,
		AccountName:   "media-list-account",
		Provider:      "grok_web",
		Model:         "grok-imagine-video",
		ModelRouteID:  1,
		UpstreamModel: "grok-imagine-video-upstream",
		Prompt:        "test prompt",
		Seconds:       8,
		Size:          "16:9",
		Quality:       "720p",
		Status:        status,
		InputJSON:     `{}`,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	if status == mediadomain.StatusCompleted || status == mediadomain.StatusFailed {
		job.Progress = 100
		completedAt := createdAt.Add(time.Minute)
		job.CompletedAt = &completedAt
	}
	return job
}

func testMediaAsset(id, storageKey string, createdAt time.Time) mediadomain.Asset {
	return mediadomain.Asset{
		ID:         id,
		Kind:       "image",
		StorageKey: storageKey,
		MIMEType:   "image/png",
		SizeBytes:  1024,
		SHA256:     strings.Repeat("a", 64),
		CreatedAt:  createdAt,
	}
}

func assertMediaJobIDs(t *testing.T, values []mediadomain.Job, expected ...string) {
	t.Helper()
	if len(values) != len(expected) {
		t.Fatalf("len(values) = %d, expected %d: %#v", len(values), len(expected), values)
	}
	for index, id := range expected {
		if values[index].ID != id {
			t.Fatalf("values[%d].ID = %q, expected %q; values = %#v", index, values[index].ID, id, values)
		}
	}
}

func assertMediaAssetIDs(t *testing.T, values []mediadomain.Asset, expected ...string) {
	t.Helper()
	if len(values) != len(expected) {
		t.Fatalf("len(values) = %d, expected %d: %#v", len(values), len(expected), values)
	}
	for index, id := range expected {
		if values[index].ID != id {
			t.Fatalf("values[%d].ID = %q, expected %q; values = %#v", index, values[index].ID, id, values)
		}
	}
}
