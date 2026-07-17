package legacy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

var videoPostIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

type VideoGateway interface {
	CreateVideo(context.Context, gateway.VideoInput) (mediadomain.Job, error)
	GetVideo(context.Context, string, clientkeydomain.Key) (mediadomain.Job, error)
	CancelVideo(context.Context, string, clientkeydomain.Key) (mediadomain.Job, error)
	ListVideos(context.Context, clientkeydomain.Key, int, int) ([]mediadomain.Job, int64, error)
	RenameVideo(context.Context, string, string, clientkeydomain.Key) (mediadomain.Job, error)
}

type LegacyCachedVideo struct {
	Name           string
	TaskID         string
	ViewURL        string
	PosterURL      string
	PostID         string
	ShareLink      string
	OriginalPostID string
	DisplayName    string
	SizeBytes      int64
	ModifiedAtMS   int64
}

type LegacyVideoCache interface {
	ListVideos() ([]LegacyCachedVideo, error)
	RenameVideo(identifier, displayName string) (LegacyCachedVideo, error)
}

type VideoReferenceStore interface {
	SaveImage(context.Context, []byte) (mediadomain.Asset, error)
}

type videoTask struct {
	id    string
	jobID string
}

type videoStartRequest struct {
	Prompt           string          `json:"prompt"`
	Preset           string          `json:"preset"`
	AspectRatio      string          `json:"aspect_ratio"`
	VideoLength      int             `json:"video_length"`
	Resolution       string          `json:"resolution_name"`
	Concurrent       int             `json:"concurrent"`
	ImageURL         string          `json:"image_url"`
	SourceImageURL   string          `json:"source_image_url"`
	ImageReferences  []string        `json:"image_references"`
	SourceImageURLs  []string        `json:"source_image_urls"`
	ReferenceItems   []referenceItem `json:"reference_items"`
	IsVideoExtension bool            `json:"is_video_extension"`
	SourceTaskID     string          `json:"source_task_id"`
	ExtendPostID     string          `json:"extend_post_id"`
	ExtensionStart   float64         `json:"video_extension_start_time"`
	OriginalPostID   string          `json:"original_post_id"`
	FileAttachmentID string          `json:"file_attachment_id"`
	StitchWithExtend *bool           `json:"stitch_with_extend"`
}

type videoStopRequest struct {
	TaskIDs []string `json:"task_ids"`
}

func (h *Handler) registerVideo(public *gin.RouterGroup) {
	public.POST("/video/start", h.videoStart)
	public.GET("/video/sse", h.videoSSE)
	public.POST("/video/stop", h.videoStop)
	public.GET("/video/cache/list", h.videoCacheList)
	public.POST("/video/rename", h.videoRename)
}

func (h *Handler) videoCacheList(c *gin.Context) {
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	pageValue, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	values, err := h.allCachedVideos(c.Request.Context(), clientKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to list videos"})
		return
	}
	pageValue = max(pageValue, 1)
	pageSize = min(max(pageSize, 1), 200)
	total := len(values)
	start, end := legacyPageRange(pageValue, pageSize, total)
	items := make([]gin.H, 0, end-start)
	for _, item := range values[start:end] {
		items = append(items, gin.H{
			"name": item.Name, "task_id": item.TaskID, "view_url": item.ViewURL, "poster_url": item.PosterURL, "post_id": item.PostID, "share_link": item.ShareLink,
			"original_post_id": item.OriginalPostID, "display_name": item.DisplayName,
			"size_bytes": item.SizeBytes, "mtime_ms": item.ModifiedAtMS,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": pageValue, "page_size": pageSize, "total": total})
}

func (h *Handler) allCachedVideos(ctx context.Context, clientKey clientkeydomain.Key) ([]LegacyCachedVideo, error) {
	items := make([]LegacyCachedVideo, 0)
	configuredSources := 0
	succeededSources := 0
	var sourceErrors []error
	if h.videoCache != nil {
		configuredSources++
		localItems, err := h.videoCache.ListVideos()
		if err != nil {
			sourceErrors = append(sourceErrors, err)
		} else {
			succeededSources++
			items = append(items, localItems...)
		}
	}
	if h.videoGateway != nil {
		configuredSources++
		databaseSucceeded := false
		for pageValue := 1; ; pageValue++ {
			values, total, err := h.videoGateway.ListVideos(ctx, clientKey, pageValue, 200)
			if err != nil {
				sourceErrors = append(sourceErrors, err)
				break
			}
			databaseSucceeded = true
			for _, job := range values {
				if item, ok := cachedVideoFromJob(job); ok {
					items = append(items, item)
				}
			}
			if len(values) == 0 || pageValue*200 >= int(total) {
				break
			}
		}
		if databaseSucceeded {
			succeededSources++
		}
	}
	if configuredSources > 0 && succeededSources == 0 {
		return nil, errors.Join(sourceErrors...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ModifiedAtMS > items[j].ModifiedAtMS })
	return dedupeCachedVideos(items), nil
}

func cachedVideoFromJob(job mediadomain.Job) (LegacyCachedVideo, bool) {
	if job.Status != mediadomain.StatusCompleted || strings.TrimSpace(job.UpstreamURL) == "" {
		return LegacyCachedVideo{}, false
	}
	displayName := strings.TrimSpace(job.DisplayName)
	if displayName == "" {
		var metadata struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.Unmarshal([]byte(job.InputJSON), &metadata)
		displayName = strings.TrimSpace(metadata.DisplayName)
	}
	name := job.ID + ".mp4"
	if parsed, parseErr := url.Parse(job.UpstreamURL); parseErr == nil {
		if base := path.Base(parsed.Path); base != "." && base != "/" && base != "" {
			name = base
		}
	}
	return LegacyCachedVideo{
		Name: name, TaskID: job.RequestID, ViewURL: job.UpstreamURL, PostID: lastVideoPostID(job.UpstreamURL),
		PosterURL: videoJobPosterURL(job), DisplayName: displayName, ModifiedAtMS: job.UpdatedAt.UnixMilli(),
	}, true
}

func videoJobPosterURL(job mediadomain.Job) string {
	var metadata struct {
		PosterURL string `json:"poster_url"`
	}
	_ = json.Unmarshal([]byte(job.InputJSON), &metadata)
	return strings.TrimSpace(metadata.PosterURL)
}

func dedupeCachedVideos(items []LegacyCachedVideo) []LegacyCachedVideo {
	result := make([]LegacyCachedVideo, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.ViewURL)
		if key == "" {
			key = strings.TrimSpace(item.Name)
		}
		if index, exists := seen[key]; exists {
			if result[index].PosterURL == "" && item.PosterURL != "" {
				result[index].PosterURL = item.PosterURL
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, item)
	}
	return result
}

func (h *Handler) videoRename(c *gin.Context) {
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	var request struct {
		PostID      string `json:"post_id"`
		ShareLink   string `json:"share_link"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	identifier := strings.TrimSpace(request.PostID)
	if identifier == "" {
		identifier = strings.TrimSpace(request.ShareLink)
	}
	if identifier == "" {
		identifier = strings.TrimSpace(request.Name)
	}
	if h.videoGateway != nil {
		job, err := h.videoGateway.RenameVideo(c.Request.Context(), identifier, request.DisplayName, clientKey)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"status": "success", "result": gin.H{
				"task_id": job.ID, "post_id": videoPostIDPattern.FindString(job.UpstreamURL),
				"display_name": job.DisplayName, "view_url": job.UpstreamURL,
			}})
			return
		}
		if !errors.Is(err, gateway.ErrResponseNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to rename video"})
			return
		}
	}
	if h.videoCache != nil {
		item, err := h.videoCache.RenameVideo(identifier, request.DisplayName)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"status": "success", "result": gin.H{
				"post_id": item.PostID, "display_name": item.DisplayName, "view_url": item.ViewURL,
			}})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"detail": "Video not found"})
}

func legacyPageRange(page, pageSize, total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	maxPage := (total-1)/pageSize + 1
	if page > maxPage {
		return total, total
	}
	start := (page - 1) * pageSize
	return start, min(start+pageSize, total)
}

func (h *Handler) videoStart(c *gin.Context) {
	if h.videoGateway == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Video generator is not configured"})
		return
	}
	var request videoStartRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.Preset = strings.ToLower(strings.TrimSpace(request.Preset))
	if request.Preset == "" {
		request.Preset = "normal"
	}
	switch request.Preset {
	case "normal", "fun", "spicy", "custom":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"detail": "preset must be normal, fun, spicy, or custom"})
		return
	}
	request.ExtendPostID = strings.TrimSpace(request.ExtendPostID)
	request.OriginalPostID = strings.TrimSpace(request.OriginalPostID)
	request.FileAttachmentID = strings.TrimSpace(request.FileAttachmentID)
	if request.IsVideoExtension && !videoPostIDPattern.MatchString(request.ExtendPostID) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "extend_post_id is required for video extension"})
		return
	}
	if request.IsVideoExtension && request.ExtensionStart <= 0 {
		request.ExtensionStart = 6
	}
	request.AspectRatio = normalizeVideoRatio(request.AspectRatio)
	if request.VideoLength == 0 {
		request.VideoLength = 6
	}
	if request.VideoLength != 6 && request.VideoLength != 10 && request.VideoLength != 15 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "video_length must be 6, 10, or 15 seconds"})
		return
	}
	request.Resolution = strings.ToLower(strings.TrimSpace(request.Resolution))
	if request.Resolution == "" {
		request.Resolution = "480p"
	}
	if request.Resolution != "480p" && request.Resolution != "720p" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "resolution_name must be 480p or 720p"})
		return
	}
	if request.Concurrent == 0 {
		request.Concurrent = 1
	}
	if request.Concurrent < 1 || request.Concurrent > 4 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "concurrent must be between 1 and 4"})
		return
	}
	references := collectVideoReferences(request)
	if request.Prompt == "" && len(references) == 0 && !request.IsVideoExtension {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Prompt cannot be empty when no image reference is provided"})
		return
	}
	if len(references) > 8 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "At most 8 image references are supported"})
		return
	}
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	references, err := h.storeInlineVideoReferences(c.Request.Context(), references)
	if err != nil {
		slog.Error("legacy_video_reference_store_failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"detail": "参考图处理失败，请重新选择图片"})
		return
	}
	taskIDs := make([]string, 0, request.Concurrent)
	for index := 0; index < request.Concurrent; index++ {
		taskID, err := newTaskID()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to create task"})
			return
		}
		job, err := h.videoGateway.CreateVideo(c.Request.Context(), gateway.VideoInput{
			RequestID: taskID, ClientKey: clientKey, PublicModel: "grok-imagine-video",
			Prompt: request.Prompt, Preset: request.Preset, Duration: request.VideoLength, AspectRatio: request.AspectRatio,
			Resolution: request.Resolution, ReferenceURLs: references,
			IsExtension: request.IsVideoExtension, ExtendPostID: request.ExtendPostID,
			SourceTaskID:       request.SourceTaskID,
			ExtensionStartTime: request.ExtensionStart, OriginalPostID: request.OriginalPostID,
			FileAttachmentID: request.FileAttachmentID, StitchWithExtend: request.StitchWithExtend == nil || *request.StitchWithExtend,
		})
		if err != nil {
			slog.Error("legacy_video_create_failed", "request_id", taskID, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"detail": publicVideoStartError(err)})
			return
		}
		h.videoMu.Lock()
		h.videoTasks[taskID] = &videoTask{id: taskID, jobID: job.ID}
		h.videoMu.Unlock()
		taskIDs = append(taskIDs, taskID)
	}
	c.JSON(http.StatusOK, gin.H{
		"task_id": taskIDs[0], "task_ids": taskIDs, "concurrent": len(taskIDs),
		"aspect_ratio": request.AspectRatio, "reference_count": len(references),
	})
}

func (h *Handler) storeInlineVideoReferences(ctx context.Context, references []string) ([]string, error) {
	if h.videoReferenceStore == nil {
		return references, nil
	}
	stored := append([]string(nil), references...)
	for index, reference := range stored {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(reference)), "data:image/") {
			continue
		}
		data, err := decodeInlineVideoReference(reference, 20<<20)
		if err != nil {
			return nil, err
		}
		asset, err := h.videoReferenceStore.SaveImage(ctx, data)
		if err != nil {
			return nil, err
		}
		referenceURL := mediadomain.ImageReference(asset.ID)
		if referenceURL == "" {
			return nil, fmt.Errorf("参考图存储标识无效")
		}
		stored[index] = referenceURL
	}
	return stored, nil
}

func decodeInlineVideoReference(value string, maxBytes int64) ([]byte, error) {
	header, encoded, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok || !strings.HasPrefix(strings.ToLower(header), "data:image/") || !strings.Contains(strings.ToLower(header), ";base64") {
		return nil, fmt.Errorf("参考图 data URI 无效")
	}
	encoded = strings.Join(strings.Fields(encoded), "")
	if encoded == "" || int64(base64.StdEncoding.DecodedLen(len(encoded))) > maxBytes {
		return nil, fmt.Errorf("参考图为空或超过 20 MB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(data) == 0 || int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("参考图 Base64 无效")
	}
	return data, nil
}

func publicVideoStartError(err error) string {
	if errors.Is(err, gateway.ErrModelNotFound) {
		return "视频模型暂时不可用"
	}
	if errors.Is(err, gateway.ErrNoAvailableAccount) {
		return "当前没有可用的视频账号，请稍后重试"
	}
	message := strings.TrimSpace(err.Error())
	if strings.Contains(message, "必须提供 prompt") || strings.Contains(message, "图片生视频") {
		return message
	}
	return "创建视频任务失败，请稍后重试"
}

func lastVideoPostID(value string) string {
	values := videoPostIDPattern.FindAllString(value, -1)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func (h *Handler) videoSSE(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("task_id"))
	task := h.getVideoTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Task not found"})
		return
	}
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	defer h.dropVideoTask(taskID)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	for {
		if c.Request.Context().Err() != nil || h.getVideoTask(taskID) == nil {
			writeLegacyDone(c)
			return
		}
		job, err := h.videoGateway.GetVideo(c.Request.Context(), task.jobID, clientKey)
		if err != nil {
			writeLegacySSE(c, gin.H{"error": err.Error(), "code": "video_status_failed"})
			writeLegacyDone(c)
			return
		}
		switch job.Status {
		case mediadomain.StatusCompleted:
			if posterURL := videoJobPosterURL(job); posterURL != "" {
				writeLegacySSE(c, gin.H{"poster_url": posterURL})
			}
			writeVideoDelta(c, "[video]("+job.UpstreamURL+")", "")
			writeVideoDelta(c, "", "stop")
			writeLegacyDone(c)
			return
		case mediadomain.StatusFailed:
			message := strings.TrimSpace(job.ErrorMessage)
			if message == "" {
				message = "Video generation failed"
			}
			writeLegacySSE(c, gin.H{"error": message, "code": job.ErrorCode})
			writeLegacyDone(c)
			return
		default:
			writeVideoDelta(c, fmt.Sprintf("当前进度 %d%%", max(0, min(99, job.Progress))), "")
		}
		timer := time.NewTimer(h.options.VideoPollInterval)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *Handler) videoStop(c *gin.Context) {
	var request videoStopRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	removed := 0
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	for _, taskID := range request.TaskIDs {
		taskID = strings.TrimSpace(taskID)
		task := h.getVideoTask(taskID)
		if task == nil {
			continue
		}
		_, _ = h.videoGateway.CancelVideo(c.Request.Context(), task.jobID, clientKey)
		if h.dropVideoTask(taskID) {
			removed++
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "removed": removed})
}

func collectVideoReferences(request videoStartRequest) []string {
	values := make([]string, 0, 8)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	for _, value := range request.ImageReferences {
		add(value)
	}
	for _, value := range request.SourceImageURLs {
		add(value)
	}
	for _, item := range request.ReferenceItems {
		if strings.TrimSpace(item.SourceImageURL) != "" {
			add(item.SourceImageURL)
		} else {
			add(item.ImageURL)
		}
	}
	add(request.SourceImageURL)
	add(request.ImageURL)
	return values
}

func normalizeVideoRatio(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "1:1", "16:9", "9:16", "3:2", "2:3":
		return value
	default:
		return "3:2"
	}
}

func writeVideoDelta(c *gin.Context, content, finishReason string) {
	delta := gin.H{}
	if content != "" {
		delta["content"] = content
	}
	choice := gin.H{"index": 0, "delta": delta, "finish_reason": nil}
	if finishReason != "" {
		choice["finish_reason"] = finishReason
	}
	writeLegacySSE(c, gin.H{
		"id": "chatcmpl-legacy-video", "object": "chat.completion.chunk",
		"choices": []gin.H{choice},
	})
}

func writeLegacyDone(c *gin.Context) {
	_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

func (h *Handler) getVideoTask(taskID string) *videoTask {
	h.videoMu.Lock()
	defer h.videoMu.Unlock()
	return h.videoTasks[taskID]
}

func (h *Handler) dropVideoTask(taskID string) bool {
	if taskID == "" {
		return false
	}
	h.videoMu.Lock()
	_, ok := h.videoTasks[taskID]
	if ok {
		delete(h.videoTasks, taskID)
	}
	h.videoMu.Unlock()
	return ok
}
