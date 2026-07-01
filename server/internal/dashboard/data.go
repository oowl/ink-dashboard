package dashboard

import "time"

type Snapshot struct {
	GeneratedAt time.Time
	Weather     Weather
	Events      []Event
	Usage       []Usage
	Notes       []string
}

type Weather struct {
	Location    string
	Condition   string
	Temperature string
	HighLow     string
	Wind        string
}

type Event struct {
	Day   string
	Time  string
	Title string
	Meta  string
}

type Usage struct {
	Name      string
	Primary   string
	Secondary string
	Percent   int
	Windows   []UsageWindow
}

type UsageWindow struct {
	Label   string
	Primary string
	Reset   string
	Percent int
}

type Provider interface {
	Current() (Snapshot, error)
}

type MockProvider struct{}

func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (p *MockProvider) Current() (Snapshot, error) {
	now := time.Now()
	return Snapshot{
		GeneratedAt: now,
		Weather: Weather{
			Location:    "Shanghai",
			Condition:   "Cloudy",
			Temperature: "24 C",
			HighLow:     "H 27 / L 21",
			Wind:        "NE 9 km/h",
		},
		Events: []Event{
			{Day: "Today", Time: "09:30", Title: "Daily planning", Meta: "15 min"},
			{Day: "Today", Time: "13:00", Title: "Focus block", Meta: "deep work"},
			{Day: "Today", Time: "16:30", Title: "Review dashboard", Meta: "personal"},
		},
		Usage: []Usage{
			{
				Name:      "Codex",
				Primary:   "42% left",
				Secondary: "mock rate limit",
				Percent:   42,
				Windows: []UsageWindow{
					{Label: "5h", Primary: "47% left", Reset: "02:01", Percent: 47},
					{Label: "weekly", Primary: "42% left", Reset: "08:26 on 11 Jun", Percent: 42},
				},
			},
		},
		Notes: []string{
			"Server data is mock for the first build.",
			"Replace providers with calendar, weather, and Codex sources next.",
		},
	}, nil
}
