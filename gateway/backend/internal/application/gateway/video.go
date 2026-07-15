package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/domain/model"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/pkg/batch"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

const (
	videoJobTimeout          = 2 * time.Hour
	videoJobLease            = videoJobTimeout + 5*time.Minute
	videoJobRecoveryInterval = 30 * time.Second
)

type VideoInput struct {
	RequestID          string
	ClientKey          clientkey.Key
	PublicModel        string
	Prompt             string
	Duration           int
	AspectRatio        string
	Resolution         string
	ReferenceURLs      []string
	IsExtension        bool
	ExtendPostID       string
	ExtensionStartTime float64
	OriginalPostID     string
	FileAttachmentID   string
	StitchWithExtend   bool
}

func (s *Service) CreateVideo(ctx context.Context, input VideoInput) (media.Job, error) {
	if s.mediaJobs == nil || s.mediaQueue == nil {
		return media.Job{}, fmt.Errorf("视频任务服务未配置")
	}
	if len(input.Prompt) > 100000 || (len(input.Prompt) == 0 && len(input.ReferenceURLs) == 0 && !input.IsExtension) {
		return media.Job{}, fmt.Errorf("文本生视频必须提供 prompt；图片生视频可以省略 prompt")
	}
	routes, err := s.models.GetByPublicIDCandidates(ctx, input.PublicModel)
	if err != nil {
		return media.Job{}, ErrModelNotFound
	}
	route, err := s.selectMediaRoute(routes, input.ClientKey, model.CapabilityVideo, func(providerValue account.Provider) bool {
		_, ok := s.providers.Videos(providerValue)
		return ok
	})
	if err != nil {
		return media.Job{}, err
	}
	externalModel := model.ExternalPublicID(route.Provider, route.PublicID)
	quotaMode := s.providers.QuotaMode(route.Provider, route.UpstreamModel)
	lease, err := s.selector.Acquire(ctx, route.Provider, route.UpstreamModel, quotaMode, "", nil, false)
	if err != nil {
		return media.Job{}, fmt.Errorf("%w: %w", ErrNoAvailableAccount, err)
	}
	accountID := lease.Credential.ID
	lease.Release()
	token, err := security.NewOpaqueToken(18)
	if err != nil {
		return media.Job{}, err
	}
	now := time.Now().UTC()
	job := media.Job{
		ID: "video_" + token, RequestID: input.RequestID,
		ClientKeyID: input.ClientKey.ID, ClientKeyName: input.ClientKey.Name,
		AccountID: accountID, AccountName: lease.Credential.Name,
		Provider: string(route.Provider), Model: externalModel, ModelRouteID: route.ID, UpstreamModel: model.DisplayUpstreamModel(route.Provider, route.UpstreamModel), Prompt: input.Prompt,
		Seconds: input.Duration, Size: input.AspectRatio, Quality: input.Resolution,
		Status: media.StatusQueued, Progress: 0, InputJSON: encodeVideoInput(input), CreatedAt: now, UpdatedAt: now,
	}
	reserved := false
	if pricing, ok := audit.EstimateOfficialVideoCost(externalModel, input.Resolution, input.Duration); ok {
		reserved, err = s.clientKeys.ReserveBilling(ctx, input.ClientKey, "video_usage_"+job.ID, pricing.CostInUSDTicks, mediaBillingReservationTTL)
		if err != nil {
			return media.Job{}, err
		}
	}
	if err := s.mediaJobs.CreateMediaJob(ctx, job); err != nil {
		if reserved {
			s.cancelBillingReservation("video_usage_" + job.ID)
		}
		return media.Job{}, err
	}
	if !s.enqueueVideoJob(job.ID) {
		s.logger.Warn("video_job_queue_full", "job_id", job.ID)
	}
	return job, nil
}

func (s *Service) GetVideo(ctx context.Context, id string, key clientkey.Key) (media.Job, error) {
	if s.mediaJobs == nil {
		return media.Job{}, ErrResponseNotFound
	}
	job, err := s.mediaJobs.GetMediaJob(ctx, id, key.ID)
	if err != nil {
		return media.Job{}, ErrResponseNotFound
	}
	return job, nil
}

func (s *Service) ListVideos(ctx context.Context, key clientkey.Key, page, pageSize int) ([]media.Job, int64, error) {
	if s.mediaJobs == nil {
		return nil, 0, ErrResponseNotFound
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 100
	}
	values, total, err := s.mediaJobs.ListMediaJobsByClientKey(ctx, key.ID, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, err
	}
	for index := range values {
		values[index].DisplayName = decodeVideoMetadata(values[index].InputJSON).DisplayName
	}
	return values, total, nil
}

func (s *Service) RenameVideo(ctx context.Context, identifier, displayName string, key clientkey.Key) (media.Job, error) {
	identifier = strings.TrimSpace(identifier)
	displayName = strings.TrimSpace(displayName)
	if identifier == "" || len(displayName) > 160 {
		return media.Job{}, fmt.Errorf("视频标识不能为空且标题不能超过 160 个字符")
	}
	for page := 1; ; page++ {
		values, total, err := s.mediaJobs.ListMediaJobsByClientKey(ctx, key.ID, (page-1)*1000, 1000)
		if err != nil {
			return media.Job{}, err
		}
		for _, job := range values {
			if job.ID != identifier && job.RequestID != identifier && job.UpstreamURL != identifier && !strings.Contains(job.UpstreamURL, identifier) {
				continue
			}
			metadata := decodeVideoMetadata(job.InputJSON)
			metadata.DisplayName = displayName
			job.InputJSON = encodeVideoMetadata(metadata)
			job.DisplayName = displayName
			job.UpdatedAt = time.Now().UTC()
			if err := s.mediaJobs.UpdateMediaJob(ctx, job); err != nil {
				return media.Job{}, err
			}
			return job, nil
		}
		if page*1000 >= int(total) || len(values) == 0 {
			break
		}
	}
	return media.Job{}, ErrResponseNotFound
}

func (s *Service) CancelVideo(ctx context.Context, id string, key clientkey.Key) (media.Job, error) {
	if s.mediaJobs == nil {
		return media.Job{}, ErrResponseNotFound
	}
	job, err := s.mediaJobs.GetMediaJob(ctx, id, key.ID)
	if err != nil {
		return media.Job{}, ErrResponseNotFound
	}
	if job.Status == media.StatusCompleted || job.Status == media.StatusFailed {
		return job, nil
	}
	s.mediaMu.Lock()
	if s.mediaCancelled == nil {
		s.mediaCancelled = make(map[string]bool)
	}
	s.mediaCancelled[id] = true
	cancel := s.mediaCancels[id]
	s.mediaMu.Unlock()
	if cancel != nil {
		cancel()
	}
	now := time.Now().UTC()
	job.Status = media.StatusFailed
	job.ErrorCode = "cancelled"
	job.ErrorMessage = "任务已取消"
	job.LeaseUntil = nil
	job.ClaimToken = ""
	job.UpdatedAt = now
	job.CompletedAt = &now
	if err := s.mediaJobs.UpdateMediaJob(ctx, job); err != nil {
		return media.Job{}, err
	}
	s.cancelBillingReservation("video_usage_" + job.ID)
	time.AfterFunc(videoJobLease, func() {
		s.mediaMu.Lock()
		delete(s.mediaCancelled, id)
		s.mediaMu.Unlock()
	})
	return job, nil
}

func (s *Service) RecoverVideoJobs(ctx context.Context) error {
	if s.mediaJobs == nil {
		return nil
	}
	usageErr := s.reconcileVideoUsage(ctx)
	repairErr := s.repairCompletedVideoOutputs(ctx)
	values, err := s.mediaJobs.ListRecoverableMediaJobs(ctx, 1000)
	if err != nil {
		return errors.Join(usageErr, repairErr, err)
	}
	for _, job := range values {
		if !s.enqueueVideoJob(job.ID) {
			break
		}
	}
	return errors.Join(usageErr, repairErr)
}

type videoOutputArchiver interface {
	ArchiveVideo(context.Context, account.Credential, provider.VideoResult) (provider.VideoResult, error)
}

// repairCompletedVideoOutputs 补齐 Go 切换前后遗漏的受保护上游视频缓存。
func (s *Service) repairCompletedVideoOutputs(ctx context.Context) error {
	if s.models == nil || s.providers == nil || s.selector == nil {
		return nil
	}
	values, _, err := s.mediaJobs.ListMediaJobs(ctx, repository.MediaJobListQuery{
		Page: repository.PageQuery{Limit: 1000}, Filter: repository.MediaJobListFilter{Status: string(media.StatusCompleted)},
	})
	if err != nil {
		return err
	}
	var result error
	for _, summary := range values {
		job, loadErr := s.mediaJobs.GetMediaJob(ctx, summary.ID, summary.ClientKeyID)
		if loadErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 读取完整记录: %w", summary.ID, loadErr))
			continue
		}
		if !protectedVideoOutput(job.UpstreamURL) {
			continue
		}
		route, routeErr := s.models.Get(ctx, job.ModelRouteID)
		if routeErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 查询模型路由: %w", job.ID, routeErr))
			continue
		}
		adapter, ok := s.providers.Videos(route.Provider)
		if !ok {
			continue
		}
		archiver, ok := adapter.(videoOutputArchiver)
		if !ok {
			continue
		}
		lease, leaseErr := s.selector.AcquirePinned(ctx, route.Provider, job.AccountID, route.UpstreamModel, "", false)
		if leaseErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 获取原账号: %w", job.ID, leaseErr))
			continue
		}
		archived, archiveErr := archiver.ArchiveVideo(ctx, lease.Credential, provider.VideoResult{URL: job.UpstreamURL, ContentType: job.ContentType})
		lease.Release()
		if archiveErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 保存视频: %w", job.ID, archiveErr))
			continue
		}
		job.UpstreamURL, job.ContentType, job.UpdatedAt = archived.URL, archived.ContentType, time.Now().UTC()
		if updateErr := s.mediaJobs.UpdateMediaJob(ctx, job); updateErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 更新本地地址: %w", job.ID, updateErr))
		}
	}
	return result
}

func protectedVideoOutput(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "https://assets.grok.com/") || strings.HasPrefix(value, "https://imagine-public.x.ai/")
}

// RunVideoWorkers 使用固定 Worker 处理持久化任务，避免突发请求按任务创建无界 goroutine。
func (s *Service) RunVideoWorkers(ctx context.Context) {
	if s.mediaQueue == nil || s.mediaWorker <= 0 {
		return
	}
	var workers sync.WaitGroup
	workers.Add(s.mediaWorker)
	for range s.mediaWorker {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-s.mediaQueue:
					err := batch.Do(ctx, func(workCtx context.Context) error {
						s.processVideoJob(workCtx, id)
						return nil
					})
					s.mediaMu.Lock()
					delete(s.mediaQueued, id)
					s.mediaMu.Unlock()
					if err != nil && ctx.Err() == nil {
						if panicErr, ok := err.(*batch.PanicError); ok {
							s.logger.Error("video_worker_panicked", "job_id", id, "error", panicErr, "stack", string(panicErr.Stack))
						} else {
							s.logger.Error("video_worker_failed", "job_id", id, "error", err)
						}
					}
				}
			}
		}()
	}
	workers.Wait()
}

func (s *Service) enqueueVideoJob(id string) bool {
	if id == "" || s.mediaQueue == nil {
		return false
	}
	s.mediaMu.Lock()
	if _, exists := s.mediaQueued[id]; exists {
		s.mediaMu.Unlock()
		return true
	}
	s.mediaQueued[id] = struct{}{}
	s.mediaMu.Unlock()
	select {
	case s.mediaQueue <- id:
		return true
	default:
		s.mediaMu.Lock()
		delete(s.mediaQueued, id)
		s.mediaMu.Unlock()
		full := s.mediaQueueFull.Add(1)
		if s.logger != nil && (full == 1 || full%100 == 0) {
			s.logger.Warn("video_queue_full", "count", full, "queued", len(s.mediaQueue), "capacity", cap(s.mediaQueue))
		}
		return false
	}
}

func (s *Service) processVideoJob(ctx context.Context, id string) {
	job, claimed, err := s.claimVideoJob(ctx, id)
	if err != nil {
		s.logger.Warn("video_job_claim_failed", "job_id", id, "error", err)
		return
	}
	if !claimed {
		return
	}
	var route model.Route
	if job.ModelRouteID != 0 {
		route, err = s.models.Get(ctx, job.ModelRouteID)
	} else {
		route, err = s.models.GetByPublicID(ctx, job.Model)
	}
	if err != nil {
		s.failVideoJob(ctx, job, "model_not_found", errors.New("模型路由不存在"))
		return
	}
	s.runVideoJob(ctx, job, route)
}

// RunVideoRecovery 周期认领新建后未启动或执行实例失联后的媒体任务。
func (s *Service) RunVideoRecovery(ctx context.Context) {
	ticker := time.NewTicker(videoJobRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RecoverVideoJobs(ctx); err != nil {
				s.logger.Warn("video_job_recovery_failed", "error", err)
			}
		}
	}
}

func (s *Service) claimVideoJob(ctx context.Context, id string) (media.Job, bool, error) {
	now := time.Now().UTC()
	claimToken, err := security.NewOpaqueToken(18)
	if err != nil {
		return media.Job{}, false, err
	}
	return s.mediaJobs.TryClaimMediaJob(ctx, id, now, now.Add(videoJobLease), claimToken)
}

func (s *Service) runVideoJob(parent context.Context, job media.Job, route model.Route) {
	ctx, cancel := context.WithTimeout(parent, videoJobTimeout)
	defer cancel()
	ctx, egressTrace := infraegress.WithTrace(ctx)
	s.mediaMu.Lock()
	if s.mediaCancels == nil {
		s.mediaCancels = make(map[string]context.CancelFunc)
	}
	s.mediaCancels[job.ID] = cancel
	cancelled := s.mediaCancelled[job.ID]
	s.mediaMu.Unlock()
	defer func() {
		s.mediaMu.Lock()
		delete(s.mediaCancels, job.ID)
		delete(s.mediaCancelled, job.ID)
		s.mediaMu.Unlock()
	}()
	if cancelled {
		cancel()
		return
	}
	startedAt := time.Now()
	job.Progress = max(job.Progress, 1)
	job.UpdatedAt = time.Now().UTC()
	if err := s.mediaJobs.UpdateMediaJob(ctx, job); err != nil {
		s.logger.Warn("video_job_progress_write_failed", "job_id", job.ID, "error", err)
	}
	lease, err := s.selector.AcquirePinned(ctx, route.Provider, job.AccountID, route.UpstreamModel, "", true)
	if err != nil {
		if s.videoCancelled(job.ID) {
			return
		}
		if parent.Err() != nil {
			s.deferVideoJob(parent, job)
			return
		}
		s.failVideoJob(parent, job, "account_unavailable", err)
		return
	}
	defer lease.Release()
	adapter, ok := s.providers.Videos(route.Provider)
	if !ok {
		s.failVideoJob(parent, job, "provider_unavailable", ErrNoAvailableAccount)
		return
	}
	lastProgress := job.Progress
	metadata := decodeVideoMetadata(job.InputJSON)
	result, err := adapter.GenerateVideo(ctx, provider.VideoRequest{
		Credential: lease.Credential, Prompt: job.Prompt, Duration: job.Seconds, AspectRatio: job.Size, Resolution: job.Quality,
		ReferenceURLs: metadata.ImageURLs, IsExtension: metadata.IsExtension, ExtendPostID: metadata.ExtendPostID,
		ExtensionStartTime: metadata.ExtensionStartTime, OriginalPostID: metadata.OriginalPostID,
		FileAttachmentID: metadata.FileAttachmentID, StitchWithExtend: metadata.StitchWithExtend,
		Progress: func(value int) {
			value = min(99, max(1, value))
			if value-lastProgress < 5 {
				return
			}
			lastProgress = value
			job.Progress, job.UpdatedAt = value, time.Now().UTC()
			leaseUntil := job.UpdatedAt.Add(videoJobLease)
			job.LeaseUntil = &leaseUntil
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.mediaJobs.UpdateMediaJob(updateCtx, job)
			updateCancel()
		},
	})
	if err != nil {
		if s.videoCancelled(job.ID) {
			return
		}
		if parent.Err() != nil {
			s.deferVideoJob(parent, job)
			return
		}
		failureCtx, failureCancel := context.WithTimeout(context.Background(), finalizationTimeout)
		failureHandled := false
		if errors.Is(err, provider.ErrUnauthorized) {
			if lease.Credential.AuthType == account.AuthTypeSSO {
				_ = s.accounts.MarkReauthRequired(failureCtx, lease.Credential.ID, fmt.Sprintf("%s SSO credential rejected", lease.Credential.Provider))
			}
			s.selector.MarkFailure(failureCtx, lease.Credential, http.StatusUnauthorized, 0)
			failureHandled = true
		} else if status, ok := provider.ErrorHTTPStatus(err); ok {
			switch {
			case status == http.StatusForbidden && s.providers.RetryForbiddenAsEgress(lease.Credential.Provider):
				failureHandled = true
			case (status == http.StatusPaymentRequired || status == http.StatusTooManyRequests) && lease.QuotaMode != "":
				exhausted, reconcileErr := s.accounts.ReconcileRateLimit(failureCtx, lease.Credential.ID, lease.QuotaMode, 0)
				s.selector.MarkQuotaStateChanged(lease.Credential.Provider)
				if reconcileErr != nil || !exhausted {
					s.selector.MarkFailure(failureCtx, lease.Credential, status, 0)
				}
				failureHandled = true
			case status >= http.StatusInternalServerError:
				failureHandled = true
			default:
				s.selector.MarkFailure(failureCtx, lease.Credential, status, 0)
				failureHandled = true
			}
		}
		if !failureHandled && !provider.IsMediaPostProcessingError(err) {
			s.selector.MarkFailure(failureCtx, lease.Credential, 0, 0)
		}
		failureCancel()
		applyMediaJobEgress(&job, egressTrace, route.Provider)
		s.failVideoJob(parent, job, "generation_failed", err)
		return
	}
	if s.videoCancelled(job.ID) {
		return
	}
	now := time.Now().UTC()
	job.Status, job.Progress, job.UpstreamURL, job.ContentType = media.StatusCompleted, 100, result.URL, result.ContentType
	applyMediaJobEgress(&job, egressTrace, route.Provider)
	job.LeaseUntil, job.UpdatedAt, job.CompletedAt = nil, now, &now
	if err := s.persistVideoJobWithRetry(parent, job); err != nil {
		s.logger.Error("video_job_terminal_write_failed", "job_id", job.ID, "error", err)
		return
	}
	s.selector.MarkSuccess(context.Background(), lease.Credential)
	if err := s.recordVideoAudit(context.Background(), job, time.Since(startedAt).Milliseconds()); err != nil {
		s.logger.Error("video_usage_record_failed", "job_id", job.ID, "event_id", "video_usage_"+job.ID, "error", err)
	}
	if quotaKind, _ := s.providers.QuotaKind(route.Provider); quotaKind == provider.QuotaRemoteWindow && lease.QuotaMode == "weekly" {
		s.accounts.QueueQuotaRefresh(job.AccountID, lease.QuotaMode)
	}
}

func (s *Service) videoCancelled(id string) bool {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	return s.mediaCancelled[id]
}

func (s *Service) reconcileVideoUsage(ctx context.Context) error {
	jobs, err := s.mediaJobs.ListUnrecordedTerminalMediaJobs(ctx, 200)
	if err != nil {
		return err
	}
	var result error
	for _, job := range jobs {
		durationMS := int64(0)
		if job.CompletedAt != nil {
			durationMS = max(int64(0), job.CompletedAt.Sub(job.CreatedAt).Milliseconds())
		}
		if err := s.recordVideoAudit(ctx, job, durationMS); err != nil {
			result = firstError(result, fmt.Errorf("任务 %s: %w", job.ID, err))
		}
	}
	return result
}

func (s *Service) recordVideoAudit(ctx context.Context, job media.Job, durationMS int64) error {
	accountID := job.AccountID
	createdAt := time.Now().UTC()
	if job.CompletedAt != nil && !job.CompletedAt.IsZero() {
		createdAt = job.CompletedAt.UTC()
	}
	statusCode := http.StatusOK
	if job.Status == media.StatusFailed {
		statusCode = http.StatusBadGateway
		switch job.ErrorCode {
		case "account_unavailable", "provider_unavailable":
			statusCode = http.StatusServiceUnavailable
		case "model_not_found":
			statusCode = http.StatusNotFound
		}
	}
	record := audit.Record{
		EventID: "video_usage_" + job.ID, RequestID: job.RequestID, ClientKeyID: job.ClientKeyID, ClientKeyName: job.ClientKeyName,
		ModelRouteID: job.ModelRouteID, ModelPublicID: job.Model, ModelUpstreamModel: job.UpstreamModel,
		Provider: job.Provider, Operation: audit.OperationVideo, UsageSource: audit.UsageSourceNone,
		AccountID: &accountID, AccountName: job.AccountName, StatusCode: statusCode, ErrorCode: job.ErrorCode,
		EgressNodeID: job.EgressNodeID, EgressNodeName: job.EgressNodeName, EgressScope: job.EgressScope, EgressMode: audit.EgressMode(job.EgressMode),
		MediaInputImages: int64(len(decodeVideoInput(job.InputJSON))),
		DurationMS:       durationMS, CreatedAt: createdAt,
	}
	if job.Status == media.StatusCompleted {
		record.MediaOutputSeconds = int64(max(0, job.Seconds))
	}
	if pricing, ok := audit.EstimateOfficialVideoCost(job.Model, job.Quality, job.Seconds); ok && job.Status == media.StatusCompleted {
		record.EstimatedCostInUSDTicks = pricing.CostInUSDTicks
		record.PricingModel = pricing.Model
		record.PricingVersion = audit.OfficialPricingAsOf
	}
	if durable, ok := s.audits.(interface {
		CreateDurable(context.Context, audit.Record) error
	}); ok {
		if err := durable.CreateDurable(ctx, record); err != nil {
			return err
		}
	} else if err := s.audits.Create(ctx, record); err != nil {
		return err
	}
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	return s.mediaJobs.MarkMediaJobUsageRecorded(markCtx, job.ID, time.Now().UTC())
}

type videoInputMetadata struct {
	ImageURLs          []string `json:"image_urls"`
	DisplayName        string   `json:"display_name,omitempty"`
	IsExtension        bool     `json:"is_extension,omitempty"`
	ExtendPostID       string   `json:"extend_post_id,omitempty"`
	ExtensionStartTime float64  `json:"extension_start_time,omitempty"`
	OriginalPostID     string   `json:"original_post_id,omitempty"`
	FileAttachmentID   string   `json:"file_attachment_id,omitempty"`
	StitchWithExtend   bool     `json:"stitch_with_extend,omitempty"`
}

func encodeVideoInput(input VideoInput) string {
	return encodeVideoMetadata(videoInputMetadata{ImageURLs: input.ReferenceURLs, IsExtension: input.IsExtension, ExtendPostID: input.ExtendPostID, ExtensionStartTime: input.ExtensionStartTime, OriginalPostID: input.OriginalPostID, FileAttachmentID: input.FileAttachmentID, StitchWithExtend: input.StitchWithExtend})
}

func encodeVideoMetadata(metadata videoInputMetadata) string {
	data, _ := json.Marshal(metadata)
	return string(data)
}

func decodeVideoInput(value string) []string {
	return decodeVideoMetadata(value).ImageURLs
}

func decodeVideoMetadata(value string) videoInputMetadata {
	var input videoInputMetadata
	_ = json.Unmarshal([]byte(value), &input)
	return input
}

func (s *Service) failVideoJob(ctx context.Context, job media.Job, code string, err error) {
	now := time.Now().UTC()
	job.Status, job.ErrorCode, job.ErrorMessage = media.StatusFailed, code, err.Error()
	if len(job.ErrorMessage) > 512 {
		job.ErrorMessage = job.ErrorMessage[:512]
	}
	job.LeaseUntil, job.UpdatedAt, job.CompletedAt = nil, now, &now
	if updateErr := s.persistVideoJobWithRetry(ctx, job); updateErr != nil {
		s.logger.Error("video_job_terminal_write_failed", "job_id", job.ID, "error", updateErr)
		return
	}
	if auditErr := s.recordVideoAudit(context.Background(), job, max(int64(0), now.Sub(job.CreatedAt).Milliseconds())); auditErr != nil {
		s.logger.Error("video_usage_record_failed", "job_id", job.ID, "event_id", "video_usage_"+job.ID, "error", auditErr)
	}
	s.cancelBillingReservation("video_usage_" + job.ID)
}

func (s *Service) deferVideoJob(ctx context.Context, job media.Job) {
	now := time.Now().UTC()
	leaseUntil := now.Add(5 * time.Minute)
	job.Status = media.StatusInProgress
	job.LeaseUntil = &leaseUntil
	job.UpdatedAt = now
	job.ErrorCode = ""
	job.ErrorMessage = ""
	if err := s.persistVideoJobWithRetry(ctx, job); err != nil {
		s.logger.Error("video_job_defer_write_failed", "job_id", job.ID, "error", err)
	}
}

// persistVideoJobWithRetry 至少执行一次收尾写入；后续退避可被工作进程关闭信号取消。
func (s *Service) persistVideoJobWithRetry(ctx context.Context, job media.Job) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		lastErr = s.mediaJobs.UpdateMediaJob(writeCtx, job)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Join(lastErr, ctx.Err())
			case <-timer.C:
			}
		}
	}
	return lastErr
}
