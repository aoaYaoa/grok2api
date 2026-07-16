package media

import "testing"

func TestImageReferenceRoundTrip(t *testing.T) {
	value := ImageReference("img_reference_123")
	if value != "grok2api-media://image/img_reference_123" {
		t.Fatalf("reference = %q", value)
	}
	if id, ok := ParseImageReference(value); !ok || id != "img_reference_123" {
		t.Fatalf("id = %q, ok = %v", id, ok)
	}
	for _, invalid := range []string{"", "https://example.com/image.png", "grok2api-media://video/id", "grok2api-media://image/../secret"} {
		if _, ok := ParseImageReference(invalid); ok {
			t.Fatalf("invalid reference accepted: %q", invalid)
		}
	}
}
