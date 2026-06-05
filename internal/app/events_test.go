package app

import (
	"testing"

	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

func TestMakeThumbKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		key          string
		contentType  string
		sourcePrefix string
		destPrefix   string
		want         string
	}{
		{
			name:         "heic converts to jpeg",
			key:          "photos/IMG_0001.HEIC",
			contentType:  storage.HEIC,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0001_heic.jpg",
		},
		{
			name:         "jpeg keeps name",
			key:          "photos/IMG_0002.jpg",
			contentType:  storage.JPEG,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0002.jpg",
		},
		{
			name:         "cr3 converts to jpeg",
			key:          "photos/IMG_0002.CR3",
			contentType:  storage.JPEG,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0002_cr3.jpg",
		},
		{
			name:         "orf converts to jpeg",
			key:          "photos/IMG_0002.ORF",
			contentType:  storage.JPEG,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0002_orf.jpg",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := makeThumbKey(tt.key, tt.contentType, tt.sourcePrefix, tt.destPrefix); got != tt.want {
				t.Fatalf("makeThumbKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
