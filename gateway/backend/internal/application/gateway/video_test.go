package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/domain/model"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

func TestVideoForbiddenRetryPlanUsesBoundedBackoff(t *testing.T) {
	for attempt, expected := range []time.Duration{30 * time.Second, 2 * time.Minute, 5 * time.Minute} {
		delay, ok := videoRetryPlan(http.StatusForbidden, attempt, true)
		if !ok || delay != expected {
			t.Fatalf("attempt %d: delay=%s retry=%v", attempt, delay, ok)
		}
	}
	if _, ok := videoRetryPlan(http.StatusForbidden, 3, true); ok {
		t.Fatal("fourth forbidden response must be terminal")
	}
	if _, ok := videoRetryPlan(http.StatusForbidden, 0, false); ok {
		t.Fatal("providers without egress retries must fail immediately")
	}
}

func TestVideoIncompleteStreamRetryPlanUsesBoundedBackoff(t *testing.T) {
	for attempt, expected := range []time.Duration{15 * time.Second, time.Minute, 3 * time.Minute} {
		delay, ok := videoRetryPlan(http.StatusBadGateway, attempt, true)
		if !ok || delay != expected {
			t.Fatalf("attempt %d: delay=%s retry=%v", attempt, delay, ok)
		}
	}
	if _, ok := videoRetryPlan(http.StatusBadGateway, 3, true); ok {
		t.Fatal("fourth incomplete stream response must be terminal")
	}
}

func TestPublicVideoFailureMessageHidesUpstreamHTML(t *testing.T) {
	err := errors.New("上传图片失败，上游返回 403: <!DOCTYPE html><html><title>Just a moment...</title></html>")
	message := publicVideoFailureMessage(err)
	if message != "上游安全验证暂时未通过，请稍后重试" {
		t.Fatalf("message = %q", message)
	}
	if strings.Contains(strings.ToLower(message), "doctype") || strings.Contains(strings.ToLower(message), "html") {
		t.Fatalf("raw HTML leaked: %q", message)
	}
}

func TestDeleteVideoTrimsIDAndDelegatesOwnedTerminalDeletion(t *testing.T) {
	repository := &videoDeleteRepository{}
	service := &Service{mediaJobs: repository}
	key := clientkey.Key{ID: 77}

	deleted, err := service.DeleteVideo(context.Background(), "  video_delete_123  ", key)
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	if repository.id != "video_delete_123" || repository.clientKeyID != key.ID {
		t.Fatalf("delegation = id %q, key %d", repository.id, repository.clientKeyID)
	}
}

func TestRecoverVideoJobsRetriesUsageWithoutRegeneratingVideo(t *testing.T) {
	completedAt := time.Now().UTC()
	repository := &videoUsageRepository{job: media.Job{
		ID: "video_usage_recovery", RequestID: "request-usage-recovery",
		ClientKeyID: 1, ClientKeyName: "client", AccountID: 2, AccountName: "account",
		Provider: "grok_web", Model: "grok-imagine-video", ModelRouteID: 3, UpstreamModel: "video",
		Seconds: 8, Quality: "720p", Status: media.StatusCompleted, InputImageCount: 2, CreatedAt: completedAt.Add(-time.Minute), CompletedAt: &completedAt,
	}}
	recorder := &durableVideoAuditRecorder{failures: 1}
	service := &Service{mediaJobs: repository, audits: recorder}
	if err := service.RecoverVideoJobs(context.Background()); err == nil {
		t.Fatal("first durable audit failure was ignored")
	}
	if repository.job.UsageRecordedAt != nil {
		t.Fatal("usage was marked before durable audit commit")
	}
	if err := service.RecoverVideoJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.job.UsageRecordedAt == nil || recorder.calls != 2 {
		t.Fatalf("recordedAt = %v, audit calls = %d", repository.job.UsageRecordedAt, recorder.calls)
	}
	if recorder.last.EventID != "video_usage_video_usage_recovery" || recorder.last.EstimatedCostInUSDTicks <= 0 || recorder.last.MediaInputImages != 2 {
		t.Fatalf("audit = %#v", recorder.last)
	}
}

func TestEncodeVideoInputEnforcesPersistedLimit(t *testing.T) {
	overhead := len(`{"image_urls":[""]}`)
	atLimit := strings.Repeat("A", media.MaxInputJSONBytes-overhead)
	encoded, err := encodeVideoInput([]string{atLimit})
	if err != nil || len(encoded) != media.MaxInputJSONBytes {
		t.Fatalf("encoded len=%d err=%v", len(encoded), err)
	}
	if _, err := encodeVideoInput([]string{atLimit + "A"}); !errors.Is(err, ErrVideoInputTooLarge) {
		t.Fatalf("oversized input error = %v", err)
	}
}

func TestRecoverVideoJobsRecordsFailedAuditWithEgress(t *testing.T) {
	completedAt := time.Now().UTC()
	nodeID := uint64(42)
	repository := &videoUsageRepository{job: media.Job{
		ID: "video_failed_recovery", RequestID: "request-failed-recovery",
		ClientKeyID: 1, ClientKeyName: "client", AccountID: 2, AccountName: "account",
		Provider: "grok_web", Model: "grok-imagine-video", ModelRouteID: 3, UpstreamModel: "video",
		Seconds: 8, Quality: "720p", Status: media.StatusFailed, ErrorCode: "generation_failed", ErrorMessage: "upstream disconnected",
		EgressNodeID: &nodeID, EgressNodeName: "warp", EgressScope: "grok_web", EgressMode: "proxy",
		InputJSON: `{}`, CreatedAt: completedAt.Add(-time.Minute), CompletedAt: &completedAt,
	}}
	recorder := &durableVideoAuditRecorder{}
	service := &Service{mediaJobs: repository, audits: recorder}
	if err := service.RecoverVideoJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.job.UsageRecordedAt == nil || recorder.calls != 1 {
		t.Fatalf("recordedAt = %v, audit calls = %d", repository.job.UsageRecordedAt, recorder.calls)
	}
	if recorder.last.StatusCode != 502 || recorder.last.ErrorCode != "generation_failed" || recorder.last.EgressNodeID == nil || *recorder.last.EgressNodeID != nodeID || recorder.last.EgressNodeName != "warp" || recorder.last.EgressMode != audit.EgressModeProxy {
		t.Fatalf("audit = %#v", recorder.last)
	}
	if recorder.last.EstimatedCostInUSDTicks != 0 || recorder.last.MediaOutputSeconds != 0 {
		t.Fatalf("failed job was billed: %#v", recorder.last)
	}
}

func TestRecoverVideoJobsRecordsDetachedAccountSnapshot(t *testing.T) {
	completedAt := time.Now().UTC()
	repository := &videoUsageRepository{job: media.Job{
		ID: "video_detached_account", RequestID: "request-detached-account",
		ClientKeyID: 1, ClientKeyName: "client", AccountName: "deleted account",
		Provider: "grok_web", Model: "grok-imagine-video", ModelRouteID: 3, UpstreamModel: "video",
		Seconds: 8, Quality: "720p", Status: media.StatusFailed, ErrorCode: "generation_failed",
		InputJSON: `{}`, CreatedAt: completedAt.Add(-time.Minute), CompletedAt: &completedAt,
	}}
	recorder := &durableVideoAuditRecorder{}
	service := &Service{mediaJobs: repository, audits: recorder}
	if err := service.RecoverVideoJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.last.AccountID != nil || recorder.last.AccountName != "deleted account" {
		t.Fatalf("detached account audit = %#v", recorder.last)
	}
}

func TestLogVideoGenerationFailurePreservesUpstreamDiagnostic(t *testing.T) {
	var output bytes.Buffer
	service := &Service{logger: slog.New(slog.NewTextHandler(&output, nil))}
	nodeID := uint64(7)
	service.logVideoGenerationFailure(media.Job{
		ID: "video_failure", RequestID: "request-failure", UpstreamModel: "grok-imagine-video",
		EgressNodeID: &nodeID, EgressNodeName: "proxy-1", EgressScope: "grok_web", EgressMode: "proxy",
	}, account.Credential{ID: 42, Provider: account.ProviderWeb}, videoStatusError{
		status:  http.StatusForbidden,
		message: "Grok Web 媒体上游返回 403: upload denied access_token=secret https://assets.grok.com/video?token=secret",
	})
	logLine := output.String()
	for _, expected := range []string{
		"msg=video_generation_failed", "job_id=video_failure", "request_id=request-failure",
		"account_id=42", "provider=grok_web", "upstream_status=403", "upload denied",
		"egress_node_id=7", "egress_node_name=proxy-1",
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("log missing %q: %s", expected, logLine)
		}
	}
	for _, secret := range []string{"access_token=secret", "token=secret"} {
		if strings.Contains(logLine, secret) {
			t.Fatalf("log exposed %q: %s", secret, logLine)
		}
	}
}

type videoStatusError struct {
	status  int
	message string
}

func (e videoStatusError) Error() string       { return e.message }
func (e videoStatusError) HTTPStatusCode() int { return e.status }

func TestShouldSwitchVideoAccountStopsForConsoleDPoPRequirement(t *testing.T) {
	if shouldSwitchVideoAccount(videoStatusError{status: http.StatusForbidden, message: "Console 媒体上游返回 403: DPoP proof required but was not verified"}) {
		t.Fatal("DPoP protocol requirement must not rotate through accounts")
	}
	if !shouldSwitchVideoAccount(videoStatusError{status: http.StatusForbidden, message: "Console 媒体上游返回 403: account denied"}) {
		t.Fatal("ordinary account-scoped forbidden response should remain switchable")
	}
}

func TestVideoQueueIsBoundedAndDeduplicated(t *testing.T) {
	service := &Service{}
	service.ConfigureMedia(&videoUsageRepository{}, 1)
	capacity := cap(service.mediaQueue)
	for index := range capacity {
		if !service.enqueueVideoJob(fmt.Sprintf("video_%d", index)) {
			t.Fatalf("enqueue %d failed before capacity", index)
		}
	}
	if !service.enqueueVideoJob("video_0") {
		t.Fatal("duplicate queued job should be treated as accepted")
	}
	if service.enqueueVideoJob("video_overflow") {
		t.Fatal("queue accepted a job beyond its capacity")
	}
}

func TestPersistRemoteVideoRetriesSameResultWithoutRegeneration(t *testing.T) {
	adapter := &videoPersistAdapter{failures: 1}
	store := &videoAssetStoreStub{}
	service := &Service{mediaAssets: store}
	credential := account.Credential{ID: 42, Provider: account.ProviderWeb}
	result, err := service.persistRemoteVideo(context.Background(), "video_job", adapter, credential, provider.VideoResult{URL: "https://assets.grok.com/video.mp4", ContentType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.generateCalls != 0 || adapter.downloadCalls != 2 || adapter.lastCredentialID != credential.ID {
		t.Fatalf("generate=%d download=%d credential=%d", adapter.generateCalls, adapter.downloadCalls, adapter.lastCredentialID)
	}
	if store.saveCalls != 1 || result.AssetID != "vid_local" || result.ContentType != "video/mp4" {
		t.Fatalf("store calls=%d result=%#v", store.saveCalls, result)
	}
}

type videoPersistAdapter struct {
	failures         int
	generateCalls    int
	downloadCalls    int
	lastCredentialID uint64
}

func (a *videoPersistAdapter) Provider() account.Provider { return account.ProviderWeb }

func (a *videoPersistAdapter) GenerateVideo(context.Context, provider.VideoRequest) (provider.VideoResult, error) {
	a.generateCalls++
	return provider.VideoResult{}, errors.New("must not regenerate")
}

func (a *videoPersistAdapter) DownloadVideo(_ context.Context, credential account.Credential, _ string) (io.ReadCloser, string, int64, error) {
	a.downloadCalls++
	a.lastCredentialID = credential.ID
	if a.downloadCalls <= a.failures {
		return nil, "", 0, errors.New("temporary download failure")
	}
	return io.NopCloser(strings.NewReader("video")), "video/mp4", 5, nil
}

type videoAssetStoreStub struct{ saveCalls int }

func (s *videoAssetStoreStub) SaveVideo(_ context.Context, jobID, contentType string, body io.Reader) (media.Asset, error) {
	s.saveCalls++
	if jobID != "video_job" {
		return media.Asset{}, fmt.Errorf("job ID = %s", jobID)
	}
	if contentType != "video/mp4" {
		return media.Asset{}, fmt.Errorf("content type = %s", contentType)
	}
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "video" {
		return media.Asset{}, fmt.Errorf("video body = %q: %w", data, err)
	}
	return media.Asset{ID: "vid_local", Kind: "video", MIMEType: "video/mp4", SizeBytes: int64(len(data))}, nil
}

func (*videoAssetStoreStub) OpenVideo(context.Context, string) (media.Asset, io.ReadCloser, error) {
	return media.Asset{}, nil, errors.New("not implemented")
}

type durableVideoAuditRecorder struct {
	failures int
	calls    int
	last     audit.Record
}

func (r *durableVideoAuditRecorder) Create(context.Context, audit.Record) error { return nil }

func (r *durableVideoAuditRecorder) CreateDurable(_ context.Context, value audit.Record) error {
	r.calls++
	r.last = value
	if r.calls <= r.failures {
		return errors.New("database unavailable")
	}
	return nil
}

type videoUsageRepository struct{ job media.Job }

type videoDeleteRepository struct {
	repository.MediaJobRepository
	id          string
	clientKeyID uint64
}

func (r *videoDeleteRepository) DeleteOwnedTerminalMediaJob(_ context.Context, id string, clientKeyID uint64) (bool, error) {
	r.id = id
	r.clientKeyID = clientKeyID
	return true, nil
}

func (r *videoUsageRepository) CreateMediaJob(context.Context, media.Job) error { return nil }

func (r *videoUsageRepository) GetMediaJob(context.Context, string, uint64) (media.Job, error) {
	return r.job, nil
}

func (r *videoUsageRepository) GetMediaJobsByIDs(context.Context, []string) ([]media.Job, error) {
	return []media.Job{r.job}, nil
}

func (r *videoUsageRepository) ListMediaJobsByClientKey(context.Context, uint64, int, int) ([]media.Job, int64, error) {
	return []media.Job{r.job}, 1, nil
}

func (r *videoUsageRepository) UpdateMediaJob(context.Context, media.Job) error { return nil }

func (r *videoUsageRepository) DeleteMediaJob(context.Context, string) error { return nil }

func (r *videoUsageRepository) DeleteOwnedTerminalMediaJob(context.Context, string, uint64) (bool, error) {
	return false, nil
}

func (r *videoUsageRepository) ListMediaJobs(context.Context, repository.MediaJobListQuery) ([]media.Job, int64, error) {
	return nil, 0, nil
}

func (r *videoUsageRepository) SummarizeMediaJobs(context.Context) (repository.MediaJobStats, error) {
	return repository.MediaJobStats{}, nil
}

func (r *videoUsageRepository) ListRecoverableMediaJobs(context.Context, int) ([]media.Job, error) {
	return nil, nil
}

func (r *videoUsageRepository) ListUnrecordedTerminalMediaJobs(context.Context, int) ([]media.Job, error) {
	if r.job.UsageRecordedAt != nil || (r.job.Status != media.StatusCompleted && r.job.Status != media.StatusFailed) {
		return nil, nil
	}
	return []media.Job{r.job}, nil
}

func (r *videoUsageRepository) TryClaimMediaJob(context.Context, string, time.Time, time.Time, string) (media.Job, bool, error) {
	return media.Job{}, false, nil
}

func (r *videoUsageRepository) MarkMediaJobUsageRecorded(_ context.Context, _ string, recordedAt time.Time) error {
	r.job.UsageRecordedAt = &recordedAt
	return nil
}

func TestVideoPosterMetadataRoundTrip(t *testing.T) {
	raw := withVideoPosterURL(`{"display_name":"Saved"}`, "/v1/files/image/poster.jpg")
	if got := videoPosterURL(media.Job{MetadataJSON: raw}); got != "/v1/files/image/poster.jpg" {
		t.Fatalf("poster URL = %q, input = %s", got, raw)
	}
	if got := withVideoPosterURL(raw, ""); got != raw {
		t.Fatalf("empty poster changed metadata: %s", got)
	}
}

func TestVideoMetadataCombinesImmutableReferencesAndMutableState(t *testing.T) {
	inputJSON, err := encodeVideoInput([]string{"data:image/png;base64,AAAA"})
	if err != nil {
		t.Fatal(err)
	}
	job := media.Job{
		InputJSON:    inputJSON,
		MetadataJSON: `{"preset":"custom","is_extension":true,"retry_count":2,"attempted_account_ids":[11,22]}`,
	}
	metadata := videoMetadataForJob(job)
	if len(metadata.ImageURLs) != 1 || metadata.ImageURLs[0] != "data:image/png;base64,AAAA" || metadata.Preset != "custom" || !metadata.IsExtension || metadata.RetryCount != 2 || len(metadata.AttemptedAccountIDs) != 2 {
		t.Fatalf("combined metadata = %#v", metadata)
	}
}

func TestVideoUnlimitedAccountAttemptsRemainUnbounded(t *testing.T) {
	service := &Service{}
	service.maxAttempts.Store(unlimitedRoutingAttempts)
	limit := service.videoAccountAttemptLimit()
	if limit != unlimitedRoutingAttempts {
		t.Fatalf("attempt limit = %d, want %d", limit, unlimitedRoutingAttempts)
	}
	metadata := videoInputMetadata{AttemptedAccountIDs: []uint64{1, 2, 3}}
	if !metadata.canTryAnotherAccount(limit) {
		t.Fatal("unlimited video routing must continue until the selector exhausts candidates")
	}
}

func TestRunVideoJobSwitchesAccountAfterCredentialRefreshFailure(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-credential-failover.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accountRepo := relational.NewAccountRepository(database)
	auditRepo := relational.NewAuditRepository(database)
	first, _, err := accountRepo.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, Name: "expired", SourceKey: "expired", EncryptedAccessToken: "expired-access", EncryptedRefreshToken: "expired-refresh",
		ExpiresAt: time.Now().Add(-time.Hour), Enabled: true, AuthStatus: account.AuthStatusActive, Priority: 200, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := accountRepo.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, Name: "ready", SourceKey: "ready", EncryptedAccessToken: "ready-access", EncryptedRefreshToken: "ready-refresh",
		ExpiresAt: time.Now().Add(time.Hour), Enabled: true, AuthStatus: account.AuthStatusActive, Priority: 100, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &videoCredentialFailoverAdapter{refreshFailureID: first.ID}
	registry := provider.NewRegistry(adapter)
	sticky := memory.NewStickyStore()
	accountService := accountapp.NewService(accountRepo, auditRepo, memory.NewDeviceSessionStore(), sticky, registry, testCipher(t), nil)
	selector := NewSelector(accountRepo, memory.NewConcurrencyLimiter(), sticky, registry, time.Hour, time.Second, time.Minute)
	keyService := clientkeyapp.NewService(relational.NewClientKeyRepository(database), nil, nil, 60, 4, testCipher(t))
	createdKey, err := keyService.Create(ctx, clientkeyapp.CreateInput{Name: "client", Enabled: true, ProviderScope: clientkey.ProviderScopeAll, TierScope: clientkey.TierScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &videoRunRepository{}
	service := &Service{
		accounts: accountService, providers: registry, selector: selector, mediaJobs: jobs,
		audits: &durableVideoAuditRecorder{}, clientKeys: keyService, logger: slog.Default(),
	}
	service.maxAttempts.Store(2)
	job := media.Job{
		ID: "video-credential-failover", RequestID: "request-credential-failover", ClientKeyID: createdKey.Key.ID, ClientKeyName: createdKey.Key.Name,
		AccountID: first.ID, AccountName: first.Name, Provider: string(account.ProviderBuild), Model: "video-test", ModelRouteID: 1, UpstreamModel: "video-test",
		Seconds: 6, Quality: "720p", Status: media.StatusInProgress, Progress: 1, CreatedAt: time.Now().UTC(), MetadataJSON: encodeVideoJobMetadata(videoInputMetadata{}),
	}
	service.runVideoJob(ctx, job, model.Route{ID: 1, Provider: account.ProviderBuild, PublicID: "video-test", UpstreamModel: "video-test"})
	if jobs.job.Status != media.StatusCompleted || jobs.job.AccountID != second.ID {
		t.Fatalf("job = %#v, want completed on account %d", jobs.job, second.ID)
	}
	if len(adapter.generateAccountIDs) != 1 || adapter.generateAccountIDs[0] != second.ID {
		t.Fatalf("generated accounts = %#v, want [%d]", adapter.generateAccountIDs, second.ID)
	}
}

func TestRunVideoJobFailsClosedWhenClientKeyScopeNoLongerAllowsProvider(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-client-key-scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accountRepo := relational.NewAccountRepository(database)
	auditRepo := relational.NewAuditRepository(database)
	credential, _, err := accountRepo.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, Name: "build-account", SourceKey: "build-account", EncryptedAccessToken: "access",
		ExpiresAt: time.Now().Add(time.Hour), Enabled: true, AuthStatus: account.AuthStatusActive, Priority: 100, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	keyService := clientkeyapp.NewService(relational.NewClientKeyRepository(database), nil, nil, 60, 4, testCipher(t))
	created, err := keyService.Create(ctx, clientkeyapp.CreateInput{
		Name: "web-only", Enabled: true, ProviderScope: clientkey.ProviderScopeWeb, TierScope: clientkey.TierScopeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &videoCredentialFailoverAdapter{}
	registry := provider.NewRegistry(adapter)
	selector := NewSelector(accountRepo, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), registry, time.Hour, time.Second, time.Minute)
	jobs := &videoRunRepository{}
	service := &Service{
		accounts:  accountapp.NewService(accountRepo, auditRepo, memory.NewDeviceSessionStore(), memory.NewStickyStore(), registry, testCipher(t), nil),
		providers: registry, selector: selector, mediaJobs: jobs, audits: &durableVideoAuditRecorder{}, clientKeys: keyService, logger: slog.Default(),
	}
	job := media.Job{
		ID: "video-client-key-scope", RequestID: "request-client-key-scope", ClientKeyID: created.Key.ID, ClientKeyName: created.Key.Name,
		AccountID: credential.ID, AccountName: credential.Name, Provider: string(account.ProviderBuild), Model: "video-test", ModelRouteID: 1, UpstreamModel: "video-test",
		Seconds: 6, Quality: "720p", Status: media.StatusInProgress, Progress: 1, CreatedAt: time.Now().UTC(), MetadataJSON: encodeVideoJobMetadata(videoInputMetadata{}),
	}
	service.runVideoJob(ctx, job, model.Route{ID: 1, Provider: account.ProviderBuild, PublicID: "video-test", UpstreamModel: "video-test"})
	if jobs.job.Status != media.StatusFailed || jobs.job.ErrorCode != "account_unavailable" {
		t.Fatalf("job = %#v, want fail-closed account_unavailable", jobs.job)
	}
	if len(adapter.generateAccountIDs) != 0 {
		t.Fatalf("generated accounts = %#v, want none", adapter.generateAccountIDs)
	}
}

func TestRunVideoJobDefersWhenClientKeyRepositoryIsTemporarilyUnavailable(t *testing.T) {
	jobs := &videoRunRepository{}
	keyService := clientkeyapp.NewService(videoClientKeyRepository{getErr: errors.New("database unavailable")}, nil, nil, 60, 4, nil)
	service := &Service{mediaJobs: jobs, clientKeys: keyService, audits: &durableVideoAuditRecorder{}, logger: slog.Default()}
	job := media.Job{
		ID: "video-client-key-runtime", ClientKeyID: 1, Status: media.StatusInProgress, Progress: 1,
		CreatedAt: time.Now().UTC(), MetadataJSON: encodeVideoJobMetadata(videoInputMetadata{}),
	}
	service.runVideoJob(context.Background(), job, model.Route{ID: 1, Provider: account.ProviderBuild, UpstreamModel: "video-test"})
	if jobs.job.Status != media.StatusInProgress || jobs.job.LeaseUntil == nil || jobs.job.CompletedAt != nil {
		t.Fatalf("job = %#v, want deferred in-progress job", jobs.job)
	}
}

type videoClientKeyRepository struct {
	repository.ClientKeyRepository
	getErr error
}

func (r videoClientKeyRepository) Get(context.Context, uint64) (clientkey.Key, error) {
	return clientkey.Key{}, r.getErr
}

type videoCredentialFailoverAdapter struct {
	refreshFailureID   uint64
	generateAccountIDs []uint64
}

func (a *videoCredentialFailoverAdapter) Provider() account.Provider { return account.ProviderBuild }

func (a *videoCredentialFailoverAdapter) Definition() provider.Definition {
	return provider.Definition{
		Provider:   account.ProviderBuild,
		Credential: provider.CredentialSurface{Refresh: true},
		Media:      provider.MediaSurface{VideoGeneration: true},
	}
}

func (a *videoCredentialFailoverAdapter) RefreshCredential(_ context.Context, credential account.Credential) (provider.RefreshedCredential, error) {
	if credential.ID == a.refreshFailureID {
		return provider.RefreshedCredential{}, errors.New("temporary credential refresh failure")
	}
	return provider.RefreshedCredential{EncryptedAccessToken: credential.EncryptedAccessToken, EncryptedRefreshToken: credential.EncryptedRefreshToken, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (a *videoCredentialFailoverAdapter) GenerateVideo(_ context.Context, request provider.VideoRequest) (provider.VideoResult, error) {
	a.generateAccountIDs = append(a.generateAccountIDs, request.Credential.ID)
	return provider.VideoResult{URL: "https://cdn.example.com/video.mp4", ContentType: "video/mp4"}, nil
}

type videoRunRepository struct {
	repository.MediaJobRepository
	job media.Job
}

func (r *videoRunRepository) UpdateMediaJob(_ context.Context, job media.Job) error {
	r.job = job
	return nil
}

func (r *videoRunRepository) MarkMediaJobUsageRecorded(_ context.Context, _ string, recordedAt time.Time) error {
	r.job.UsageRecordedAt = &recordedAt
	return nil
}
