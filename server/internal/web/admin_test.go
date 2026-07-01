package web

import (
	"strings"
	"testing"

	"ink-dashboard/server/internal/config"
	"ink-dashboard/server/internal/layout"
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
		Layout:        layout.DefaultDocument(),
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

func TestAdminDataIncludesLayoutEditorJSON(t *testing.T) {
	data := adminData(config.Config{
		DefaultWidth:  600,
		DefaultHeight: 800,
		DefaultOrient: "auto",
		Language:      "zh-CN",
		Layout:        layout.DefaultDocument(),
	}, "token", false, "")

	for _, want := range []string{`"artboards"`, `"components"`, `"calendar"`} {
		if !strings.Contains(string(data.LayoutJSON), want) {
			t.Fatalf("expected layout JSON to contain %q, got %s", want, data.LayoutJSON)
		}
	}
	if !strings.Contains(string(data.ComponentCatalogJSON), `"ai_usage"`) {
		t.Fatalf("expected component catalog JSON, got %s", data.ComponentCatalogJSON)
	}
	if data.LayoutArtboard != "portrait" {
		t.Fatalf("expected portrait artboard, got %s", data.LayoutArtboard)
	}
}

func TestAdminTemplateRendersLayoutEditor(t *testing.T) {
	data := adminData(config.Config{
		DefaultWidth:  600,
		DefaultHeight: 800,
		DefaultOrient: "auto",
		Language:      "zh-CN",
		Layout:        layout.DefaultDocument(),
	}, "token", false, "")

	var out strings.Builder
	if err := adminTemplate.Execute(&out, data); err != nil {
		t.Fatalf("expected admin template to render: %v", err)
	}
	html := out.String()
	for _, want := range []string{`id="layout-canvas"`, `id="layout-data"`, `"artboards"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected admin HTML to contain %q", want)
		}
	}
	for _, want := range []string{
		`data-component-source="weather"`,
		`配置天气`,
		`form="admin-form" name="weather_provider"`,
		`data-component-source="calendar"`,
		`配置日历`,
		`form="admin-form" name="calendar_ics_urls"`,
		`>视图<`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected source settings HTML to contain %q", want)
		}
	}
	for _, unwanted := range []string{`组件 Key`, `数据 Key`, `数据源 Key`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("expected generated key setting %q to stay hidden from the editor", unwanted)
		}
	}
}
