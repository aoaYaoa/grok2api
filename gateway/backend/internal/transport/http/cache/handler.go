package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

var postIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

var allowedExtensions = map[string]map[string]struct{}{
	"image": {".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".webp": {}, ".bmp": {}},
	"video": {".mp4": {}, ".mov": {}, ".m4v": {}, ".webm": {}, ".avi": {}, ".mkv": {}},
}

type Handler struct {
	root       string
	mediaRoots map[string]*os.Root
}

type Stats struct {
	Count     int     `json:"count"`
	SizeBytes int64   `json:"sizeBytes"`
	SizeMB    float64 `json:"sizeMB"`
}

type Item struct {
	Name           string `json:"name"`
	SizeBytes      int64  `json:"sizeBytes"`
	ModifiedAtMS   int64  `json:"modifiedAtMs"`
	ViewURL        string `json:"viewURL"`
	PreviewURL     string `json:"previewURL,omitempty"`
	PostID         string `json:"postID,omitempty"`
	ShareLink      string `json:"shareLink,omitempty"`
	OriginalPostID string `json:"originalPostID,omitempty"`
	MediaURL       string `json:"mediaURL,omitempty"`
	ThumbnailURL   string `json:"thumbnailURL,omitempty"`
	LocalURL       string `json:"localURL,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
}

type ListResult struct {
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Items    []Item `json:"items"`
}

func NewHandler(root string) (*Handler, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("legacy cache path is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve legacy cache path: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("create legacy cache root: %w", err)
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("legacy cache root must be a real directory")
	}
	mediaRoots := make(map[string]*os.Root, 3)
	for _, directory := range []string{"image", "video", "media-meta"} {
		path := filepath.Join(absolute, directory)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(path, 0o750); err != nil {
				return nil, fmt.Errorf("create legacy cache directory %s: %w", directory, err)
			}
			info, statErr = os.Lstat(path)
		}
		if statErr != nil {
			return nil, fmt.Errorf("inspect legacy cache directory %s: %w", directory, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("legacy cache directory %s must be a real directory", directory)
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			return nil, fmt.Errorf("open legacy cache directory %s: %w", directory, err)
		}
		mediaRoots[directory] = root
	}
	return &Handler{root: filepath.Clean(absolute), mediaRoots: mediaRoots}, nil
}

func (h *Handler) RegisterPublic(router gin.IRoutes) {
	router.GET("/v1/files/:type/*name", h.serveFile)
	router.HEAD("/v1/files/:type/*name", h.serveFile)
}

func (h *Handler) RegisterAdmin(router *gin.RouterGroup) {
	router.GET("/cache", h.adminStats)
	router.DELETE("/cache", h.adminClear)
	router.GET("/cache/items", h.adminList)
	router.DELETE("/cache/items", h.adminDelete)
	router.PATCH("/cache/videos/:postID", h.adminRenameVideo)
}

func (h *Handler) RegisterLegacy(router *gin.RouterGroup) {
	router.GET("/cache", h.legacyStats)
	router.GET("/cache/list", h.legacyList)
	router.POST("/cache/clear", h.legacyClear)
	router.POST("/cache/item/delete", h.legacyDelete)
	router.POST("/cache/video/rename", h.legacyRenameVideo)
}

func (h *Handler) adminStats(c *gin.Context) {
	image, err := h.stats("image")
	if err != nil {
		respondAdminError(c, err)
		return
	}
	video, err := h.stats("video")
	if err != nil {
		respondAdminError(c, err)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"image": image, "video": video})
}

func (h *Handler) adminList(c *gin.Context) {
	result, err := h.list(c.Query("type"), queryInt(c, "page", 1), queryInt(c, "pageSize", 100))
	if err != nil {
		respondAdminError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result)
}

func (h *Handler) adminDelete(c *gin.Context) {
	deleted, err := h.delete(c.Query("type"), c.Query("name"))
	if err != nil {
		respondAdminError(c, err)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"deleted": deleted})
}

func (h *Handler) adminClear(c *gin.Context) {
	stats, err := h.clear(c.Query("type"))
	if err != nil {
		respondAdminError(c, err)
		return
	}
	response.Success(c, http.StatusOK, stats)
}

func (h *Handler) adminRenameVideo(c *gin.Context) {
	var request struct {
		DisplayName string `json:"displayName"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "invalidCacheRequest", "invalid request body")
		return
	}
	result, err := h.renameVideo(c.Param("postID"), "", "", request.DisplayName)
	if err != nil {
		respondAdminError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result)
}

func (h *Handler) legacyStats(c *gin.Context) {
	image, err := h.stats("image")
	if err != nil {
		respondError(c, err)
		return
	}
	video, err := h.stats("video")
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"local_image":     legacyStats(image),
		"local_video":     legacyStats(video),
		"online":          gin.H{"count": 0, "status": "not_loaded", "token": nil, "last_asset_clear_at": nil},
		"online_accounts": []any{}, "online_scope": "none", "online_details": []any{},
	})
}

func (h *Handler) legacyList(c *gin.Context) {
	mediaType := c.DefaultQuery("cache_type", "image")
	if value := strings.TrimSpace(c.Query("type")); value != "" {
		mediaType = value
	}
	result, err := h.list(mediaType, queryInt(c, "page", 1), queryInt(c, "page_size", 1000))
	if err != nil {
		respondError(c, err)
		return
	}
	items := make([]gin.H, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, legacyItem(item))
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": result.Total, "page": result.Page, "page_size": result.PageSize, "items": items})
}

func (h *Handler) legacyClear(c *gin.Context) {
	var request struct {
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request body"})
		return
	}
	result, err := h.clear(request.Type)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": legacyStats(result)})
}

func (h *Handler) legacyDelete(c *gin.Context) {
	var request struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request body"})
		return
	}
	deleted, err := h.delete(request.Type, request.Name)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": gin.H{"deleted": deleted}})
}

func (h *Handler) legacyRenameVideo(c *gin.Context) {
	var request struct {
		PostID      string `json:"post_id"`
		ShareLink   string `json:"share_link"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid request body"})
		return
	}
	result, err := h.renameVideo(request.PostID, request.ShareLink, request.Name, request.DisplayName)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": gin.H{
		"post_id": result.PostID, "display_name": result.DisplayName, "share_link": result.ShareLink,
	}})
}

func (h *Handler) serveFile(c *gin.Context) {
	mediaType, err := normalizeType(c.Param("type"))
	if err != nil {
		respondError(c, err)
		return
	}
	name := strings.TrimSpace(strings.TrimPrefix(c.Param("name"), "/"))
	path, err := h.filePath(mediaType, name)
	if err != nil {
		respondError(c, err)
		return
	}
	root := h.mediaRoots[mediaType]
	if root == nil {
		respondError(c, errors.New("cache operation failed"))
		return
	}
	name = filepath.Base(path)
	lstat, err := root.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.Status(http.StatusNotFound)
			return
		}
		respondError(c, err)
		return
	}
	if lstat.Mode()&os.ModeSymlink != 0 || !lstat.Mode().IsRegular() {
		respondError(c, errors.New("invalid cache file name"))
		return
	}
	file, err := root.Open(name)
	if err != nil {
		respondError(c, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=86400")
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), file)
}

func (h *Handler) stats(mediaType string) (Stats, error) {
	mediaType, err := normalizeType(mediaType)
	if err != nil {
		return Stats{}, err
	}
	root, entries, err := h.rootEntries(mediaType)
	if err != nil {
		return Stats{}, err
	}
	var result Stats
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !allowedFile(mediaType, entry.Name()) {
			continue
		}
		info, err := root.Lstat(entry.Name())
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		result.Count++
		result.SizeBytes += info.Size()
	}
	result.SizeMB = roundMB(result.SizeBytes)
	return result, nil
}

func (h *Handler) list(mediaType string, page, pageSize int) (ListResult, error) {
	mediaType, err := normalizeType(mediaType)
	if err != nil {
		return ListResult{}, err
	}
	page = max(page, 1)
	pageSize = min(max(pageSize, 1), 1000)
	items, err := h.collectItems(mediaType)
	if err != nil {
		return ListResult{}, err
	}
	total := len(items)
	start, end := pageRange(page, pageSize, total)
	return ListResult{Total: total, Page: page, PageSize: pageSize, Items: items[start:end]}, nil
}

func (h *Handler) collectItems(mediaType string) ([]Item, error) {
	root, entries, err := h.rootEntries(mediaType)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(entries))
	metadata := map[string]map[string]any{}
	if mediaType == "video" {
		metadata = h.videoMetadata()
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !allowedFile(mediaType, entry.Name()) {
			continue
		}
		info, err := root.Lstat(entry.Name())
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		item := Item{Name: entry.Name(), SizeBytes: info.Size(), ModifiedAtMS: info.ModTime().UnixMilli()}
		item.ViewURL = "/v1/files/" + mediaType + "/" + entry.Name()
		if mediaType == "image" {
			item.PreviewURL = item.ViewURL
		} else if postID := extractPostID(entry.Name()); postID != "" {
			applyMetadata(&item, metadata[postID])
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModifiedAtMS > items[j].ModifiedAtMS })
	return items, nil
}

func (h *Handler) ListAllItems(mediaType string) ([]Item, error) {
	mediaType, err := normalizeType(mediaType)
	if err != nil {
		return nil, err
	}
	return h.collectItems(mediaType)
}

func (h *Handler) RenameVideo(postID, shareLink, name, displayName string) (Item, error) {
	return h.renameVideo(postID, shareLink, name, displayName)
}

func (h *Handler) DeleteItem(mediaType, name string) (bool, error) {
	mediaType, err := normalizeType(mediaType)
	if err != nil {
		return false, err
	}
	if err := validateDeleteName(mediaType, name); err != nil {
		return false, err
	}
	path, err := h.filePath(mediaType, name)
	if err != nil {
		return false, err
	}
	if filepath.Base(path) != name {
		return false, errors.New("invalid cache file name")
	}
	root := h.mediaRoots[mediaType]
	if root == nil {
		return false, errors.New("cache operation failed")
	}
	entry, err := root.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mediaType == "video" {
				if cleanupErr := h.removeOrphanVideoMetadata(extractPostID(name)); cleanupErr != nil {
					return false, cleanupErr
				}
			}
			return false, nil
		}
		return false, err
	}
	if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
		return false, errors.New("invalid cache file name")
	}
	if err := root.Remove(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mediaType == "video" {
				if cleanupErr := h.removeOrphanVideoMetadata(extractPostID(name)); cleanupErr != nil {
					return false, cleanupErr
				}
			}
			return false, nil
		}
		return false, err
	}
	if mediaType == "video" {
		if err := h.removeOrphanVideoMetadata(extractPostID(name)); err != nil {
			return true, err
		}
	}
	return true, nil
}

func validateDeleteName(mediaType, name string) error {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) || filepath.Base(name) != name || !allowedFile(mediaType, name) {
		return errors.New("invalid cache file name")
	}
	return nil
}

func (h *Handler) delete(mediaType, name string) (bool, error) {
	return h.DeleteItem(mediaType, name)
}

func (h *Handler) removeOrphanVideoMetadata(postID string) error {
	if postID == "" {
		return nil
	}
	root, entries, err := h.rootEntries("video")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !allowedFile("video", entry.Name()) {
			continue
		}
		info, err := root.Lstat(entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		if extractPostID(entry.Name()) == postID {
			return nil
		}
	}
	return h.removeVideoMetadata(postID)
}

func (h *Handler) removeVideoMetadata(postID string) error {
	root := h.mediaRoots["media-meta"]
	if root == nil {
		return errors.New("cache operation failed")
	}
	name := postID + ".json"
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
		return errors.New("invalid video metadata file")
	}
	if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (h *Handler) clear(mediaType string) (Stats, error) {
	mediaType, err := normalizeType(mediaType)
	if err != nil {
		return Stats{}, err
	}
	root, entries, err := h.rootEntries(mediaType)
	if err != nil {
		return Stats{}, err
	}
	var removed Stats
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !allowedFile(mediaType, entry.Name()) {
			continue
		}
		info, err := root.Lstat(entry.Name())
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err := root.Remove(entry.Name()); err != nil {
			return removed, err
		}
		removed.Count++
		removed.SizeBytes += info.Size()
	}
	removed.SizeMB = roundMB(removed.SizeBytes)
	return removed, nil
}

func (h *Handler) renameVideo(postID, shareLink, name, displayName string) (Item, error) {
	postID = strings.TrimSpace(postID)
	if !postIDPattern.MatchString(postID) {
		postID = extractPostID(shareLink)
	}
	if postID == "" {
		postID = extractPostID(name)
	}
	if !postIDPattern.MatchString(postID) || postIDPattern.FindString(postID) != postID {
		return Item{}, errors.New("unable to resolve post_id")
	}
	displayName = strings.TrimSpace(displayName)
	if len([]rune(displayName)) > 160 {
		return Item{}, errors.New("display name must be at most 160 characters")
	}
	items, err := h.collectItems("video")
	if err != nil {
		return Item{}, err
	}
	found := false
	for _, item := range items {
		if item.PostID == postID || extractPostID(item.Name) == postID {
			found = true
			break
		}
	}
	if !found {
		return Item{}, errors.New("video cache item does not exist")
	}
	payload := map[string]any{}
	path := filepath.Join(h.root, "media-meta", postID+".json")
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &payload)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Item{}, err
	}
	payload["post_id"] = postID
	if strings.TrimSpace(stringValue(payload["media_type"])) == "" {
		payload["media_type"] = "video"
	}
	if displayName == "" {
		delete(payload, "display_name")
	} else {
		payload["display_name"] = displayName
	}
	if err := writeJSONAtomic(path, payload); err != nil {
		return Item{}, err
	}
	item := Item{PostID: postID}
	applyMetadata(&item, payload)
	return item, nil
}

func (h *Handler) videoMetadata() map[string]map[string]any {
	result := map[string]map[string]any{}
	root, entries, err := h.rootEntries("media-meta")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		info, err := root.Lstat(entry.Name())
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		raw, err := root.ReadFile(entry.Name())
		if err != nil {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil || !strings.EqualFold(strings.TrimSpace(stringValue(payload["media_type"])), "video") {
			continue
		}
		postID := strings.TrimSpace(stringValue(payload["post_id"]))
		if postID != "" {
			result[postID] = payload
		}
	}
	return result
}

func (h *Handler) rootEntries(name string) (*os.Root, []os.DirEntry, error) {
	root := h.mediaRoots[name]
	if root == nil {
		return nil, nil, errors.New("cache operation failed")
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, nil, err
	}
	return root, entries, nil
}

func (h *Handler) filePath(mediaType, name string) (string, error) {
	name, err := normalizeCachedFilename(name)
	if err != nil || !allowedFile(mediaType, name) {
		return "", errors.New("invalid cache file name")
	}
	return filepath.Join(h.root, mediaType, name), nil
}

func normalizeCachedFilename(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimRight(value, "/\\")
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "://") || strings.HasPrefix(value, "\\") {
		return "", errors.New("invalid cache file name")
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", errors.New("invalid cache file name")
		}
	}
	normalized = strings.ReplaceAll(normalized, "/", "-")
	if normalized == "" || filepath.Base(normalized) != normalized {
		return "", errors.New("invalid cache file name")
	}
	return normalized, nil
}

func normalizeType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := allowedExtensions[value]; !ok {
		return "", errors.New("cache type must be image or video")
	}
	return value, nil
}

func allowedFile(mediaType, name string) bool {
	_, ok := allowedExtensions[mediaType][strings.ToLower(filepath.Ext(name))]
	return ok
}

func extractPostID(value string) string {
	return postIDPattern.FindString(strings.TrimSpace(value))
}

func applyMetadata(item *Item, payload map[string]any) {
	if payload == nil {
		return
	}
	item.PostID = stringValue(payload["post_id"])
	item.ShareLink = stringValue(payload["share_link"])
	item.OriginalPostID = stringValue(payload["original_post_id"])
	item.MediaURL = stringValue(payload["media_url"])
	item.ThumbnailURL = stringValue(payload["thumbnail_url"])
	item.LocalURL = stringValue(payload["local_url"])
	item.DisplayName = stringValue(payload["display_name"])
}

func legacyStats(value Stats) gin.H {
	return gin.H{"count": value.Count, "size_mb": value.SizeMB}
}

func legacyItem(item Item) gin.H {
	return gin.H{
		"name": item.Name, "size_bytes": item.SizeBytes, "mtime_ms": item.ModifiedAtMS,
		"view_url": item.ViewURL, "preview_url": item.PreviewURL, "post_id": item.PostID,
		"share_link": item.ShareLink, "original_post_id": item.OriginalPostID, "media_url": item.MediaURL,
		"thumbnail_url": item.ThumbnailURL, "local_url": item.LocalURL, "display_name": item.DisplayName,
	}
}

func queryInt(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(name)))
	if err != nil {
		return fallback
	}
	return value
}

func pageRange(page, pageSize, total int) (int, int) {
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

func roundMB(bytes int64) float64 {
	return math.Round(float64(bytes)/(1024*1024)*100) / 100
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func writeJSONAtomic(path string, payload map[string]any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".metadata-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func respondError(c *gin.Context, err error) {
	status, message := cacheErrorDetails(err)
	c.JSON(status, gin.H{"error": message, "detail": message})
}

func respondAdminError(c *gin.Context, err error) {
	status, message := cacheErrorDetails(err)
	code := "cacheOperationFailed"
	if status == http.StatusBadRequest {
		code = "invalidCacheRequest"
	}
	response.Error(c, status, code, message)
}

func cacheErrorDetails(err error) (int, string) {
	status := http.StatusInternalServerError
	message := "cache operation failed"
	if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "must be") || strings.Contains(err.Error(), "resolve post_id") || strings.Contains(err.Error(), "does not exist") {
		status = http.StatusBadRequest
		message = err.Error()
	}
	return status, message
}
