package media

import "strings"

const imageReferencePrefix = "grok2api-media://image/"

func ImageReference(id string) string {
	id = strings.TrimSpace(id)
	if !validReferenceID(id) {
		return ""
	}
	return imageReferencePrefix + id
}

func ParseImageReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, imageReferencePrefix) {
		return "", false
	}
	id := strings.TrimPrefix(value, imageReferencePrefix)
	return id, validReferenceID(id)
}

func validReferenceID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
