package httpserver

import (
	"os"
	"path/filepath"
	"testing"

	cachehttp "github.com/chenyme/grok2api/backend/internal/transport/http/cache"
)

func TestLegacyVideoCacheAdapterRenamesByMigratedFilename(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "generated-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "video", name), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &legacyVideoCacheAdapter{handler: handler}
	item, err := adapter.RenameVideo(name, "Renamed local")
	if err != nil {
		t.Fatal(err)
	}
	if item.PostID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || item.DisplayName != "Renamed local" || item.ViewURL == "" {
		t.Fatalf("item=%#v", item)
	}
}

func TestLegacyImageCacheAdapterListsCachedImages(t *testing.T) {
	root := t.TempDir()
	handler, err := cachehttp.NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "cached-reference.jpg"
	if err := os.WriteFile(filepath.Join(root, "image", name), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := &legacyImageCacheAdapter{handler: handler}
	items, err := adapter.ListImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != name || items[0].ViewURL != "/v1/files/image/"+name {
		t.Fatalf("items=%#v", items)
	}
}
