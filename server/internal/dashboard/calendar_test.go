package dashboard

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ink-dashboard/server/internal/config"
)

func TestParseICSAndExpandEvents(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}

	content := `BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:One-on-one\, planning
DTSTART;TZID=Asia/Shanghai:20260608T093000
DTEND;TZID=Asia/Shanghai:20260608T100000
END:VEVENT
BEGIN:VEVENT
SUMMARY:Travel day
DTSTART;VALUE=DATE:20260609
DTEND;VALUE=DATE:20260610
END:VEVENT
BEGIN:VEVENT
SUMMARY:Weekly sync
DTSTART;TZID=Asia/Shanghai:20260601T160000
DTEND;TZID=Asia/Shanghai:20260601T163000
RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=4
END:VEVENT
END:VCALENDAR`

	parsed, err := parseICS(content, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 events, got %d", len(parsed))
	}

	now := time.Date(2026, 6, 8, 8, 0, 0, 0, loc)
	windowEnd := startOfDay(now, loc).AddDate(0, 0, 7)

	var occurrences []occurrence
	for _, event := range parsed {
		occurrences = append(occurrences, expandEvent(event, now, windowEnd, loc)...)
	}

	if len(occurrences) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(occurrences))
	}

	first := occurrenceToEvent(occurrences[0], now, loc, "en")
	if first.Day != "Today" || first.Time != "09:30" || first.Title != "One-on-one, planning" || first.Meta != "30 min" {
		t.Fatalf("unexpected first event: %#v", first)
	}

	allDay := occurrenceToEvent(occurrences[1], now, loc, "en")
	if allDay.Day != "Tue 6.9" || allDay.Time != "All day" {
		t.Fatalf("unexpected all-day event: %#v", allDay)
	}

	weekly := occurrenceToEvent(occurrences[2], now, loc, "en")
	if weekly.Day != "Today" || weekly.Time != "16:00" || weekly.Title != "Weekly sync" {
		t.Fatalf("unexpected weekly event: %#v", weekly)
	}

	zhFirst := occurrenceToEvent(occurrences[0], now, loc, "zh-CN")
	if zhFirst.Day != "今天" || zhFirst.Time != "09:30" || zhFirst.Meta != "30 分钟" {
		t.Fatalf("unexpected Chinese first event: %#v", zhFirst)
	}

	zhAllDay := occurrenceToEvent(occurrences[1], now, loc, "zh-CN")
	if zhAllDay.Day != "6.9 周二" || zhAllDay.Time != "全天" || zhAllDay.Meta != "全天" {
		t.Fatalf("unexpected Chinese all-day event: %#v", zhAllDay)
	}
}

func TestFetchICSCached(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}

	requests := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Cached event
DTSTART;TZID=Asia/Shanghai:20260608T093000
DTEND;TZID=Asia/Shanghai:20260608T100000
END:VEVENT
END:VCALENDAR`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	provider := &RealProvider{
		cfg: config.Config{
			CalendarCacheSeconds: 900,
			CalendarMaxBytes:     25 * 1024 * 1024,
			HTTPTimeoutSec:       10,
		},
		client:   client,
		location: loc,
		icsCache: make(map[string]icsCacheEntry),
	}

	first, err := provider.fetchICSCached(nilContext(t), "https://calendar.example/basic.ics")
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.fetchICSCached(nilContext(t), "https://calendar.example/basic.ics")
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected cached event counts: %d %d", len(first), len(second))
	}
	if requests != 1 {
		t.Fatalf("expected one HTTP request, got %d", requests)
	}
}

func TestDedupeOccurrences(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 6, 10, 15, 0, 0, 0, loc)
	values := []occurrence{
		{title: "AI Gateway Office Hours", start: start, end: start.Add(time.Hour)},
		{title: "AI Gateway Office Hours", start: start, end: start.Add(time.Hour)},
		{title: "AI Gateway Office Hours", start: start.AddDate(0, 0, 7), end: start.AddDate(0, 0, 7).Add(time.Hour)},
	}

	deduped := dedupeOccurrences(values, loc)
	if len(deduped) != 2 {
		t.Fatalf("expected duplicate occurrence to be removed, got %d", len(deduped))
	}
	if !deduped[0].start.Equal(start) || !deduped[1].start.Equal(start.AddDate(0, 0, 7)) {
		t.Fatalf("unexpected dedupe result: %#v", deduped)
	}
}

func nilContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
