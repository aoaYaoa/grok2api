package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	modeldomain "github.com/chenyme/grok2api/backend/internal/domain/model"
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
		Seconds: 8, Quality: "720p", Status: media.StatusCompleted, InputJSON: `{}`, CreatedAt: completedAt.Add(-time.Minute), CompletedAt: &completedAt,
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
	if recorder.last.EventID != "video_usage_video_usage_recovery" || recorder.last.EstimatedCostInUSDTicks <= 0 {
		t.Fatalf("audit = %#v", recorder.last)
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

func TestFindExtensionSourceAccountUsesOriginalVideoTask(t *testing.T) {
	repository := &videoRepairRepository{job: media.Job{
		ID: "video-source", RequestID: "source-task-1", ClientKeyID: 9, AccountID: 42,
		Status: media.StatusCompleted, UpstreamURL: "/v1/files/video/source.mp4",
	}}
	service := &Service{mediaJobs: repository}
	accountID, found, err := service.findExtensionSourceAccountID(context.Background(), 9, "source-task-1")
	if err != nil || !found || accountID != 42 {
		t.Fatalf("accountID=%d found=%v err=%v", accountID, found, err)
	}
}

func TestVideoExtensionAccountMetadataTracksUniqueAttempts(t *testing.T) {
	metadata := videoInputMetadata{}
	metadata.markAccountAttempt(12)
	metadata.markAccountAttempt(12)
	metadata.markAccountAttempt(34)

	if metadata.AccountRetryCount != 1 {
		t.Fatalf("account retry count = %d", metadata.AccountRetryCount)
	}
	if len(metadata.AttemptedAccountIDs) != 2 || metadata.AttemptedAccountIDs[0] != 12 || metadata.AttemptedAccountIDs[1] != 34 {
		t.Fatalf("attempted accounts = %#v", metadata.AttemptedAccountIDs)
	}
	excluded := metadata.excludedAccounts()
	if !excluded[12] || !excluded[34] || excluded[56] {
		t.Fatalf("excluded accounts = %#v", excluded)
	}
}

func TestVideoExtensionSwitchesOnTerminalAccountFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests} {
		if !shouldSwitchVideoAccount(videoStatusError{status: status}) {
			t.Fatalf("status %d should switch account", status)
		}
	}
	if shouldSwitchVideoAccount(videoStatusError{status: http.StatusInternalServerError}) {
		t.Fatal("server failures must not consume another account")
	}
}

type videoStatusError struct{ status int }

func (e videoStatusError) Error() string       { return http.StatusText(e.status) }
func (e videoStatusError) HTTPStatusCode() int { return e.status }

func TestAcquireVideoFallbackExcludesAttemptedAccountsForNormalGeneration(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-account-fallback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := relational.NewAccountRepository(database)
	first, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "first", SourceKey: "first", EncryptedAccessToken: "first", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "second", SourceKey: "second", EncryptedAccessToken: "second", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), nil, time.Hour, time.Second, time.Minute)
	repository := &videoRepairRepository{}
	service := &Service{selector: selector, mediaJobs: repository}
	service.UpdateMaxAttempts(3)
	metadata := videoInputMetadata{AttemptedAccountIDs: []uint64{first.ID}}
	job := media.Job{ID: "video_same_task", AccountID: first.ID, AccountName: first.Name, InputJSON: encodeVideoMetadata(metadata)}

	lease, err := service.acquireVideoAccountFallback(ctx, &job, modeldomain.Route{Provider: account.ProviderWeb, UpstreamModel: "grok-imagine-video"}, "", &metadata)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if lease.Credential.ID != second.ID || job.AccountID != second.ID || repository.job.ID != job.ID {
		t.Fatalf("lease=%d job=%d persisted=%#v", lease.Credential.ID, job.AccountID, repository.job)
	}
	if metadata.AccountRetryCount != 1 || len(metadata.AttemptedAccountIDs) != 2 {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestNormalVideoSwitchesAccountBeforeTerminalFailure(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-normal-failover.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := relational.NewAccountRepository(database)
	first, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "first", SourceKey: "first", EncryptedAccessToken: "first", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "second", SourceKey: "second", EncryptedAccessToken: "second", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &videoFailoverAdapter{failedAccountID: first.ID}
	registry := provider.NewRegistry(adapter)
	selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), registry, time.Hour, time.Second, time.Minute)
	repository := &videoRepairRepository{}
	service := &Service{providers: registry, selector: selector, mediaJobs: repository, audits: &durableVideoAuditRecorder{}, logger: slog.Default()}
	service.UpdateMaxAttempts(3)
	job := media.Job{
		ID: "video_normal_failover", RequestID: "request_normal_failover", ClientKeyID: 1,
		AccountID: first.ID, AccountName: first.Name, Provider: string(account.ProviderWeb), Model: "grok-imagine-video",
		UpstreamModel: "grok-imagine-video", Seconds: 6, Status: media.StatusInProgress, InputJSON: `{}`, CreatedAt: time.Now().UTC(),
	}
	service.runVideoJob(ctx, job, modeldomain.Route{Provider: account.ProviderWeb, UpstreamModel: "grok-imagine-video"})

	if !slices.Equal(adapter.accountIDs, []uint64{first.ID, second.ID}) {
		t.Fatalf("account attempts = %#v", adapter.accountIDs)
	}
	if repository.job.Status != media.StatusCompleted || repository.job.AccountID != second.ID {
		t.Fatalf("job = %#v", repository.job)
	}
	metadata := decodeVideoMetadata(repository.job.InputJSON)
	if metadata.AccountRetryCount != 1 || !slices.Equal(metadata.AttemptedAccountIDs, []uint64{first.ID, second.ID}) {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestEarlyIncompleteVideoStreamDefersSameTask(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-incomplete-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := relational.NewAccountRepository(database)
	credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "first", SourceKey: "first", EncryptedAccessToken: "first", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &videoRetryAdapter{}
	registry := provider.NewRegistry(adapter)
	selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), registry, time.Hour, time.Second, time.Minute)
	repository := &videoRepairRepository{}
	service := &Service{providers: registry, selector: selector, mediaJobs: repository, logger: slog.Default()}
	service.UpdateMaxAttempts(3)
	job := media.Job{
		ID: "video_incomplete_retry", RequestID: "request_incomplete_retry", ClientKeyID: 1,
		AccountID: credential.ID, AccountName: credential.Name, Provider: string(account.ProviderWeb), Model: "grok-imagine-video",
		UpstreamModel: "grok-imagine-video", Seconds: 6, Status: media.StatusInProgress, InputJSON: `{}`, CreatedAt: time.Now().UTC(),
	}
	service.runVideoJob(ctx, job, modeldomain.Route{Provider: account.ProviderWeb, UpstreamModel: "grok-imagine-video"})

	metadata := decodeVideoMetadata(repository.job.InputJSON)
	if repository.job.Status != media.StatusInProgress || repository.job.LeaseUntil == nil || metadata.RetryCount != 1 {
		t.Fatalf("job=%#v metadata=%#v", repository.job, metadata)
	}
	if delay := repository.job.LeaseUntil.Sub(repository.job.UpdatedAt); delay != 15*time.Second {
		t.Fatalf("retry delay = %s", delay)
	}
}

type videoFailoverAdapter struct {
	failedAccountID uint64
	accountIDs      []uint64
}

type videoRetryAdapter struct{}

func (*videoRetryAdapter) Provider() account.Provider { return account.ProviderWeb }

func (*videoRetryAdapter) GenerateVideo(context.Context, provider.VideoRequest) (provider.VideoResult, error) {
	return provider.VideoResult{}, retrySafeVideoStatusError{status: http.StatusBadGateway}
}

type retrySafeVideoStatusError struct{ status int }

func (e retrySafeVideoStatusError) Error() string         { return http.StatusText(e.status) }
func (e retrySafeVideoStatusError) HTTPStatusCode() int   { return e.status }
func (retrySafeVideoStatusError) MediaJobRetrySafe() bool { return true }

type accountNeutralVideoError struct{}

func (accountNeutralVideoError) Error() string              { return "request rejected by content policy" }
func (accountNeutralVideoError) HTTPStatusCode() int        { return http.StatusBadRequest }
func (accountNeutralVideoError) AccountHealthNeutral() bool { return true }

type accountNeutralVideoAdapter struct{}

func (*accountNeutralVideoAdapter) Provider() account.Provider { return account.ProviderWeb }

func (*accountNeutralVideoAdapter) GenerateVideo(context.Context, provider.VideoRequest) (provider.VideoResult, error) {
	return provider.VideoResult{}, accountNeutralVideoError{}
}

func TestVideoContentPolicyFailureDoesNotPenalizeAccount(t *testing.T) {
	if shouldSwitchVideoAccount(accountNeutralVideoError{}) {
		t.Fatal("request-scoped content rejection must not switch accounts")
	}
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-account-neutral.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := relational.NewAccountRepository(database)
	credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "first", SourceKey: "first", EncryptedAccessToken: "first", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &accountNeutralVideoAdapter{}
	registry := provider.NewRegistry(adapter)
	selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), registry, time.Hour, time.Second, time.Minute)
	repository := &videoRepairRepository{}
	service := &Service{providers: registry, selector: selector, mediaJobs: repository, audits: &durableVideoAuditRecorder{}, clientKeys: clientkeyapp.NewService(nil, nil, nil, 60, 4, nil), logger: slog.Default()}
	job := media.Job{
		ID: "video_account_neutral", RequestID: "request_account_neutral", ClientKeyID: 1,
		AccountID: credential.ID, AccountName: credential.Name, Provider: string(account.ProviderWeb), Model: "grok-imagine-video",
		UpstreamModel: "grok-imagine-video", Seconds: 6, Status: media.StatusInProgress, InputJSON: `{}`, CreatedAt: time.Now().UTC(),
	}
	service.runVideoJob(ctx, job, modeldomain.Route{Provider: account.ProviderWeb, UpstreamModel: "grok-imagine-video"})

	observed, err := accounts.Get(ctx, credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.FailureCount != 0 || observed.CooldownUntil != nil || observed.AuthStatus != account.AuthStatusActive {
		t.Fatalf("account was incorrectly penalized: %#v", observed)
	}
	if repository.job.Status != media.StatusFailed {
		t.Fatalf("job = %#v", repository.job)
	}
}

func (*videoFailoverAdapter) Provider() account.Provider { return account.ProviderWeb }

func (a *videoFailoverAdapter) GenerateVideo(_ context.Context, request provider.VideoRequest) (provider.VideoResult, error) {
	a.accountIDs = append(a.accountIDs, request.Credential.ID)
	if request.Credential.ID == a.failedAccountID {
		return provider.VideoResult{}, videoStatusError{status: http.StatusForbidden}
	}
	return provider.VideoResult{URL: "/v1/files/video/failover.mp4", ContentType: "video/mp4"}, nil
}

func TestRecoverVideoJobsArchivesCompletedProtectedOutputs(t *testing.T) {
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "video-repair.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := relational.NewAccountRepository(database)
	credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "web", SourceKey: "web",
		EncryptedAccessToken: "encrypted", Enabled: true, AuthStatus: account.AuthStatusActive, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	completedAt := time.Now().UTC()
	repository := &videoRepairRepository{job: media.Job{
		ID: "video_repair", RequestID: "request-repair", ClientKeyID: 1, AccountID: credential.ID,
		Provider: string(account.ProviderWeb), Model: "grok-imagine-video", ModelRouteID: 3, UpstreamModel: "Web/grok-imagine-video",
		Seconds: 6, Size: "3:2", Quality: "480p", Status: media.StatusCompleted, Progress: 100,
		UpstreamURL: "https://assets.grok.com/users/test/generated/video/generated_video.mp4", ContentType: "video/mp4",
		InputJSON: `{}`, CreatedAt: completedAt.Add(-time.Minute), UpdatedAt: completedAt, CompletedAt: &completedAt, UsageRecordedAt: &completedAt,
	}}
	adapter := &videoRepairAdapter{}
	registry := provider.NewRegistry(adapter)
	selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), memory.NewStickyStore(), registry, time.Hour, time.Second, time.Minute)
	service := &Service{
		models:    videoRepairRouteResolver{route: modeldomain.Route{ID: 3, Provider: account.ProviderWeb, UpstreamModel: "grok-imagine-video", Enabled: true}},
		providers: registry, selector: selector, mediaJobs: repository, logger: slog.Default(),
	}
	if err := service.RecoverVideoJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.archived != 1 || repository.job.UpstreamURL != "/v1/files/video/generated_video.mp4" {
		t.Fatalf("archive calls=%d repaired URL=%q", adapter.archived, repository.job.UpstreamURL)
	}
}

type videoRepairAdapter struct{ archived int }

func (*videoRepairAdapter) Provider() account.Provider { return account.ProviderWeb }
func (*videoRepairAdapter) GenerateVideo(context.Context, provider.VideoRequest) (provider.VideoResult, error) {
	return provider.VideoResult{}, errors.New("not used")
}
func (a *videoRepairAdapter) ArchiveVideo(_ context.Context, _ account.Credential, result provider.VideoResult) (provider.VideoResult, error) {
	a.archived++
	result.URL = "/v1/files/video/generated_video.mp4"
	return result, nil
}

type videoRepairRouteResolver struct{ route modeldomain.Route }

func (r videoRepairRouteResolver) Get(context.Context, uint64) (modeldomain.Route, error) {
	return r.route, nil
}
func (r videoRepairRouteResolver) GetByPublicID(context.Context, string) (modeldomain.Route, error) {
	return r.route, nil
}
func (r videoRepairRouteResolver) GetByPublicIDCandidates(context.Context, string) ([]modeldomain.Route, error) {
	return []modeldomain.Route{r.route}, nil
}
func (r videoRepairRouteResolver) GetByProviderUpstream(context.Context, account.Provider, string) (modeldomain.Route, error) {
	return r.route, nil
}

type videoRepairRepository struct{ job media.Job }

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

func (r *videoRepairRepository) CreateMediaJob(context.Context, media.Job) error { return nil }
func (r *videoRepairRepository) GetMediaJob(context.Context, string, uint64) (media.Job, error) {
	return r.job, nil
}
func (r *videoRepairRepository) ListMediaJobsByClientKey(context.Context, uint64, int, int) ([]media.Job, int64, error) {
	return []media.Job{r.job}, 1, nil
}
func (r *videoRepairRepository) UpdateMediaJob(_ context.Context, job media.Job) error {
	r.job = job
	return nil
}
func (*videoRepairRepository) DeleteOwnedTerminalMediaJob(context.Context, string, uint64) (bool, error) {
	return false, nil
}
func (r *videoRepairRepository) ListMediaJobs(_ context.Context, query repository.MediaJobListQuery) ([]media.Job, int64, error) {
	if query.Filter.Status == string(media.StatusCompleted) {
		return []media.Job{r.job}, 1, nil
	}
	return nil, 0, nil
}
func (*videoRepairRepository) SummarizeMediaJobs(context.Context) (repository.MediaJobStats, error) {
	return repository.MediaJobStats{}, nil
}
func (*videoRepairRepository) ListRecoverableMediaJobs(context.Context, int) ([]media.Job, error) {
	return nil, nil
}
func (*videoRepairRepository) ListUnrecordedTerminalMediaJobs(context.Context, int) ([]media.Job, error) {
	return nil, nil
}
func (*videoRepairRepository) TryClaimMediaJob(context.Context, string, time.Time, time.Time, string) (media.Job, bool, error) {
	return media.Job{}, false, nil
}
func (*videoRepairRepository) MarkMediaJobUsageRecorded(context.Context, string, time.Time) error {
	return nil
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

func (r *videoUsageRepository) CreateMediaJob(context.Context, media.Job) error { return nil }

func (r *videoUsageRepository) GetMediaJob(context.Context, string, uint64) (media.Job, error) {
	return r.job, nil
}

func (r *videoUsageRepository) ListMediaJobsByClientKey(context.Context, uint64, int, int) ([]media.Job, int64, error) {
	return []media.Job{r.job}, 1, nil
}

func (r *videoUsageRepository) UpdateMediaJob(context.Context, media.Job) error { return nil }

func (r *videoUsageRepository) DeleteOwnedTerminalMediaJob(_ context.Context, id string, clientKeyID uint64) (bool, error) {
	if r.job.ID != id || r.job.ClientKeyID != clientKeyID || (r.job.Status != media.StatusCompleted && r.job.Status != media.StatusFailed) {
		return false, nil
	}
	r.job = media.Job{}
	return true, nil
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
	raw := withVideoPosterURL(`{"reference_urls":[]}`, "/v1/files/image/poster.jpg")
	if got := videoPosterURL(media.Job{InputJSON: raw}); got != "/v1/files/image/poster.jpg" {
		t.Fatalf("poster URL = %q, input = %s", got, raw)
	}
	if got := withVideoPosterURL(raw, ""); got != raw {
		t.Fatalf("empty poster changed metadata: %s", got)
	}
}
