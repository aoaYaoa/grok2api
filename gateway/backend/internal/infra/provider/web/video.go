package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	domainegress "github.com/chenyme/grok2api/backend/internal/domain/egress"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/infra/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

type videoUpstreamError struct {
	status int
	body   string
}

func (e *videoUpstreamError) Error() string {
	return fmt.Sprintf("视频上游返回 %d: %s", e.status, e.body)
}

func (e *videoUpstreamError) HTTPStatusCode() int { return e.status }

func (a *Adapter) GenerateVideo(ctx context.Context, request provider.VideoRequest) (provider.VideoResult, error) {
	cfg := a.config()
	token, err := a.cipher.Decrypt(request.Credential.EncryptedAccessToken)
	if err != nil {
		return provider.VideoResult{}, err
	}
	lease, err := a.egress.Acquire(ctx, domainegress.ScopeWeb, fmt.Sprintf("%d", request.Credential.ID))
	if err != nil {
		return provider.VideoResult{}, err
	}
	defer lease.Release()
	if request.IsExtension {
		return a.generateExtendedVideo(ctx, cfg, lease, token, request)
	}
	parentID := ""
	references := make([]string, 0, len(request.ReferenceURLs))
	for _, rawReference := range request.ReferenceURLs {
		reference, referenceErr := a.prepareVideoReference(ctx, cfg, lease, token, rawReference)
		if referenceErr != nil {
			return provider.VideoResult{}, referenceErr
		}
		references = append(references, reference)
	}
	if len(references) > 0 {
		parentID, err = a.createMediaPost(ctx, cfg, lease, token, "MEDIA_POST_TYPE_IMAGE", references[0], "")
	} else {
		parentID, err = a.createMediaPost(ctx, cfg, lease, token, "MEDIA_POST_TYPE_VIDEO", "", request.Prompt)
	}
	if err != nil {
		return provider.VideoResult{}, err
	}
	segments := videoSegments(request.Duration)
	if len(segments) == 0 {
		return provider.VideoResult{}, fmt.Errorf("duration 必须在 1 到 15 秒之间")
	}
	ratio := resolveAspectRatio(request.AspectRatio)
	resolution := request.Resolution
	if resolution == "" {
		resolution = "720p"
	}
	payload := videoCreatePayload(request.Prompt, parentID, ratio, resolution, segments[0], references)
	response, err := a.postJSON(ctx, cfg, lease, token, cfg.BaseURL+"/rest/app-chat/conversations/new", payload, time.Duration(cfg.VideoTimeoutSeconds)*time.Second)
	if err != nil {
		return provider.VideoResult{}, err
	}
	result, _, parseErr := parseVideoStream(response, request.Progress)
	_ = response.Body.Close()
	if parseErr != nil {
		return provider.VideoResult{}, parseErr
	}
	if result.URL == "" {
		return provider.VideoResult{}, fmt.Errorf("视频生成完成但没有返回内容 URL")
	}
	return a.ArchiveVideo(ctx, request.Credential, result)
}

func (a *Adapter) generateExtendedVideo(ctx context.Context, cfg Config, lease *egress.Lease, token string, request provider.VideoRequest) (provider.VideoResult, error) {
	startTime := request.ExtensionStartTime
	if startTime <= 0 {
		startTime = 6
	}
	originalPostID := strings.TrimSpace(request.OriginalPostID)
	if originalPostID == "" {
		originalPostID = request.ExtendPostID
	}
	mode := "normal"
	prompt := strings.TrimSpace(request.Prompt)
	if prompt != "" {
		mode = "custom"
	}
	message := "--mode=" + mode
	if prompt != "" {
		message = prompt + " --mode=" + mode
	}
	config := map[string]any{
		"isVideoExtension": true, "videoExtensionStartTime": startTime, "extendPostId": request.ExtendPostID,
		"stitchWithExtendPostId": request.StitchWithExtend, "originalPostId": originalPostID,
		"originalRefType": "ORIGINAL_REF_TYPE_VIDEO_EXTENSION", "mode": mode, "aspectRatio": resolveAspectRatio(request.AspectRatio),
		"videoLength": request.Duration, "resolutionName": request.Resolution, "parentPostId": request.ExtendPostID, "isVideoEdit": false,
	}
	if prompt != "" {
		config["originalPrompt"] = prompt
	}
	payload := map[string]any{
		"temporary": true, "modelName": "grok-3", "message": message, "fileAttachments": []string{},
		"toolOverrides": map[string]any{"videoGen": true}, "enableSideBySide": true,
		"responseMetadata": map[string]any{"experiments": []any{}, "modelConfigOverride": map[string]any{"modelMap": map[string]any{"videoGenModelConfig": config}}},
	}
	response, err := a.postJSON(ctx, cfg, lease, token, cfg.BaseURL+"/rest/app-chat/conversations/new", payload, time.Duration(cfg.VideoTimeoutSeconds)*time.Second)
	if err != nil {
		return provider.VideoResult{}, err
	}
	result, _, parseErr := parseVideoStream(response, request.Progress)
	_ = response.Body.Close()
	if parseErr != nil {
		return provider.VideoResult{}, parseErr
	}
	if result.URL == "" {
		return provider.VideoResult{}, fmt.Errorf("视频延长完成但没有返回内容 URL")
	}
	return a.ArchiveVideo(ctx, request.Credential, result)
}

// ArchiveVideo 使用生成账号的 SSO 会话下载受保护的视频，并返回本站文件地址。
func (a *Adapter) ArchiveVideo(ctx context.Context, credential account.Credential, result provider.VideoResult) (provider.VideoResult, error) {
	if strings.HasPrefix(strings.TrimSpace(result.URL), "/v1/files/video/") {
		return a.archiveVideoPoster(ctx, credential, result), nil
	}
	if a.videos == nil {
		return provider.VideoResult{}, provider.NewMediaPostProcessingError(provider.MediaPostProcessingStorage, fmt.Errorf("视频媒体存储未配置"))
	}
	cfg := a.config()
	parsed, err := url.Parse(strings.TrimSpace(result.URL))
	if err != nil || parsed.User != nil || !trustedVideoAssetURL(parsed, cfg.BaseURL) {
		return provider.VideoResult{}, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, fmt.Errorf("视频内容 URL 不受信任"))
	}
	token, err := a.cipher.Decrypt(credential.EncryptedAccessToken)
	if err != nil {
		return provider.VideoResult{}, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, err)
	}
	downloadCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.VideoTimeoutSeconds)*time.Second)
	defer cancel()
	var lastErr error
	lastStage := provider.MediaPostProcessingDownload
	for attempt := 0; attempt < mediaOutputAttempts; attempt++ {
		localURL, stage, retryable, attemptErr := a.archiveVideoAttempt(downloadCtx, credential.ID, token, parsed.String(), result.ContentType)
		if attemptErr == nil {
			result.URL = localURL
			if result.ContentType == "" {
				result.ContentType = "video/mp4"
			}
			return a.archiveVideoPoster(ctx, credential, result), nil
		}
		lastErr, lastStage = attemptErr, stage
		if !retryable || downloadCtx.Err() != nil || attempt+1 >= mediaOutputAttempts {
			break
		}
		if err := waitMediaOutputRetry(downloadCtx, attempt); err != nil {
			lastErr = err
			break
		}
	}
	return provider.VideoResult{}, provider.NewMediaPostProcessingError(lastStage, lastErr)
}

func (a *Adapter) archiveVideoPoster(ctx context.Context, credential account.Credential, result provider.VideoResult) provider.VideoResult {
	posterURL := strings.TrimSpace(result.PosterURL)
	if posterURL == "" || strings.HasPrefix(posterURL, "/v1/files/image/") {
		return result
	}
	store, ok := a.videos.(provider.VideoPosterStore)
	if !ok {
		result.PosterURL = ""
		return result
	}
	raw, err := a.downloadImage(ctx, credential, posterURL)
	if err != nil {
		a.log().Warn("video_poster_download_failed", "url", posterURL, "error", err)
		result.PosterURL = ""
		return result
	}
	localURL, err := store.SaveVideoPoster(ctx, posterURL, raw)
	if err != nil {
		a.log().Warn("video_poster_store_failed", "url", posterURL, "error", err)
		result.PosterURL = ""
		return result
	}
	result.PosterURL = localURL
	return result
}

func (a *Adapter) archiveVideoAttempt(ctx context.Context, accountID uint64, token, rawURL, fallbackContentType string) (string, provider.MediaPostProcessingStage, bool, error) {
	lease, err := a.egress.Acquire(ctx, domainegress.ScopeWebAsset, fmt.Sprintf("%d", accountID))
	if err != nil {
		return "", provider.MediaPostProcessingDownload, true, err
	}
	defer lease.Release()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", provider.MediaPostProcessingDownload, false, err
	}
	request.Header = buildHeaders(token, lease, "")
	request.Header.Del("Content-Type")
	request.Header.Set("Range", "bytes=0-")
	applyAppHeaders(request.Header, "https://grok.com", "https://grok.com/")
	request.Header.Set("Sec-Fetch-Site", "same-site")
	response, err := lease.Do(request)
	if err != nil {
		a.egress.Feedback(context.WithoutCancel(ctx), lease.NodeID, 0, err)
		return "", provider.MediaPostProcessingDownload, ctx.Err() == nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		a.egress.Feedback(context.WithoutCancel(ctx), lease.NodeID, response.StatusCode, nil)
		retryable := response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooEarly || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return "", provider.MediaPostProcessingDownload, retryable, fmt.Errorf("下载视频返回 %d", response.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType == "" {
		contentType = strings.ToLower(strings.TrimSpace(strings.Split(fallbackContentType, ";")[0]))
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	if !strings.HasPrefix(contentType, "video/") {
		return "", provider.MediaPostProcessingDownload, false, fmt.Errorf("上游视频 Content-Type 无效")
	}
	localURL, err := a.videos.SaveVideo(ctx, rawURL, contentType, response.Body)
	if err != nil {
		return "", provider.MediaPostProcessingStorage, ctx.Err() == nil, err
	}
	a.egress.Feedback(context.WithoutCancel(ctx), lease.NodeID, response.StatusCode, nil)
	return localURL, provider.MediaPostProcessingStorage, false, nil
}

func trustedVideoAssetHost(host, baseURL string) bool {
	if strings.EqualFold(host, "assets.grok.com") || strings.EqualFold(host, "imagine-public.x.ai") {
		return true
	}
	parsed, err := url.Parse(baseURL)
	return err == nil && parsed.Hostname() != "" && strings.EqualFold(host, parsed.Hostname())
}

func trustedVideoAssetURL(value *url.URL, baseURL string) bool {
	if value == nil {
		return false
	}
	if strings.EqualFold(value.Scheme, "https") {
		return trustedVideoAssetHost(value.Hostname(), baseURL)
	}
	if !strings.EqualFold(value.Scheme, "http") {
		return false
	}
	base, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(base.Scheme, "http") && strings.EqualFold(value.Hostname(), base.Hostname()) && value.Port() == base.Port()
}

func (a *Adapter) prepareVideoReference(ctx context.Context, cfg Config, lease *egress.Lease, token, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("视频参考图片 URL 不能为空")
	}
	var image provider.ImageInput
	var err error
	if assetID, ok := mediadomain.ParseImageReference(value); ok {
		image, err = a.loadStoredVideoReference(ctx, assetID, 20<<20)
	} else {
		image, err = a.loadChatImage(ctx, lease, value, 20<<20)
	}
	if err != nil {
		return "", err
	}
	uploaded, err := a.uploadImage(ctx, cfg, lease, token, image, cfg.BaseURL+"/imagine")
	if err != nil {
		return "", err
	}
	if uploaded.URI == "" {
		return "", fmt.Errorf("上传视频参考图片后未返回 fileUri")
	}
	return uploaded.URI, nil
}

func (a *Adapter) loadStoredVideoReference(ctx context.Context, assetID string, maxBytes int64) (provider.ImageInput, error) {
	reader, ok := a.assets.(provider.ImageAssetReader)
	if !ok {
		return provider.ImageInput{}, fmt.Errorf("视频参考图存储不支持读取")
	}
	asset, body, err := reader.OpenImage(ctx, assetID)
	if err != nil {
		return provider.ImageInput{}, fmt.Errorf("读取视频参考图: %w", err)
	}
	defer body.Close()
	if asset.SizeBytes <= 0 || asset.SizeBytes > maxBytes {
		return provider.ImageInput{}, fmt.Errorf("视频参考图为空或超过 %d MiB", maxBytes>>20)
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil || int64(len(raw)) > maxBytes {
		return provider.ImageInput{}, fmt.Errorf("读取视频参考图失败或超过 %d MiB", maxBytes>>20)
	}
	mimeType, err := validatedImageMIME(raw, asset.MIMEType)
	if err != nil {
		return provider.ImageInput{}, err
	}
	return provider.ImageInput{Filename: "image" + imageExtension(mimeType), MIMEType: mimeType, Data: raw}, nil
}

func parseVideoStream(response *http.Response, progress func(int)) (provider.VideoResult, string, error) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if response.StatusCode == http.StatusUnauthorized {
			return provider.VideoResult{}, "", provider.ErrUnauthorized
		}
		return provider.VideoResult{}, "", &videoUpstreamError{status: response.StatusCode, body: strings.TrimSpace(string(body))}
	}
	var result provider.VideoResult
	var postID string
	handle := func(root map[string]any) (bool, error) {
		if errorValue, ok := root["error"].(map[string]any); ok {
			return false, fmt.Errorf("视频上游错误: %v", errorValue["message"])
		}
		stream := nestedMap(root, "result", "response", "streamingVideoGenerationResponse")
		if stream == nil {
			return false, nil
		}
		if value, ok := numberAsInt(stream["progress"]); ok && progress != nil {
			progress(value)
		}
		if value, _ := stream["videoPostId"].(string); value != "" {
			postID = value
		} else if value, _ := stream["videoId"].(string); value != "" {
			postID = value
		}
		moderated, _ := stream["moderated"].(bool)
		if moderated {
			return false, nil
		}
		if value, _ := stream["thumbnailImageUrl"].(string); value != "" {
			result.PosterURL = absoluteVideoAssetURL(value)
		}
		if value, _ := stream["videoUrl"].(string); value != "" {
			result.URL = absoluteVideoAssetURL(value)
			result.ContentType = "video/mp4"
			return true, nil
		}
		return false, nil
	}

	reader := bufio.NewReader(response.Body)
	prefix, _ := reader.Peek(64)
	trimmedPrefix := strings.TrimSpace(string(prefix))
	var err error
	if strings.HasPrefix(trimmedPrefix, "data:") || strings.HasPrefix(trimmedPrefix, "event:") {
		err = consumeVideoSSE(reader, handle)
	} else {
		err = consumeVideoJSON(reader, handle)
	}
	if err != nil {
		return provider.VideoResult{}, "", err
	}
	return result, postID, nil
}

func absoluteVideoAssetURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		return value
	}
	return "https://assets.grok.com/" + strings.TrimPrefix(value, "/")
}

func consumeVideoSSE(reader io.Reader, handle func(map[string]any) (bool, error)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || line == "[DONE]" || !strings.HasPrefix(line, "{") {
			continue
		}
		var root map[string]any
		if json.Unmarshal([]byte(line), &root) != nil {
			continue
		}
		complete, err := handle(root)
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
	}
	return scanner.Err()
}

func consumeVideoJSON(reader io.Reader, handle func(map[string]any) (bool, error)) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 64<<20))
	for {
		var root map[string]any
		if err := decoder.Decode(&root); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("解析视频上游流: %w", err)
		}
		complete, err := handle(root)
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
	}
}

func nestedMap(value map[string]any, keys ...string) map[string]any {
	current := value
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func videoSegments(seconds int) []int {
	if seconds < 1 || seconds > 15 {
		return nil
	}
	return []int{seconds}
}

func videoCreatePayload(prompt, parentID, ratio, resolution string, seconds int, references []string) map[string]any {
	config := map[string]any{"parentPostId": parentID, "aspectRatio": ratio, "videoLength": seconds, "resolutionName": resolution}
	if len(references) > 0 {
		config["isVideoEdit"] = false
		config["isReferenceToVideo"] = true
		config["imageReferences"] = references
	}
	return map[string]any{
		"temporary": true, "modelName": "imagine-video-gen", "message": prompt + " --mode=custom", "enableSideBySide": true,
		"responseMetadata": map[string]any{"experiments": []any{}, "modelConfigOverride": map[string]any{"modelMap": map[string]any{"videoGenModelConfig": config}}},
	}
}
