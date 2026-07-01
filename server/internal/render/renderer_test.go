package render

import (
	"strings"
	"testing"
	"time"

	"ink-dashboard/server/internal/dashboard"
	"ink-dashboard/server/internal/layout"
)

func TestRenderSVGTruncatesLongScheduleTitles(t *testing.T) {
	longTitle := "[OLD] AI Gateway Shanghai Offsite with an extremely long planning meeting title"
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Weather: dashboard.Weather{
				Location:    "Shanghai",
				Condition:   "Cloudy",
				Temperature: "24 C",
				HighLow:     "H 27 / L 21",
				Wind:        "NE 9 km/h",
			},
			Events: []dashboard.Event{
				{Time: "Tue 06/09", Title: longTitle, Meta: "all day"},
			},
		},
	})

	if strings.Contains(svg, longTitle) {
		t.Fatal("expected long title to be truncated")
	}
	if !strings.Contains(svg, "...") {
		t.Fatal("expected truncated text to include ellipsis")
	}
}

func TestRenderSVGUsesTimeWeatherHeader(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Weather: dashboard.Weather{
				Location:    "Shanghai",
				Condition:   "Cloudy",
				Temperature: "24 C",
				HighLow:     "H 27 / L 21",
				Wind:        "NE 9 km/h",
			},
			Events: []dashboard.Event{
				{Time: "Tue 09:30", Title: "Planning", Meta: "30 min"},
			},
		},
	})

	if strings.Contains(svg, "Ink Dashboard") {
		t.Fatal("expected dashboard brand text to be removed from generated screen")
	}
	if strings.Contains(svg, ">Weather<") {
		t.Fatal("expected weather to be rendered in the header, not as a standalone card")
	}
	if !strings.Contains(svg, "01:11") || !strings.Contains(svg, "Cloudy") || !strings.Contains(svg, "24 C") {
		t.Fatal("expected time and weather values in the header")
	}
	if !strings.Contains(svg, ">H 27<") || !strings.Contains(svg, ">L 21<") {
		t.Fatal("expected high/low weather text to be split beside the temperature")
	}
	if strings.Contains(svg, "H 27 / L 21") {
		t.Fatal("expected high/low weather text not to be rendered as one inline string")
	}
	if strings.Contains(svg, "Shanghai · H 27 / L 21 · NE 9 km/h") {
		t.Fatal("expected high/low text to be separated from the bottom weather meta")
	}
}

func TestRenderSVGOmitsClockForLocalClockOverlay(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		LocalClock:  true,
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Weather: dashboard.Weather{
				Location:    "Shanghai",
				Condition:   "Cloudy",
				Temperature: "24 C",
				HighLow:     "H 27 / L 21",
				Wind:        "NE 9 km/h",
			},
		},
	})

	if strings.Contains(svg, "01:11") || strings.Contains(svg, "Updated 01:11:00") {
		t.Fatal("expected backend clock text to be omitted for local clock overlays")
	}
	if !strings.Contains(svg, "Cloudy") || !strings.Contains(svg, "24 C") {
		t.Fatal("expected weather header to remain rendered")
	}
}

func TestRenderSVGGroupsCalendarEventsByDay(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Events: []dashboard.Event{
				{Day: "Today", Time: "09:30", Title: "Today meeting", Meta: "30 min"},
				{Day: "Tue 6.9", Time: "All day", Title: "Tomorrow task", Meta: "all day"},
				{Day: "Tue 6.9", Time: "15:00", Title: "Tomorrow meeting", Meta: "1 h"},
			},
		},
	})

	if !strings.Contains(svg, "Calendar") || !strings.Contains(svg, "Today") || !strings.Contains(svg, "Tue 6.9") {
		t.Fatal("expected calendar day group headers")
	}
	if strings.Contains(svg, "Tue 15:00") || strings.Contains(svg, "Tue 06/09") {
		t.Fatal("expected non-today events to use a day separator instead of date prefixes in the time column")
	}
}

func TestRenderSVGSupportsChineseLabels(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Language:    "zh-CN",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Weather: dashboard.Weather{
				Location:    "上海",
				Condition:   "多云",
				Temperature: "24°C",
				HighLow:     "高 27 / 低 21",
				Wind:        "风 9 km/h",
			},
			Events: []dashboard.Event{
				{Day: "今天", Time: "09:30", Title: "产品例会", Meta: "30 分钟"},
				{Day: "6.9 周二", Time: "全天", Title: "离线日", Meta: "全天"},
			},
			Usage: []dashboard.Usage{
				{Name: "Codex", Primary: "剩余 68%", Secondary: "模拟限额", Percent: 68},
			},
		},
	})

	for _, want := range []string{"6月08日 周一", "日程", "今天", "6.9 周二", "AI 用量", "更新 01:11:00"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("expected Chinese SVG to contain %q", want)
		}
	}
	for _, unwanted := range []string{"KOReader 管理刷新", "备注", "600x800"} {
		if strings.Contains(svg, unwanted) {
			t.Fatalf("expected Chinese SVG not to contain %q", unwanted)
		}
	}
	if !strings.Contains(svg, "Noto Sans CJK SC") {
		t.Fatal("expected SVG to include a CJK-capable font fallback")
	}
	if !strings.Contains(svg, ">高 27<") || !strings.Contains(svg, ">低 21<") {
		t.Fatal("expected Chinese high/low weather text to be split beside the temperature")
	}
}

func TestRenderSVGShowsCodexWindows(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       800,
		Height:      600,
		Orientation: "landscape",
		Language:    "zh-CN",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Usage: []dashboard.Usage{
				{
					Name:    "Codex",
					Primary: "剩余 40%",
					Percent: 40,
					Windows: []dashboard.UsageWindow{
						{Label: "5小时", Primary: "剩余 99%", Reset: "02:30", Percent: 99},
						{Label: "每周", Primary: "剩余 40%", Reset: "6月11日 02:00", Percent: 40},
					},
				},
			},
		},
	})

	for _, want := range []string{"Codex", "5小时", "每周", "重置 02:30", "重置 6月11日 02:00"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("expected Codex window SVG to contain %q", want)
		}
	}
	if strings.Count(svg, `height="12"`) < 20 {
		t.Fatal("expected Codex windows to render block-style bars")
	}
	first := strings.Index(svg, ">5小时<")
	second := strings.Index(svg, ">每周<")
	if first < 0 || second < 0 || second <= first {
		t.Fatal("expected Codex windows to render as stacked rows")
	}
}

func TestRenderSVGOmitsNotesInLandscape(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       800,
		Height:      600,
		Orientation: "landscape",
		Language:    "en",
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 1, 11, 0, 0, time.UTC),
			Events: []dashboard.Event{
				{Day: "Today", Time: "09:30", Title: "Planning", Meta: "30 min"},
			},
			Usage: []dashboard.Usage{
				{Name: "Codex", Primary: "68% left", Secondary: "mock rate limit", Percent: 68},
			},
			Notes: []string{"This should not be rendered"},
		},
	})

	if strings.Contains(svg, ">Notes<") || strings.Contains(svg, "This should not be rendered") {
		t.Fatal("expected notes column to be omitted")
	}
	if strings.Contains(svg, "refresh managed by KOReader") || strings.Contains(svg, "800x600") {
		t.Fatal("expected footer to be omitted")
	}
}

func TestRenderSVGUsesPositionedLayoutComponents(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Language:    "en",
		Layout: layout.Document{
			Version: 1,
			Artboards: map[string]layout.Artboard{
				"portrait": {
					Width:  600,
					Height: 800,
					Components: []layout.Component{
						{ID: "notes-1", Type: "notes", X: 50, Y: 70, W: 300, H: 150},
					},
				},
			},
		},
		Snapshot: dashboard.Snapshot{
			Notes: []string{"Pinned from the canvas"},
		},
	})

	if !strings.Contains(svg, `x="50" y="70" width="300" height="150"`) {
		t.Fatal("expected notes component to render at the configured rectangle")
	}
	if !strings.Contains(svg, "Pinned from the canvas") {
		t.Fatal("expected configured notes component content")
	}
	if strings.Contains(svg, ">Calendar<") {
		t.Fatal("expected renderer not to add default calendar outside the configured layout")
	}
}

func TestRenderSVGUsesComponentProps(t *testing.T) {
	svg := renderSVG(RenderRequest{
		Width:       600,
		Height:      800,
		Orientation: "auto",
		Language:    "en",
		Layout: layout.Document{
			Version: 1,
			Artboards: map[string]layout.Artboard{
				"portrait": {
					Width:  600,
					Height: 800,
					Components: []layout.Component{
						{ID: "clock-1", Type: "clock", X: 20, Y: 20, W: 220, H: 80, Props: map[string]string{"format": "3:04 PM", "show_date": "false"}},
						{ID: "calendar-1", Type: "calendar", X: 20, Y: 120, W: 360, H: 300, Props: map[string]string{"title": "Today Only", "max_items": "1"}},
					},
				},
			},
		},
		Snapshot: dashboard.Snapshot{
			GeneratedAt: time.Date(2026, 6, 8, 13, 11, 0, 0, time.UTC),
			Events: []dashboard.Event{
				{Day: "Today", Time: "09:30", Title: "First event", Meta: "30 min"},
				{Day: "Today", Time: "10:30", Title: "Second event", Meta: "30 min"},
			},
		},
	})

	if !strings.Contains(svg, "1:11 PM") {
		t.Fatal("expected clock format prop to be used")
	}
	if strings.Contains(svg, "Updated 13:11:00") {
		t.Fatal("expected show_date=false to hide the clock date")
	}
	if !strings.Contains(svg, "Today Only") || !strings.Contains(svg, "First event") {
		t.Fatal("expected calendar title prop and first event")
	}
	if strings.Contains(svg, "Second event") {
		t.Fatal("expected max_items=1 to limit calendar events")
	}
}
