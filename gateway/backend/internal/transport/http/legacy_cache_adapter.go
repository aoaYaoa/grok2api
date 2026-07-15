package httpserver

import (
	cachehttp "github.com/chenyme/grok2api/backend/internal/transport/http/cache"
	legacyhttp "github.com/chenyme/grok2api/backend/internal/transport/http/legacy"
)

type legacyVideoCacheAdapter struct {
	handler *cachehttp.Handler
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
