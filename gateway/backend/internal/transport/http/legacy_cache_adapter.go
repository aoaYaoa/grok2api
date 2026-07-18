package httpserver

import (
	"context"
	"errors"
	"sort"
	"strings"

	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	cachehttp "github.com/chenyme/grok2api/backend/internal/transport/http/cache"
	legacyhttp "github.com/chenyme/grok2api/backend/internal/transport/http/legacy"
)

type legacyVideoCacheAdapter struct {
	handler *cachehttp.Handler
}

type legacyImageCacheAdapter struct {
	handler *cachehttp.Handler
	media   legacyImageMediaLibrary
}

type legacyImageMediaLibrary interface {
	AdminListImages(context.Context, int, int, string) ([]mediadomain.Asset, int64, error)
	PublicImageURL(string) string
}

func newLegacyImageCacheAdapter(handler *cachehttp.Handler, media legacyImageMediaLibrary) legacyhttp.LegacyImageCache {
	if handler == nil && media == nil {
		return nil
	}
	return &legacyImageCacheAdapter{handler: handler, media: media}
}

func (a *legacyImageCacheAdapter) ListImages(ctx context.Context) ([]legacyhttp.LegacyCachedImage, error) {
	result := make([]legacyhttp.LegacyCachedImage, 0)
	if a.handler != nil {
		items, err := a.handler.ListAllItems("image")
		if err != nil {
			return nil, err
		}
		result = make([]legacyhttp.LegacyCachedImage, 0, len(items))
		for _, item := range items {
			result = append(result, legacyhttp.LegacyCachedImage{
				Name: item.Name, ViewURL: item.ViewURL,
				SizeBytes: item.SizeBytes, ModifiedAtMS: item.ModifiedAtMS,
			})
		}
	}
	if a.media != nil {
		const pageSize = 100
		for page, loaded := 1, int64(0); ; page++ {
			assets, total, err := a.media.AdminListImages(ctx, page, pageSize, "")
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				if len(result) > 0 {
					break
				}
				return nil, err
			}
			for _, asset := range assets {
				result = append(result, legacyhttp.LegacyCachedImage{
					Name:         mediaImageName(asset),
					ViewURL:      a.media.PublicImageURL(asset.ID),
					SizeBytes:    asset.SizeBytes,
					ModifiedAtMS: asset.CreatedAt.UnixMilli(),
				})
			}
			loaded += int64(len(assets))
			if loaded >= total || len(assets) == 0 {
				break
			}
		}
	}
	result = deduplicateLegacyImages(result)
	sort.SliceStable(result, func(i, j int) bool { return result[i].ModifiedAtMS > result[j].ModifiedAtMS })
	return result, nil
}

func mediaImageName(asset mediadomain.Asset) string {
	extension := map[string]string{
		"image/gif": ".gif", "image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
	}[strings.ToLower(strings.TrimSpace(asset.MIMEType))]
	return asset.ID + extension
}

func deduplicateLegacyImages(items []legacyhttp.LegacyCachedImage) []legacyhttp.LegacyCachedImage {
	result := make([]legacyhttp.LegacyCachedImage, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.ViewURL)
		if key == "" {
			key = strings.TrimSpace(item.Name)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func newLegacyVideoCacheAdapter(handler *cachehttp.Handler) legacyhttp.LegacyVideoCache {
	if handler == nil {
		return nil
	}
	return &legacyVideoCacheAdapter{handler: handler}
}

func (a *legacyVideoCacheAdapter) ListVideos() ([]legacyhttp.LegacyCachedVideo, error) {
	items, err := a.handler.ListAllItems("video")
	if err != nil {
		return nil, err
	}
	result := make([]legacyhttp.LegacyCachedVideo, 0, len(items))
	for _, item := range items {
		result = append(result, legacyhttp.LegacyCachedVideo{
			Name: item.Name, ViewURL: item.ViewURL, PostID: item.PostID, ShareLink: item.ShareLink,
			OriginalPostID: item.OriginalPostID, DisplayName: item.DisplayName,
			SizeBytes: item.SizeBytes, ModifiedAtMS: item.ModifiedAtMS,
		})
	}
	return result, nil
}

func (a *legacyVideoCacheAdapter) RenameVideo(identifier, displayName string) (legacyhttp.LegacyCachedVideo, error) {
	item, err := a.handler.RenameVideo("", identifier, identifier, displayName)
	if err != nil {
		return legacyhttp.LegacyCachedVideo{}, err
	}
	items, listErr := a.ListVideos()
	if listErr == nil {
		for _, candidate := range items {
			if candidate.PostID == item.PostID {
				candidate.DisplayName = item.DisplayName
				return candidate, nil
			}
		}
	}
	return legacyhttp.LegacyCachedVideo{
		PostID: item.PostID, ShareLink: item.ShareLink, DisplayName: item.DisplayName, ViewURL: item.ViewURL,
	}, nil
}
