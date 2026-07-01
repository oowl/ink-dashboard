package dashboard

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ink-dashboard/server/internal/config"
)

type RealProvider struct {
	cfg      config.Config
	client   *http.Client
	fallback *MockProvider
	location *time.Location
	icsCache map[string]icsCacheEntry
	cacheMu  sync.Mutex
	codexMu  sync.Mutex
	codex    codexCacheEntry
}

type icsCacheEntry struct {
	events    []icsEvent
	fetchedAt time.Time
	err       error
}

type codexCacheEntry struct {
	usage        Usage
	fetchedAt    time.Time
	jsonlPath    string
	jsonlModTime time.Time
	err          error
}

func NewProvider(cfg config.Config) Provider {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Printf("invalid timezone %q, using local timezone: %v", cfg.Timezone, err)
		loc = time.Local
	}

	return &RealProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.HTTPTimeoutSec) * time.Second,
		},
		fallback: NewMockProvider(),
		location: loc,
		icsCache: make(map[string]icsCacheEntry),
	}
}

func (p *RealProvider) Current() (Snapshot, error) {
	fallback, _ := p.fallback.Current()
	now := time.Now().In(p.location)
	snapshot := fallback
	snapshot.GeneratedAt = now
	snapshot.Notes = nil
	if config.IsChinese(p.cfg.Language) {
		localizeFallbackSnapshot(&snapshot)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.cfg.HTTPTimeoutSec)*time.Second)
	defer cancel()

	if p.cfg.WeatherEnabled {
		weather, err := p.FetchWeather(ctx)
		if err != nil {
			log.Printf("weather fetch failed: %v", err)
			snapshot.Notes = append(snapshot.Notes, p.note("Weather fallback: ", "天气回退: ")+shortError(err))
		} else {
			snapshot.Weather = weather
			snapshot.Notes = append(snapshot.Notes, p.note("Weather: ", "天气: ")+p.weatherProviderLabel())
		}
	} else {
		snapshot.Notes = append(snapshot.Notes, p.note("Weather: mock data", "天气: 模拟数据"))
	}

	if len(p.cfg.CalendarICSURLs) > 0 {
		events, err := p.FetchCalendar(ctx, now)
		if err != nil {
			log.Printf("calendar fetch failed: %v", err)
			snapshot.Notes = append(snapshot.Notes, p.note("Calendar fallback: ", "日历回退: ")+shortError(err))
		} else {
			snapshot.Events = events
			snapshot.Notes = append(snapshot.Notes, p.note("Calendar: Google ICS", "日历: Google ICS"))
			if len(events) == 0 {
				if config.IsChinese(p.cfg.Language) {
					snapshot.Events = []Event{{Day: "今天", Title: "没有即将到来的日程", Meta: "日历已连接"}}
				} else {
					snapshot.Events = []Event{{Day: "Today", Title: "No upcoming calendar events", Meta: "calendar connected"}}
				}
			}
		}
	} else {
		snapshot.Notes = append(snapshot.Notes, p.note("Calendar: mock data", "日历: 模拟数据"))
	}

	codexUsage, err := p.FetchCodexUsage(context.Background(), now)
	if codexUsage.Name != "" {
		snapshot.Usage = replaceUsage(snapshot.Usage, codexUsage)
	}
	if err != nil {
		log.Printf("codex usage fetch failed: %v", err)
		snapshot.Notes = append(snapshot.Notes, p.note("Codex fallback: ", "Codex 回退: ")+shortError(err))
	} else {
		snapshot.Notes = append(snapshot.Notes, p.note("Codex: JSONL", "Codex: JSONL"))
	}
	return snapshot, nil
}

func (p *RealProvider) weatherProviderLabel() string {
	switch p.cfg.WeatherProvider {
	case "caiyun":
		return "Caiyun"
	case "openmeteo":
		return "Open-Meteo"
	default:
		return p.cfg.WeatherProvider
	}
}

func (p *RealProvider) note(en, zh string) string {
	if config.IsChinese(p.cfg.Language) {
		return zh
	}
	return en
}

func replaceUsage(usages []Usage, updated Usage) []Usage {
	if updated.Name == "" {
		return usages
	}
	for idx := range usages {
		if strings.EqualFold(usages[idx].Name, updated.Name) {
			usages[idx] = updated
			return usages
		}
	}
	return append(usages, updated)
}

func localizeFallbackSnapshot(snapshot *Snapshot) {
	snapshot.Weather.Condition = "多云"
	snapshot.Weather.Temperature = "24°C"
	snapshot.Weather.HighLow = "高 27 / 低 21"
	snapshot.Weather.Wind = "东北风 9 km/h"
	for idx := range snapshot.Events {
		if strings.EqualFold(snapshot.Events[idx].Day, "Today") || snapshot.Events[idx].Day == "" {
			snapshot.Events[idx].Day = "今天"
		}
	}
	for idx := range snapshot.Usage {
		switch snapshot.Usage[idx].Name {
		case "Codex":
			snapshot.Usage[idx].Primary = "剩余 42%"
			snapshot.Usage[idx].Secondary = "模拟限额"
			snapshot.Usage[idx].Windows = []UsageWindow{
				{Label: "5小时", Primary: "剩余 47%", Reset: "02:01", Percent: 47},
				{Label: "每周", Primary: "剩余 42%", Reset: "6月11日 08:26", Percent: 42},
			}
		}
	}
}

func shortError(err error) string {
	message := err.Error()
	if len(message) > 42 {
		return message[:39] + "..."
	}
	return message
}
