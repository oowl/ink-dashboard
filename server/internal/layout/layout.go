package layout

import "strings"

type Document struct {
	Version   int                 `json:"version"`
	Artboards map[string]Artboard `json:"artboards"`
}

type Artboard struct {
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Components []Component `json:"components"`
}

type Component struct {
	ID    string            `json:"id"`
	Type  string            `json:"type"`
	X     int               `json:"x"`
	Y     int               `json:"y"`
	W     int               `json:"w"`
	H     int               `json:"h"`
	Props map[string]string `json:"props,omitempty"`
}

type ComponentDef struct {
	Type     string `json:"type"`
	DefaultW int    `json:"default_w"`
	DefaultH int    `json:"default_h"`
	MinW     int    `json:"min_w"`
	MinH     int    `json:"min_h"`
}

func ComponentDefs() []ComponentDef {
	return []ComponentDef{
		{Type: "clock", DefaultW: 220, DefaultH: 80, MinW: 120, MinH: 54},
		{Type: "weather", DefaultW: 280, DefaultH: 88, MinW: 180, MinH: 64},
		{Type: "calendar", DefaultW: 360, DefaultH: 360, MinW: 220, MinH: 180},
		{Type: "ai_usage", DefaultW: 280, DefaultH: 200, MinW: 220, MinH: 156},
		{Type: "notes", DefaultW: 280, DefaultH: 160, MinW: 180, MinH: 120},
	}
}

func DefaultDocument() Document {
	return Document{
		Version: 1,
		Artboards: map[string]Artboard{
			"portrait": {
				Width:  600,
				Height: 800,
				Components: []Component{
					{ID: "clock-1", Type: "clock", X: 24, Y: 24, W: 220, H: 80},
					{ID: "weather-1", Type: "weather", X: 288, Y: 24, W: 288, H: 80},
					{ID: "calendar-1", Type: "calendar", X: 24, Y: 128, W: 552, H: 432},
					{ID: "ai-usage-1", Type: "ai_usage", X: 24, Y: 574, W: 552, H: 202},
				},
			},
			"landscape": {
				Width:  800,
				Height: 600,
				Components: []Component{
					{ID: "clock-1", Type: "clock", X: 24, Y: 24, W: 220, H: 80},
					{ID: "weather-1", Type: "weather", X: 512, Y: 24, W: 264, H: 80},
					{ID: "calendar-1", Type: "calendar", X: 24, Y: 128, W: 428, H: 448},
					{ID: "ai-usage-1", Type: "ai_usage", X: 466, Y: 128, W: 310, H: 448},
				},
			},
		},
	}
}

func IsZero(doc Document) bool {
	return doc.Version == 0 && len(doc.Artboards) == 0
}

func NormalizeDocument(doc Document) Document {
	if IsZero(doc) {
		return DefaultDocument()
	}
	if doc.Version <= 0 {
		doc.Version = 1
	}
	if doc.Artboards == nil {
		doc.Artboards = map[string]Artboard{}
	}

	defaults := DefaultDocument()
	for key, fallback := range defaults.Artboards {
		board, ok := doc.Artboards[key]
		if !ok {
			board = fallback
		}
		doc.Artboards[key] = NormalizeArtboard(board, fallback)
	}
	return doc
}

func ArtboardFor(doc Document, key string, width, height int) Artboard {
	doc = NormalizeDocument(doc)
	board, ok := doc.Artboards[key]
	if !ok {
		board = doc.Artboards["portrait"]
	}
	if board.Width <= 0 {
		board.Width = width
	}
	if board.Height <= 0 {
		board.Height = height
	}
	return NormalizeArtboard(board, Artboard{Width: width, Height: height})
}

func NormalizeArtboard(board, fallback Artboard) Artboard {
	if board.Width <= 0 {
		board.Width = fallback.Width
	}
	if board.Height <= 0 {
		board.Height = fallback.Height
	}

	defs := componentDefMap()
	seen := map[string]bool{}
	components := make([]Component, 0, len(board.Components))
	for idx, component := range board.Components {
		component.Type = strings.ToLower(strings.TrimSpace(component.Type))
		def, ok := defs[component.Type]
		if !ok {
			continue
		}
		component.ID = strings.TrimSpace(component.ID)
		if component.ID == "" || seen[component.ID] {
			component.ID = component.Type + "-" + itoa(idx+1)
		}
		seen[component.ID] = true
		if component.W <= 0 {
			component.W = def.DefaultW
		}
		if component.H <= 0 {
			component.H = def.DefaultH
		}
		component.W = clamp(component.W, def.MinW, board.Width)
		component.H = clamp(component.H, def.MinH, board.Height)
		component.X = clamp(component.X, 0, max(0, board.Width-component.W))
		component.Y = clamp(component.Y, 0, max(0, board.Height-component.H))
		component.Props = normalizeProps(component.Props)
		components = append(components, component)
	}
	board.Components = components
	return board
}

func normalizeProps(props map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range props {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func componentDefMap() map[string]ComponentDef {
	out := map[string]ComponentDef{}
	for _, def := range ComponentDefs() {
		out[def.Type] = def
	}
	return out
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
