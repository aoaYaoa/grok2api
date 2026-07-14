package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type VideoGateway interface {
	CreateVideo(context.Context, gateway.VideoInput) (mediadomain.Job, error)
	GetVideo(context.Context, string, clientkeydomain.Key) (mediadomain.Job, error)
}

type videoTask struct {
	id    string
	jobID string
}

type videoStartRequest struct {
	Prompt           string          `json:"prompt"`
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
	ExtendPostID     string          `json:"extend_post_id"`
}

type videoStopRequest struct {
	TaskIDs []string `json:"task_ids"`
}

func (h *Handler) registerVideo(public *gin.RouterGroup) {
	public.POST("/video/start", h.videoStart)
	public.GET("/video/sse", h.videoSSE)
	public.POST("/video/stop", h.videoStop)
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
	if request.IsVideoExtension {
		c.JSON(http.StatusNotImplemented, gin.H{"detail": "Native Go video extension is not implemented yet"})
		return
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
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
	if request.Prompt == "" && len(references) == 0 {
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
	taskIDs := make([]string, 0, request.Concurrent)
	for index := 0; index < request.Concurrent; index++ {
		taskID, err := newTaskID()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to create task"})
			return
		}
		job, err := h.videoGateway.CreateVideo(c.Request.Context(), gateway.VideoInput{
			RequestID: taskID, ClientKey: clientKey, PublicModel: "grok-imagine-video",
			Prompt: request.Prompt, Duration: request.VideoLength, AspectRatio: request.AspectRatio,
			Resolution: request.Resolution, ReferenceURLs: references,
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"detail": err.Error()})
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
	for _, taskID := range request.TaskIDs {
		if h.dropVideoTask(strings.TrimSpace(taskID)) {
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
