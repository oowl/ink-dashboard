package web

import (
	"strings"
	"testing"

	"ink-dashboard/server/internal/config"
)

func TestAdminPreviewGeometryRotatesPreviewUpright(t *testing.T) {
	width, height, orientation, landscape := adminPreviewGeometry(config.Config{
		DefaultWidth:  600,
		DefaultHeight: 800,
		DefaultOrient: "rotated",
	})

	if width != 800 || height != 600 || orientation != "landscape" || !landscape {
		t.Fatalf("unexpected preview geometry: %dx%d %s landscape=%v", width, height, orientation, landscape)
	}
}

func TestAdminDataUsesLandscapePreviewURL(t *testing.T) {
	data := adminData(config.Config{
		DefaultWidth:  600,
		DefaultHeight: 800,
		DefaultOrient: "rotated",
		Language:      "zh-CN",
	}, "token", false, "")

	if !data.PreviewLandscape {
		t.Fatal("expected landscape preview flag")
	}
	for _, want := range []string{"width=800", "height=600", "orientation=landscape"} {
		if !strings.Contains(data.PreviewURL, want) {
			t.Fatalf("expected preview URL to contain %q, got %s", want, data.PreviewURL)
		}
	}
}
