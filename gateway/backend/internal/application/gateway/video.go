package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	videoOutputAttempts      = 3
)

type VideoInput struct {
	RequestID          string
	ClientKey          clientkey.Key
	PublicModel        string
	Prompt             string
	Preset             string
	Duration           int
	AspectRatio        string
	Resolution         string
	ReferenceURLs      []string
	IsExtension        bool
	SourceTaskID       string
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
	if err := s.checkLedgerReady(); err != nil {
		return media.Job{}, err
	}
	externalModel := model.ExternalPublicID(route.Provider, route.PublicID)
	quotaMode := s.providers.QuotaMode(route.Provider, route.UpstreamModel)
	metadata := videoMetadataFromInput(input)
	var lease *accountLease
	sourceAccountID, sourceFound, sourceErr := s.findExtensionSourceAccountID(ctx, input.ClientKey.ID, input.SourceTaskID)
	if sourceErr != nil {
		return media.Job{}, sourceErr
	}
	if input.IsExtension && sourceFound {
		metadata.markAccountAttempt(sourceAccountID)
		lease, err = s.selector.AcquirePinned(ctx, route.Provider, sourceAccountID, route.ID, route.UpstreamModel, quotaMode, true)
		if err != nil && metadata.canTryAnotherAccount(s.videoAccountAttemptLimit()) {
			lease, err = s.selector.Acquire(ctx, route.Provider, route.ID, route.UpstreamModel, quotaMode, "", metadata.excludedAccounts(), false)
		}
	} else {
		lease, err = s.selector.Acquire(ctx, route.Provider, route.ID, route.UpstreamModel, quotaMode, "", nil, false)
	}
	if err != nil {
		return media.Job{}, fmt.Errorf("%w: %w", ErrNoAvailableAccount, err)
	}
	if input.IsExtension {
		metadata.markAccountAttempt(lease.Credential.ID)
	}
	inputJSON, err := encodeVideoInput(input.ReferenceURLs)
	if err != nil {
		lease.Release()
		return media.Job{}, err
	}
	metadataJSON := encodeVideoJobMetadata(metadata)
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
		Status: media.StatusQueued, Progress: 0, InputJSON: inputJSON, MetadataJSON: metadataJSON, InputImageCount: len(input.ReferenceURLs), CreatedAt: now, UpdatedAt: now,
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

func (s *Service) findExtensionSourceAccountID(ctx context.Context, clientKeyID uint64, sourceTaskID string) (uint64, bool, error) {
	sourceTaskID = strings.TrimSpace(sourceTaskID)
	if sourceTaskID == "" || s.mediaJobs == nil {
		return 0, false, nil
	}
	for offset := 0; ; offset += 200 {
		values, total, err := s.mediaJobs.ListMediaJobsByClientKey(ctx, clientKeyID, offset, 200)
		if err != nil {
			return 0, false, err
		}
		for _, job := range values {
			if job.RequestID == sourceTaskID || job.ID == sourceTaskID {
				return job.AccountID, true, nil
			}
		}
		if len(values) == 0 || offset+len(values) >= int(total) {
			return 0, false, nil
		}
	}
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

func (s *Service) DeleteVideo(ctx context.Context, id string, key clientkey.Key) (bool, error) {
	if s.mediaJobs == nil {
		return false, ErrResponseNotFound
	}
	return s.mediaJobs.DeleteOwnedTerminalMediaJob(ctx, strings.TrimSpace(id), key.ID)
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
		values[index].DisplayName = videoMetadataForJob(values[index]).DisplayName
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
			metadata := videoMetadataForJob(job)
			metadata.DisplayName = displayName
			job.MetadataJSON = encodeVideoJobMetadata(metadata)
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

func (s *Service) OpenVideoContent(ctx context.Context, id string, key clientkey.Key) (io.ReadCloser, string, int64, error) {
	job, err := s.GetVideo(ctx, id, key)
	if err != nil {
		return nil, "", 0, err
	}
	if job.Status != media.StatusCompleted {
		return nil, "", 0, fmt.Errorf("视频内容尚未可用")
	}
	// 本地资产优先：XAI ZDR 上传完成后不经公网回环下载。
	if job.ResultAssetID != "" && s.mediaAssets != nil {
		asset, body, openErr := s.mediaAssets.OpenVideo(ctx, job.ResultAssetID)
		if openErr == nil {
			return body, asset.MIMEType, asset.SizeBytes, nil
		}
	}
	if job.UpstreamURL == "" {
		return nil, "", 0, fmt.Errorf("视频内容尚未可用")
	}
	adapter, ok := s.providers.Videos(account.Provider(job.Provider))
	if !ok {
		return nil, "", 0, ErrResponseAccountUnavailable
	}
	downloader, ok := adapter.(provider.VideoContentDownloader)
	if !ok || s.selector == nil || s.selector.accounts == nil || s.accounts == nil {
		return nil, "", 0, ErrResponseAccountUnavailable
	}
	credential, err := s.selector.accounts.Get(ctx, job.AccountID)
	if err != nil {
		return nil, "", 0, ErrResponseAccountUnavailable
	}
	credential, err = s.accounts.EnsureCredential(ctx, credential, false)
	if err != nil {
		return nil, "", 0, ErrResponseAccountUnavailable
	}
	return downloader.DownloadVideo(ctx, credential, job.UpstreamURL)
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
		lease, leaseErr := s.selector.AcquirePinned(ctx, route.Provider, job.AccountID, route.ID, route.UpstreamModel, "", false)
		if leaseErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 获取原账号: %w", job.ID, leaseErr))
			continue
		}
		archived, archiveErr := archiver.ArchiveVideo(ctx, lease.Credential, provider.VideoResult{URL: job.UpstreamURL, PosterURL: videoPosterURL(job), ContentType: job.ContentType})
		lease.Release()
		if archiveErr != nil {
			result = firstError(result, fmt.Errorf("任务 %s 保存视频: %w", job.ID, archiveErr))
			continue
		}
		job.UpstreamURL, job.ContentType, job.MetadataJSON, job.UpdatedAt = archived.URL, archived.ContentType, withVideoPosterURL(encodeVideoJobMetadata(videoMetadataForJob(job)), archived.PosterURL), time.Now().UTC()
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
	metadata := videoMetadataForJob(job)
	lease, err := s.selector.AcquirePinned(ctx, route.Provider, job.AccountID, route.ID, route.UpstreamModel, "", true)
	if err != nil {
		if s.videoCancelled(job.ID) {
			return
		}
		if parent.Err() != nil {
			s.deferVideoJob(parent, job)
			return
		}
		fallback, fallbackErr := s.acquireVideoAccountFallback(parent, &job, route, "", &metadata)
		if fallbackErr == nil {
			if s.logger != nil {
				s.logger.Warn("video_account_switched", "job_id", job.ID, "account_id", job.AccountID, "account_retry", metadata.AccountRetryCount, "extension", metadata.IsExtension, "error", err)
			}
			fallback.Release()
			s.runVideoJob(parent, job, route)
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
	result, err := adapter.GenerateVideo(ctx, provider.VideoRequest{
		Credential: lease.Credential, Billing: lease.Billing, JobID: job.ID, Prompt: job.Prompt, Preset: metadata.Preset,
		Duration: job.Seconds, AspectRatio: job.Size, Resolution: job.Quality, ReferenceURLs: metadata.ImageURLs,
		IsExtension: metadata.IsExtension, ExtendPostID: metadata.ExtendPostID, ExtensionStartTime: metadata.ExtensionStartTime,
		OriginalPostID: metadata.OriginalPostID, FileAttachmentID: metadata.FileAttachmentID, StitchWithExtend: metadata.StitchWithExtend,
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
	if err == nil && result.AssetID == "" && protectedVideoOutput(result.URL) {
		result, err = s.persistRemoteVideo(ctx, job.ID, adapter, lease.Credential, result)
	}
	if err != nil {
		if s.videoCancelled(job.ID) {
			return
		}
		if parent.Err() != nil {
			s.deferVideoJob(parent, job)
			return
		}
		if status, ok := provider.ErrorHTTPStatus(err); ok {
			retrySafe := provider.IsMediaJobRetrySafe(err)
			if status == http.StatusForbidden {
				retrySafe = retrySafe && s.providers.RetryForbiddenAsEgress(lease.Credential.Provider)
			}
			if delay, retry := videoRetryPlan(status, metadata.RetryCount, retrySafe); retry {
				metadata.RetryCount++
				job.MetadataJSON = encodeVideoJobMetadata(metadata)
				applyMediaJobEgress(&job, egressTrace, route.Provider)
				s.deferVideoJobFor(parent, job, delay)
				if s.logger != nil {
					s.logger.Warn("video_job_retry_scheduled", "job_id", job.ID, "retry", metadata.RetryCount, "delay", delay, "error", err)
				}
				return
			}
		}
		failureCtx, failureCancel := context.WithTimeout(context.Background(), finalizationTimeout)
		failureHandled := false
		if provider.IsAccountHealthNeutral(err) {
			failureHandled = true
		} else if errors.Is(err, provider.ErrUnauthorized) {
			if lease.Credential.AuthType == account.AuthTypeSSO {
				s.markSSOCredentialRejected(failureCtx, lease.Credential, fmt.Sprintf("%s SSO credential rejected", lease.Credential.Provider))
			}
			failureHandled = true
		} else if status, ok := provider.ErrorHTTPStatus(err); ok {
			switch {
			case status == http.StatusUnauthorized && lease.Credential.AuthType == account.AuthTypeSSO:
				s.markSSOCredentialRejected(failureCtx, lease.Credential, fmt.Sprintf("%s SSO credential rejected", lease.Credential.Provider))
				failureHandled = true
			case status == http.StatusForbidden && s.providers.RetryForbiddenAsEgress(lease.Credential.Provider):
				// Web Provider 已对 anti-bot 403 降低出口健康并重建浏览器会话；
				// 视频请求已提交，不能换号重试，也不能误伤账号池。
				// 符合资格的 Build 主地址 403 由 Adapter 尝试 XAI，不在此禁用账号。
				failureHandled = true
			case status == http.StatusForbidden && lease.Credential.Provider == account.ProviderBuild:
				if !account.IsBuildSuper(lease.Credential, lease.Billing) {
					// 非 Super 的 403 按账号级故障处理；auto 模式不会因此回退 XAI。
					s.selector.MarkFailure(failureCtx, lease.Credential, status, 0)
				}
				// Super（Billing paid 或 entitlement）的 403 保持服务级处理。
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
		s.logVideoGenerationFailure(job, lease.Credential, err)
		if shouldSwitchVideoAccount(err) {
			lease.Release()
			fallback, fallbackErr := s.acquireVideoAccountFallback(parent, &job, route, lease.QuotaMode, &metadata)
			if fallbackErr == nil {
				if s.logger != nil {
					s.logger.Warn("video_account_switched", "job_id", job.ID, "account_id", job.AccountID, "account_retry", metadata.AccountRetryCount, "extension", metadata.IsExtension, "error", err)
				}
				fallback.Release()
				s.runVideoJob(parent, job, route)
				return
			}
			if s.logger != nil {
				s.logger.Warn("video_account_switch_failed", "job_id", job.ID, "extension", metadata.IsExtension, "error", fallbackErr)
			}
		}
		failureCode, publicErr := "generation_failed", err
		if status, ok := provider.ErrorHTTPStatus(err); errors.Is(err, provider.ErrUnauthorized) || (ok && (status == http.StatusUnauthorized || status == http.StatusForbidden)) {
			failureCode, publicErr = "provider_unavailable", errors.New("上游服务暂不可用")
		}
		s.failVideoJob(parent, job, failureCode, publicErr)
		return
	}
	if s.videoCancelled(job.ID) {
		return
	}
	now := time.Now().UTC()
	job.Status, job.Progress, job.UpstreamURL, job.ContentType = media.StatusCompleted, 100, result.URL, result.ContentType
	// 成功终态必须清空历史错误字段，避免管理端/恢复路径把中间失败文案当成最终结果。
	job.ErrorCode, job.ErrorMessage = "", ""
	if result.AssetID != "" {
		job.ResultAssetID = result.AssetID
	}
	job.MetadataJSON = withVideoPosterURL(encodeVideoJobMetadata(videoMetadataForJob(job)), result.PosterURL)
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

// persistRemoteVideo 只重试已经生成的视频结果下载与本地归档，不重新调用生成接口，
// 且所有尝试固定使用创建任务的同一凭据。
func (s *Service) persistRemoteVideo(ctx context.Context, jobID string, adapter provider.VideoAdapter, credential account.Credential, result provider.VideoResult) (provider.VideoResult, error) {
	if s.mediaAssets == nil {
		return result, provider.NewMediaPostProcessingError(provider.MediaPostProcessingStorage, errors.New("视频媒体存储未配置"))
	}
	downloader, ok := adapter.(provider.VideoContentDownloader)
	if !ok {
		return result, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, errors.New("Provider 不支持视频内容下载"))
	}
	var lastErr error
	for attempt := 0; attempt < videoOutputAttempts; attempt++ {
		body, contentType, _, downloadErr := downloader.DownloadVideo(ctx, credential, result.URL)
		if downloadErr != nil {
			lastErr = provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, downloadErr)
		} else {
			asset, saveErr := s.mediaAssets.SaveVideo(ctx, jobID, contentType, body)
			_ = body.Close()
			if saveErr == nil {
				result.AssetID = asset.ID
				result.ContentType = asset.MIMEType
				return result, nil
			}
			lastErr = provider.NewMediaPostProcessingError(provider.MediaPostProcessingStorage, saveErr)
		}
		if ctx.Err() != nil || attempt+1 >= videoOutputAttempts {
			break
		}
		if waitErr := waitVideoOutputRetry(ctx, attempt); waitErr != nil {
			return result, waitErr
		}
	}
	return result, lastErr
}

func waitVideoOutputRetry(ctx context.Context, attempt int) error {
	delays := [...]time.Duration{200 * time.Millisecond, 750 * time.Millisecond}
	timer := time.NewTimer(delays[min(attempt, len(delays)-1)])
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func withVideoPosterURL(raw, posterURL string) string {
	posterURL = strings.TrimSpace(posterURL)
	if posterURL == "" {
		return raw
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		metadata = make(map[string]any)
	}
	metadata["poster_url"] = posterURL
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func videoPosterURL(job media.Job) string {
	var metadata struct {
		PosterURL string `json:"poster_url"`
	}
	if json.Unmarshal([]byte(videoMetadataPayload(job)), &metadata) != nil {
		return ""
	}
	return strings.TrimSpace(metadata.PosterURL)
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
	var accountID *uint64
	if job.AccountID > 0 {
		value := job.AccountID
		accountID = &value
	}
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
		AccountID: accountID, AccountName: job.AccountName, StatusCode: statusCode, ErrorCode: job.ErrorCode,
		EgressNodeID: job.EgressNodeID, EgressNodeName: job.EgressNodeName, EgressScope: job.EgressScope, EgressMode: audit.EgressMode(job.EgressMode),
		MediaInputImages: int64(job.InputImageCount),
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
	ImageURLs           []string `json:"image_urls"`
	Preset              string   `json:"preset,omitempty"`
	DisplayName         string   `json:"display_name,omitempty"`
	IsExtension         bool     `json:"is_extension,omitempty"`
	SourceTaskID        string   `json:"source_task_id,omitempty"`
	ExtendPostID        string   `json:"extend_post_id,omitempty"`
	ExtensionStartTime  float64  `json:"extension_start_time,omitempty"`
	OriginalPostID      string   `json:"original_post_id,omitempty"`
	FileAttachmentID    string   `json:"file_attachment_id,omitempty"`
	StitchWithExtend    bool     `json:"stitch_with_extend,omitempty"`
	RetryCount          int      `json:"retry_count,omitempty"`
	AccountRetryCount   int      `json:"account_retry_count,omitempty"`
	AttemptedAccountIDs []uint64 `json:"attempted_account_ids,omitempty"`
}

func videoMetadataFromInput(input VideoInput) videoInputMetadata {
	return videoInputMetadata{ImageURLs: input.ReferenceURLs, Preset: input.Preset, IsExtension: input.IsExtension, SourceTaskID: input.SourceTaskID, ExtendPostID: input.ExtendPostID, ExtensionStartTime: input.ExtensionStartTime, OriginalPostID: input.OriginalPostID, FileAttachmentID: input.FileAttachmentID, StitchWithExtend: input.StitchWithExtend}
}

func (m *videoInputMetadata) markAccountAttempt(accountID uint64) {
	if accountID == 0 {
		return
	}
	for _, attempted := range m.AttemptedAccountIDs {
		if attempted == accountID {
			return
		}
	}
	if len(m.AttemptedAccountIDs) > 0 {
		m.AccountRetryCount++
	}
	m.AttemptedAccountIDs = append(m.AttemptedAccountIDs, accountID)
}

func (m videoInputMetadata) excludedAccounts() map[uint64]bool {
	excluded := make(map[uint64]bool, len(m.AttemptedAccountIDs))
	for _, accountID := range m.AttemptedAccountIDs {
		if accountID != 0 {
			excluded[accountID] = true
		}
	}
	return excluded
}

func (m videoInputMetadata) canTryAnotherAccount(limit int) bool {
	return limit > 0 && len(m.excludedAccounts()) < limit
}

func (s *Service) videoAccountAttemptLimit() int {
	limit := int(s.maxAttempts.Load())
	if limit <= 0 {
		return 3
	}
	return limit
}

func (s *Service) acquireVideoAccountFallback(ctx context.Context, job *media.Job, route model.Route, quotaMode string, metadata *videoInputMetadata) (*accountLease, error) {
	if job == nil || metadata == nil {
		return nil, &SelectionUnavailableError{Reason: SelectionNoAccounts}
	}
	metadata.markAccountAttempt(job.AccountID)
	if !metadata.canTryAnotherAccount(s.videoAccountAttemptLimit()) {
		return nil, &SelectionUnavailableError{Reason: SelectionNoAccounts}
	}
	lease, err := s.selector.Acquire(ctx, route.Provider, route.ID, route.UpstreamModel, quotaMode, "", metadata.excludedAccounts(), false)
	if err != nil {
		return nil, err
	}
	metadata.RetryCount = 0
	metadata.markAccountAttempt(lease.Credential.ID)
	job.AccountID = lease.Credential.ID
	job.AccountName = lease.Credential.Name
	job.MetadataJSON = encodeVideoJobMetadata(*metadata)
	job.UpdatedAt = time.Now().UTC()
	if err := s.mediaJobs.UpdateMediaJob(ctx, *job); err != nil {
		lease.Release()
		return nil, err
	}
	return lease, nil
}

func shouldSwitchVideoAccount(err error) bool {
	if err == nil || provider.IsMediaPostProcessingError(err) {
		return false
	}
	if errors.Is(err, provider.ErrUnauthorized) {
		return true
	}
	status, ok := provider.ErrorHTTPStatus(err)
	if !ok {
		return false
	}
	return status == http.StatusUnauthorized || status == http.StatusPaymentRequired || status == http.StatusForbidden || status == http.StatusTooManyRequests
}

func encodeVideoMetadata(metadata videoInputMetadata) string {
	data, _ := json.Marshal(metadata)
	return string(data)
}

func encodeVideoJobMetadata(metadata videoInputMetadata) string {
	metadata.ImageURLs = nil
	return encodeVideoMetadata(metadata)
}

func encodeVideoInput(referenceURLs []string) (string, error) {
	return encodeVideoMetadataChecked(videoInputMetadata{ImageURLs: referenceURLs})
}

func encodeVideoMetadataChecked(metadata videoInputMetadata) (string, error) {
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("编码视频输入: %w", err)
	}
	if len(data) > media.MaxInputJSONBytes {
		return "", ErrVideoInputTooLarge
	}
	return string(data), nil
}

func decodeVideoInput(value string) []string {
	return decodeVideoMetadata(value).ImageURLs
}

func videoMetadataPayload(job media.Job) string {
	if value := strings.TrimSpace(job.MetadataJSON); value != "" && value != "{}" {
		return value
	}
	return job.InputJSON
}

func videoMetadataForJob(job media.Job) videoInputMetadata {
	metadata := decodeVideoMetadata(videoMetadataPayload(job))
	metadata.ImageURLs = decodeVideoInput(job.InputJSON)
	return metadata
}

func decodeVideoMetadata(value string) videoInputMetadata {
	var input videoInputMetadata
	_ = json.Unmarshal([]byte(value), &input)
	return input
}

func (s *Service) failVideoJob(ctx context.Context, job media.Job, code string, err error) {
	now := time.Now().UTC()
	if s.logger != nil {
		s.logger.Warn("video_job_failed", "job_id", job.ID, "code", code, "error", err)
	}
	job.Status, job.ErrorCode, job.ErrorMessage = media.StatusFailed, code, publicVideoFailureMessage(err)
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

func (s *Service) logVideoGenerationFailure(job media.Job, credential account.Credential, err error) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	attributes := []any{
		"job_id", job.ID,
		"request_id", job.RequestID,
		"account_id", credential.ID,
		"provider", credential.Provider,
		"model", job.UpstreamModel,
		"egress_scope", job.EgressScope,
		"egress_mode", job.EgressMode,
		"error", sanitizeDiagnosticText(err.Error(), 512),
	}
	if status, ok := provider.ErrorHTTPStatus(err); ok {
		attributes = append(attributes, "upstream_status", status)
	}
	if job.EgressNodeID != nil {
		attributes = append(attributes, "egress_node_id", *job.EgressNodeID, "egress_node_name", job.EgressNodeName)
	}
	logger.Warn("video_generation_failed", attributes...)
}

func (s *Service) deferVideoJob(ctx context.Context, job media.Job) {
	s.deferVideoJobFor(ctx, job, 5*time.Minute)
}

func (s *Service) deferVideoJobFor(ctx context.Context, job media.Job, delay time.Duration) {
	now := time.Now().UTC()
	leaseUntil := now.Add(delay)
	job.Status = media.StatusInProgress
	job.LeaseUntil = &leaseUntil
	job.UpdatedAt = now
	job.ErrorCode = ""
	job.ErrorMessage = ""
	if err := s.persistVideoJobWithRetry(ctx, job); err != nil {
		s.logger.Error("video_job_defer_write_failed", "job_id", job.ID, "error", err)
	}
}

func videoRetryPlan(status, retryCount int, retrySafe bool) (time.Duration, bool) {
	if !retrySafe {
		return 0, false
	}
	var delays []time.Duration
	switch status {
	case http.StatusForbidden:
		delays = []time.Duration{30 * time.Second, 2 * time.Minute, 5 * time.Minute}
	case http.StatusBadGateway:
		delays = []time.Duration{15 * time.Second, time.Minute, 3 * time.Minute}
	default:
		return 0, false
	}
	if retryCount < 0 || retryCount >= len(delays) {
		return 0, false
	}
	return delays[retryCount], true
}

func publicVideoFailureMessage(err error) string {
	if err == nil {
		return "视频生成失败，请稍后重试"
	}
	if errors.Is(err, provider.ErrUnauthorized) {
		return "上游认证失败，请检查账号状态"
	}
	if provider.IsAccountHealthNeutral(err) {
		return "内容未通过上游审核，请调整提示词或素材后重试"
	}
	if status, ok := provider.ErrorHTTPStatus(err); ok {
		switch status {
		case http.StatusForbidden:
			return "上游安全验证暂时未通过，请稍后重试"
		case http.StatusPaymentRequired, http.StatusTooManyRequests:
			return "当前账号额度不足或请求过于频繁，请稍后重试"
		case http.StatusNotFound:
			return "上游未找到源视频，无法继续处理"
		default:
			if status >= http.StatusInternalServerError {
				return "上游服务暂时不可用，请稍后重试"
			}
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "<!doctype html") || strings.Contains(message, "<html") || strings.Contains(message, "just a moment") || strings.Contains(message, "challenge-platform") {
		return "上游安全验证暂时未通过，请稍后重试"
	}
	return "视频生成失败，请稍后重试"
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
