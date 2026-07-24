package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
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

func TestFastVideoFallbackIsNeverDeferredOrRetried(t *testing.T) {
	err := &terminalVideoError{}
	status, ok := provider.ErrorHTTPStatus(err)
	if !ok || status != http.StatusBadGateway || provider.IsMediaJobRetrySafe(err) || !provider.IsAccountHealthNeutral(err) {
		t.Fatalf("status=%d classified=%v retry=%v neutral=%v", status, ok, provider.IsMediaJobRetrySafe(err), provider.IsAccountHealthNeutral(err))
	}
	if _, retry := videoRetryPlan(status, 0, provider.IsMediaJobRetrySafe(err)); retry {
		t.Fatal("fast fallback must not trigger an automatic account retry")
	}
	if message := publicVideoFailureMessage(err); message != "上游返回了异常快速的兜底视频，已停止任务且不会自动重试" {
		t.Fatalf("public message = %q", message)
	}
}

type terminalVideoError struct{}

func (*terminalVideoError) Error() string           { return "fast video fallback" }
func (*terminalVideoError) HTTPStatusCode() int     { return http.StatusBadGateway }
func (*terminalVideoError) MediaJobRetrySafe() bool { return false }
func (*terminalVideoError) AccountHealthNeutral() bool {
	return true
}
func (*terminalVideoError) MediaJobPublicMessage() string {
	return "上游返回了异常快速的兜底视频，已停止任务且不会自动重试"
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
