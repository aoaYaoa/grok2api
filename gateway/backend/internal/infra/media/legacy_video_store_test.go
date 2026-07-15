package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyVideoStoreStreamsProtectedVideoIntoCompatibleCache(t *testing.T) {
	root := t.TempDir()
	store, err := NewLegacyVideoStore(root)
	if err != nil {
		t.Fatal(err)
	}
	localURL, err := store.SaveVideo(
		context.Background(),
		"https://assets.grok.com/users/test/generated/123e4567-e89b-12d3-a456-426614174000/generated_video.mp4",
		"video/mp4",
		strings.NewReader("video-bytes"),
	)
	if err != nil {
		t.Fatal(err)
	}
	name := "users-test-generated-123e4567-e89b-12d3-a456-426614174000-generated_video.mp4"
	if localURL != "/v1/files/video/"+name {
		t.Fatalf("local URL = %q", localURL)
	}
	file, err := os.Open(filepath.Join(root, "video", name))
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(file)
	_ = file.Close()
	if readErr != nil || string(data) != "video-bytes" {
		t.Fatalf("stored video = %q, err=%v", data, readErr)
	}
}

func TestLegacyVideoStoreRejectsUnsupportedExtension(t *testing.T) {
	store, err := NewLegacyVideoStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveVideo(context.Background(), "https://assets.grok.com/video.exe", "video/mp4", strings.NewReader("video")); err == nil {
		t.Fatal("unsupported video extension was accepted")
	}
}
