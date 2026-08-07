package legacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	maxLegacyImageResponseBytes = 64 << 20
	legacyWebImageLiteModel     = "Web/grok-imagine-image-lite"
	legacyWebImageQualityModel  = "Web/grok-imagine-image-quality-lite"
)

func legacyWebImageModel(pro bool) string {
	if pro {
		return legacyWebImageQualityModel
	}
	return legacyWebImageLiteModel
}

var parentPostIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{32,36}$`)

type LegacyCachedImage struct {
	Source       string `json:"source"`
	CacheKey     string `json:"cache_key"`
	Name         string
	ViewURL      string
	SizeBytes    int64
	ModifiedAtMS int64
}

type LegacyImageCache interface {
	ListImages(context.Context) ([]LegacyCachedImage, error)
}

type LegacyImageCacheDeleter interface {
	DeleteImages(context.Context, []CacheDeleteTarget) (CacheDeleteResult, error)
}

type imageTask struct {
	id          string
	prompt      string
	aspectRatio string
	nsfw        bool
	pro         bool
	ctx         context.Context
	cancel      context.CancelFunc
}

type imagineStartRequest struct {
	Prompt      string `json:"prompt"`
	AspectRatio string `json:"aspect_ratio"`
	NSFW        *bool  `json:"nsfw"`
	Pro         bool   `json:"pro"`
}

type imagineStopRequest struct {
	TaskIDs []string `json:"task_ids"`
}

type imagineEditRequest struct {
	Prompt          string          `json:"prompt"`
	ParentPostID    string          `json:"parent_post_id"`
	SourceImageURL  string          `json:"source_image_url"`
	ImageBase64     string          `json:"image_base64"`
	ImageURL        string          `json:"image_url"`
	ImageReferences []string        `json:"image_references"`
	ReferenceItems  []referenceItem `json:"reference_items"`
	Stream          bool            `json:"stream"`
}

type referenceItem struct {
	ImageURL       string `json:"image_url"`
	SourceImageURL string `json:"source_image_url"`
	ParentPostID   string `json:"parent_post_id"`
}

type imageAPIResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		Base64   string `json:"b64_json"`
		URL      string `json:"url"`
		MIMEType string `json:"mime_type"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (h *Handler) registerImagine(public *gin.RouterGroup) {
	public.POST("/imagine/start", h.imagineStart)
	public.POST("/imagine/stop", h.imagineStop)
	public.GET("/imagine/sse", h.imagineSSE)
	public.GET("/imagine/ws", h.imagineWS)
	public.POST("/imagine/edit", h.imagineEdit)
	public.POST("/imagine/workbench/edit", h.imagineWorkbenchEdit)
	public.GET("/imagine/cache/list", h.imagineCacheList)
	public.POST("/imagine/cache/delete", h.imagineCacheDelete)
}

func (h *Handler) imagineCacheDelete(c *gin.Context) {
	var request struct {
		Items []CacheDeleteTarget `json:"items"`
	}
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil || len(request.Items) == 0 || len(request.Items) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid cache delete request"})
		return
	}
	deleter, ok := h.imageCache.(LegacyImageCacheDeleter)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Image cache deletion is unavailable"})
		return
	}
	result, err := deleter.DeleteImages(c.Request.Context(), request.Items)
	if err != nil && result.Deleted+result.Skipped+result.Failed == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to delete images"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) imagineCacheList(c *gin.Context) {
	pageValue, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	pageValue = max(pageValue, 1)
	pageSize = min(max(pageSize, 1), 200)

	items := []LegacyCachedImage{}
	if h.imageCache != nil {
		values, err := h.imageCache.ListImages(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to list images"})
			return
		}
		items = values
	}

	total := len(items)
	start, end := legacyPageRange(pageValue, pageSize, total)
	result := make([]gin.H, 0, end-start)
	for _, item := range items[start:end] {
		result = append(result, gin.H{
			"name": item.Name, "view_url": item.ViewURL, "source": item.Source, "cache_key": item.CacheKey,
			"size_bytes": item.SizeBytes, "mtime_ms": item.ModifiedAtMS,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "page": pageValue, "page_size": pageSize, "total": total})
}

var imageUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, HandshakeTimeout: 15 * time.Second}

func (h *Handler) imagineWS(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("task_id"))
	task := h.getImageTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Task not found"})
		return
	}
	clientValue, ok := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !ok || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	connection, err := imageUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	runContext, cancel := context.WithCancel(c.Request.Context())
	stopTaskWatch := context.AfterFunc(task.ctx, cancel)
	defer func() { stopTaskWatch(); cancel(); h.dropImageTask(taskID) }()
	go func() {
		for {
			_, message, readErr := connection.ReadMessage()
			if readErr != nil {
				cancel()
				return
			}
			var command struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(message, &command) == nil && strings.EqualFold(command.Type, "stop") {
				cancel()
				return
			}
		}
	}()
	if connection.WriteJSON(gin.H{"type": "status", "status": "running", "run_id": taskID}) != nil {
		return
	}
	for sequence := 1; ; sequence++ {
		if runContext.Err() != nil {
			_ = connection.WriteJSON(gin.H{"type": "status", "status": "stopped", "run_id": taskID})
			return
		}
		startedAt := time.Now()
		resolution := "1k"
		if task.pro {
			resolution = "2k"
		}
		result, generateErr := h.imageGenerator.GenerateImage(runContext, gateway.ImageGenerationInput{RequestID: taskID + "-" + fmt.Sprint(sequence), ClientKey: clientKey, PublicModel: legacyWebImageModel(task.pro), Prompt: task.prompt, Count: 1, AspectRatio: task.aspectRatio, Resolution: resolution, ResponseFormat: "b64_json", NSFW: &task.nsfw})
		if generateErr != nil {
			_ = connection.WriteJSON(gin.H{"type": "error", "message": generateErr.Error(), "code": "image_generation_failed"})
			return
		}
		response, consumeErr := consumeImageResult(result)
		if consumeErr != nil {
			_ = connection.WriteJSON(gin.H{"type": "error", "message": consumeErr.Error(), "code": "image_generation_failed"})
			return
		}
		for _, image := range response.Data {
			if image.Base64 == "" {
				continue
			}
			mimeType := image.MIMEType
			if mimeType == "" {
				mimeType = "image/jpeg"
			}
			imageID := fmt.Sprintf("local-%s-%d", taskID, sequence)
			source := "data:" + mimeType + ";base64," + image.Base64
			if connection.WriteJSON(gin.H{"type": "image", "b64_json": image.Base64, "mime_type": mimeType, "image_id": imageID, "parent_post_id": imageID, "current_source_image_url": source, "sequence": sequence, "prompt": task.prompt, "elapsed_ms": time.Since(startedAt).Milliseconds()}) != nil {
				return
			}
		}
	}
}

func (h *Handler) imagineConfig(c *gin.Context) {
	if !h.options.PublicEnabled {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"final_min_bytes":  0,
		"medium_min_bytes": 0,
		"nsfw":             h.options.AllowNSFW,
	})
}

func (h *Handler) imagineStart(c *gin.Context) {
	if h.imageGenerator == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Image generator is not configured"})
		return
	}
	var request imagineStartRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Prompt cannot be empty"})
		return
	}
	taskID, err := newTaskID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to create task"})
		return
	}
	taskContext, cancel := context.WithCancel(context.Background())
	task := &imageTask{
		id: taskID, prompt: request.Prompt, aspectRatio: normalizeImagineRatio(request.AspectRatio),
		nsfw: request.NSFW == nil || *request.NSFW, pro: request.Pro, ctx: taskContext, cancel: cancel,
	}
	h.imageMu.Lock()
	h.imageTasks[taskID] = task
	h.imageMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"task_id": taskID, "aspect_ratio": task.aspectRatio, "pro": task.pro})
}

func (h *Handler) imagineStop(c *gin.Context) {
	var request imagineStopRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	removed := 0
	for _, taskID := range request.TaskIDs {
		if h.dropImageTask(strings.TrimSpace(taskID)) {
			removed++
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "removed": removed})
}

func (h *Handler) imagineSSE(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("task_id"))
	task := h.getImageTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Task not found"})
		return
	}
	clientValue, ok := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !ok || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}

	runContext, cancel := context.WithCancel(c.Request.Context())
	stopTaskWatch := context.AfterFunc(task.ctx, cancel)
	defer func() {
		stopTaskWatch()
		cancel()
		h.dropImageTask(taskID)
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	runID := taskID
	writeLegacySSE(c, gin.H{"type": "status", "status": "running", "run_id": runID})

	sequence := 0
	for {
		if err := runContext.Err(); err != nil {
			writeLegacySSE(c, gin.H{"type": "status", "status": "stopped", "run_id": runID})
			return
		}
		sequence++
		startedAt := time.Now()
		resolution := "1k"
		if task.pro {
			resolution = "2k"
		}
		result, err := h.imageGenerator.GenerateImage(runContext, gateway.ImageGenerationInput{
			RequestID: taskID + "-" + fmt.Sprint(sequence), ClientKey: clientKey,
			PublicModel: legacyWebImageModel(task.pro), Prompt: task.prompt, Count: 1,
			AspectRatio: task.aspectRatio, Resolution: resolution, ResponseFormat: "b64_json", NSFW: &task.nsfw,
		})
		if err != nil {
			writeLegacySSE(c, gin.H{"type": "error", "message": err.Error(), "code": "image_generation_failed"})
			writeLegacySSE(c, gin.H{"type": "status", "status": "stopped", "run_id": runID})
			return
		}
		response, consumeErr := consumeImageResult(result)
		if consumeErr != nil {
			writeLegacySSE(c, gin.H{"type": "error", "message": consumeErr.Error(), "code": "image_generation_failed"})
			writeLegacySSE(c, gin.H{"type": "status", "status": "stopped", "run_id": runID})
			return
		}
		for _, image := range response.Data {
			if image.Base64 == "" {
				continue
			}
			mimeType := image.MIMEType
			if mimeType == "" {
				mimeType = "image/jpeg"
			}
			imageID := fmt.Sprintf("local-%s-%d", taskID, sequence)
			sourceImageURL := "data:" + mimeType + ";base64," + image.Base64
			writeLegacySSE(c, gin.H{
				"type": "image", "b64_json": image.Base64, "mime_type": mimeType,
				"image_id": imageID, "parent_post_id": imageID, "current_source_image_url": sourceImageURL,
				"sequence": sequence, "prompt": task.prompt, "elapsed_ms": time.Since(startedAt).Milliseconds(),
			})
		}
	}
}

func (h *Handler) imagineEdit(c *gin.Context) {
	h.handleImagineEdit(c, false)
}

func (h *Handler) imagineWorkbenchEdit(c *gin.Context) {
	h.handleImagineEdit(c, true)
}

func (h *Handler) handleImagineEdit(c *gin.Context, workbench bool) {
	editor, ok := h.imageGenerator.(ImageEditor)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Image editor is not configured"})
		return
	}
	var request imagineEditRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request"})
		return
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Prompt cannot be empty"})
		return
	}
	imageURLs := collectEditImages(request, workbench)
	if len(imageURLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "A source image is required"})
		return
	}
	if len(imageURLs) > 8 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "At most 8 source images are supported"})
		return
	}
	clientValue, exists := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	startedAt := time.Now()
	result, err := editor.EditImage(c.Request.Context(), gateway.ImageEditInput{
		RequestID: newEditRequestID(), ClientKey: clientKey, PublicModel: "grok-imagine-image-edit",
		Prompt: request.Prompt, ImageURLs: imageURLs, Count: 1, Resolution: "1k", ResponseFormat: "b64_json",
	})
	if err != nil {
		writeImagineEditError(c, request.Stream, err)
		return
	}
	response, err := consumeImageResult(result)
	if err != nil {
		writeImagineEditError(c, request.Stream, err)
		return
	}
	payload := buildImagineEditPayload(response, request, time.Since(startedAt).Milliseconds())
	if !request.Stream {
		c.JSON(http.StatusOK, payload)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	writeNamedLegacySSE(c, "progress", gin.H{"event": "request_accepted", "progress": 4, "message": "已接收编辑请求"})
	writeNamedLegacySSE(c, "progress", gin.H{"event": "completed", "progress": 100, "message": "编辑完成 100%"})
	writeNamedLegacySSE(c, "result", payload)
}

func collectEditImages(request imagineEditRequest, workbench bool) []string {
	values := make([]string, 0, 8)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if workbench {
		for _, value := range request.ImageReferences {
			add(value)
		}
		for _, item := range request.ReferenceItems {
			if strings.TrimSpace(item.SourceImageURL) != "" {
				add(item.SourceImageURL)
			} else if strings.TrimSpace(item.ImageURL) != "" {
				add(item.ImageURL)
			} else if parentPostIDPattern.MatchString(strings.TrimSpace(item.ParentPostID)) {
				add(imaginePublicImageURL(strings.TrimSpace(item.ParentPostID)))
			}
		}
	}
	add(request.SourceImageURL)
	add(request.ImageURL)
	if raw := strings.TrimSpace(request.ImageBase64); raw != "" {
		if strings.HasPrefix(raw, "data:image/") {
			add(raw)
		} else {
			add("data:image/jpeg;base64," + raw)
		}
	}
	if len(values) == 0 && parentPostIDPattern.MatchString(strings.TrimSpace(request.ParentPostID)) {
		add(imaginePublicImageURL(strings.TrimSpace(request.ParentPostID)))
	}
	return values
}

func imaginePublicImageURL(parentPostID string) string {
	return "https://imagine-public.x.ai/imagine-public/images/" + parentPostID + ".jpg"
}

func buildImagineEditPayload(response imageAPIResponse, request imagineEditRequest, elapsedMS int64) gin.H {
	currentParentID := strings.TrimSpace(request.ParentPostID)
	if currentParentID == "" {
		currentParentID = newEditRequestID()
	}
	currentSource := ""
	if len(response.Data) > 0 {
		first := response.Data[0]
		if first.Base64 != "" {
			mimeType := first.MIMEType
			if mimeType == "" {
				mimeType = "image/jpeg"
			}
			currentSource = "data:" + mimeType + ";base64," + first.Base64
		} else {
			currentSource = first.URL
		}
	}
	return gin.H{
		"created": response.Created, "data": response.Data,
		"parent_post_id": request.ParentPostID, "generated_parent_post_id": currentParentID,
		"current_parent_post_id": currentParentID, "current_source_image_url": currentSource,
		"elapsed_ms": elapsedMS,
	}
}

func writeImagineEditError(c *gin.Context, stream bool, err error) {
	if !stream {
		status := http.StatusBadGateway
		if upstreamStatus, ok := provider.ErrorHTTPStatus(err); ok && upstreamStatus >= 400 && upstreamStatus <= 599 {
			status = upstreamStatus
		}
		c.JSON(status, gin.H{"detail": err.Error()})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	writeNamedLegacySSE(c, "error", gin.H{"message": err.Error()})
}

func writeNamedLegacySSE(c *gin.Context, event string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw)
	c.Writer.Flush()
}

func newEditRequestID() string {
	value, err := newTaskID()
	if err != nil {
		return fmt.Sprintf("edit-%d", time.Now().UnixNano())
	}
	return "edit-" + value
}

func consumeImageResult(result *gateway.Result) (imageAPIResponse, error) {
	if result == nil || result.Body == nil {
		return imageAPIResponse{}, errors.New("image generation returned an empty response")
	}
	body, readErr := io.ReadAll(io.LimitReader(result.Body, maxLegacyImageResponseBytes+1))
	_ = result.Body.Close()
	errorCode := ""
	if readErr != nil {
		errorCode = "response_read_failed"
	}
	if result.Finalize != nil {
		result.Finalize(gateway.Usage{}, "", errorCode)
	}
	if readErr != nil {
		return imageAPIResponse{}, readErr
	}
	if len(body) > maxLegacyImageResponseBytes {
		return imageAPIResponse{}, errors.New("image response exceeded the safety limit")
	}
	var response imageAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return imageAPIResponse{}, errors.New("image generation returned invalid JSON")
	}
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		if response.Error != nil && strings.TrimSpace(response.Error.Message) != "" {
			return imageAPIResponse{}, errors.New(response.Error.Message)
		}
		return imageAPIResponse{}, fmt.Errorf("image generation returned HTTP %d", result.StatusCode)
	}
	if len(response.Data) == 0 {
		return imageAPIResponse{}, errors.New("image generation returned no images")
	}
	return response, nil
}

func writeLegacySSE(c *gin.Context, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", raw)
	c.Writer.Flush()
}

func (h *Handler) getImageTask(taskID string) *imageTask {
	h.imageMu.Lock()
	defer h.imageMu.Unlock()
	return h.imageTasks[taskID]
}

func (h *Handler) dropImageTask(taskID string) bool {
	if taskID == "" {
		return false
	}
	h.imageMu.Lock()
	task, ok := h.imageTasks[taskID]
	if ok {
		delete(h.imageTasks, taskID)
	}
	h.imageMu.Unlock()
	if ok {
		task.cancel()
	}
	return ok
}

func normalizeImagineRatio(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "1:1", "2:3", "3:2", "9:16", "16:9":
		return value
	default:
		return "2:3"
	}
}

func newTaskID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
