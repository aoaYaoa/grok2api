package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxLegacyVideoBytes int64 = 512 << 20

// LegacyVideoStore 保持旧版缓存文件命名和公开路由兼容。
type LegacyVideoStore struct{ root string }

func NewLegacyVideoStore(cacheRoot string) (*LegacyVideoStore, error) {
	root, err := filepath.Abs(filepath.Join(strings.TrimSpace(cacheRoot), "video"))
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		return nil, fmt.Errorf("视频缓存目录无效")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("创建视频缓存目录: %w", err)
	}
	return &LegacyVideoStore{root: filepath.Clean(root)}, nil
}

func (s *LegacyVideoStore) SaveVideo(ctx context.Context, sourceURL, mimeType string, body io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, err := legacyVideoFilename(sourceURL, mimeType)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(s.root, name)
	temporary, err := os.CreateTemp(s.root, ".video-*")
	if err != nil {
		return "", fmt.Errorf("创建视频临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o640); err != nil {
		return "", err
	}
	written, err := io.Copy(temporary, io.LimitReader(&contextReader{ctx: ctx, reader: body}, maxLegacyVideoBytes+1))
	if err != nil {
		return "", fmt.Errorf("写入视频: %w", err)
	}
	if written == 0 || written > maxLegacyVideoBytes {
		return "", fmt.Errorf("视频为空或超过 512 MiB")
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("同步视频文件: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("关闭视频文件: %w", err)
	}
	if err := os.Link(temporaryPath, destination); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("提交视频文件: %w", err)
	}
	return "/v1/files/video/" + url.PathEscape(name), nil
}

func legacyVideoFilename(sourceURL, mimeType string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || parsed.Path == "" {
		return "", fmt.Errorf("视频来源地址无效")
	}
	name := strings.Trim(parsed.EscapedPath(), "/")
	if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil {
		name = decoded
	}
	name = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(name)
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("视频文件名无效")
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		switch strings.ToLower(strings.TrimSpace(mimeType)) {
		case "video/webm":
			name += ".webm"
		case "video/quicktime":
			name += ".mov"
		default:
			name += ".mp4"
		}
	} else if extension != ".mp4" && extension != ".webm" && extension != ".mov" && extension != ".m4v" {
		return "", fmt.Errorf("视频扩展名不受支持")
	}
	return name, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
