package render

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ink-dashboard/server/internal/config"
	"ink-dashboard/server/internal/dashboard"
	"ink-dashboard/server/internal/layout"
)

const svgFontFamily = "'Noto Sans CJK SC', 'Microsoft YaHei', 'PingFang SC', 'WenQuanYi Micro Hei', Arial, Helvetica, sans-serif"

type Renderer struct {
	cfg config.Config
}

type RenderRequest struct {
	Width       int
	Height      int
	Orientation string
	Language    string
	Layout      layout.Document
	LocalClock  bool
	Snapshot    dashboard.Snapshot
}

type RenderedScreen struct {
	Filename string
	Path     string
	SVG      string
}

func NewRenderer(cfg config.Config) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) RenderPNG(req RenderRequest) (RenderedScreen, error) {
	if req.Width <= 0 {
		req.Width = r.cfg.DefaultWidth
	}
	if req.Height <= 0 {
		req.Height = r.cfg.DefaultHeight
	}
	if req.Orientation == "" {
		req.Orientation = r.cfg.DefaultOrient
	}
	if req.Language == "" {
		req.Language = r.cfg.Language
	}
	if layout.IsZero(req.Layout) {
		req.Layout = r.cfg.Layout
	}

	svg := renderSVG(req)
	sum := sha1.Sum([]byte(svg))
	hash := hex.EncodeToString(sum[:])[:12]
	filename := fmt.Sprintf("inkdash-%dx%d-%s.png", req.Width, req.Height, hash)
	path := filepath.Join(r.cfg.ScreensDir, filename)

	if _, err := os.Stat(path); err == nil {
		return RenderedScreen{Filename: filename, Path: path, SVG: svg}, nil
	}

	if err := os.MkdirAll(r.cfg.ScreensDir, 0o755); err != nil {
		return RenderedScreen{}, err
	}

	svgFile, err := os.CreateTemp("", "inkdash-*.svg")
	if err != nil {
		return RenderedScreen{}, err
	}
	defer os.Remove(svgFile.Name())

	if _, err := svgFile.WriteString(svg); err != nil {
		svgFile.Close()
		return RenderedScreen{}, err
	}
	if err := svgFile.Close(); err != nil {
		return RenderedScreen{}, err
	}

	cmd := exec.Command(r.cfg.RSVGConvertPath, "-w", fmt.Sprint(req.Width), "-h", fmt.Sprint(req.Height), "-o", path, svgFile.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		return RenderedScreen{}, fmt.Errorf("rsvg-convert failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	cleanupOldScreens(r.cfg.ScreensDir, 48*time.Hour)

	return RenderedScreen{Filename: filename, Path: path, SVG: svg}, nil
}

func (r *Renderer) PreviewSVG(req RenderRequest) string {
	if req.Width <= 0 {
		req.Width = r.cfg.DefaultWidth
	}
	if req.Height <= 0 {
		req.Height = r.cfg.DefaultHeight
	}
	if req.Orientation == "" {
		req.Orientation = r.cfg.DefaultOrient
	}
	if req.Language == "" {
		req.Language = r.cfg.Language
	}
	if layout.IsZero(req.Layout) {
		req.Layout = r.cfg.Layout
	}
	return renderSVG(req)
}

func renderSVG(req RenderRequest) string {
	language := config.NormalizeLanguage(req.Language)
	width := req.Width
	height := req.Height
	landscapeCanvas := req.Orientation == "landscape" || (req.Orientation == "auto" && width > height)
	rotated := req.Orientation == "rotated"

	layoutW := width
	layoutH := height
	transformOpen := ""
	transformClose := ""
	if rotated {
		layoutW = height
		layoutH = width
		transformOpen = fmt.Sprintf(`<g transform="translate(%d 0) rotate(90)">`, width)
		transformClose = `</g>`
		landscapeCanvas = true
	}

	if landscapeCanvas && !rotated {
		layoutW = width
		layoutH = height
	}

	now := req.Snapshot.GeneratedAt
	if now.IsZero() {
		now = time.Now()
	}

	cardRadius := 0

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	b.WriteString(`<rect width="100%" height="100%" fill="#f8f8f3"/>`)
	b.WriteString(transformOpen)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="#f8f8f3"/>`, layoutW, layoutH)

	renderLayoutComponents(&b, req, layoutW, layoutH, landscapeCanvas, cardRadius, language, now)

	b.WriteString(transformClose)
	b.WriteString(`</svg>`)
	return b.String()
}

func renderLayoutComponents(b *strings.Builder, req RenderRequest, layoutW, layoutH int, landscapeCanvas bool, radius int, language string, now time.Time) {
	key := "portrait"
	if landscapeCanvas {
		key = "landscape"
	}
	board := layout.ArtboardFor(req.Layout, key, layoutW, layoutH)
	scaleX := float64(layoutW) / float64(board.Width)
	scaleY := float64(layoutH) / float64(board.Height)

	for _, component := range board.Components {
		x := scaled(component.X, scaleX)
		y := scaled(component.Y, scaleY)
		w := scaled(component.W, scaleX)
		h := scaled(component.H, scaleY)
		renderLayoutComponent(b, component, x, y, w, h, radius, req, language, now)
	}
}

func renderLayoutComponent(b *strings.Builder, component layout.Component, x, y, w, h, radius int, req RenderRequest, language string, now time.Time) {
	switch component.Type {
	case "clock":
		if !req.LocalClock {
			renderClockWidget(
				b,
				x,
				y,
				h,
				now,
				language,
				componentProp(component.Props, "format", "15:04"),
				componentBoolProp(component.Props, "show_date", true),
			)
		}
	case "weather":
		renderWeatherWidget(
			b,
			x,
			y,
			w,
			h,
			req.Snapshot.Weather,
			componentBoolProp(component.Props, "show_condition", true),
			componentBoolProp(component.Props, "show_high_low", true),
			componentBoolProp(component.Props, "show_meta", true),
		)
	case "ai_usage":
		renderUsage(
			b,
			x,
			y,
			w,
			h,
			radius,
			req.Snapshot.Usage,
			language,
			componentProp(component.Props, "title", label("ai_usage", language)),
			componentIntProp(component.Props, "max_items", 0),
		)
	case "notes":
		renderNotes(
			b,
			x,
			y,
			w,
			h,
			radius,
			req.Snapshot.Notes,
			componentProp(component.Props, "title", label("notes", language)),
			componentIntProp(component.Props, "max_items", 0),
		)
	default:
		renderSchedule(
			b,
			x,
			y,
			w,
			h,
			radius,
			req.Snapshot.Events,
			language,
			componentProp(component.Props, "title", label("calendar", language)),
			componentIntProp(component.Props, "max_items", 0),
		)
	}
}

func scaled(value int, scale float64) int {
	return int(math.Round(float64(value) * scale))
}

func componentProp(props map[string]string, key, fallback string) string {
	value := strings.TrimSpace(props[key])
	if value == "" {
		return fallback
	}
	return value
}

func componentBoolProp(props map[string]string, key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(props[key]))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func componentIntProp(props map[string]string, key string, fallback int) int {
	value := strings.TrimSpace(props[key])
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func renderClockWidget(b *strings.Builder, x, y, h int, now time.Time, language string, format string, showDate bool) {
	timeSize := clamp(h*13/20, 26, 64)
	dateSize := clamp(h/5, 12, 18)
	text(b, x, y+timeSize, now.Format(format), timeSize, 800)
	if showDate {
		text(b, x+2, y+timeSize+dateSize+12, headerDate(now, language), dateSize, 500)
	}
}

func renderWeatherWidget(b *strings.Builder, x, y, w, h int, weather dashboard.Weather, showCondition, showHighLow, showMeta bool) {
	right := x + w
	conditionSize := clamp(h/4, 13, 20)
	tempSize := clamp(h*9/20, 24, 42)
	metaSize := clamp(h/6, 11, 14)

	if showCondition {
		textAnchor(b, right, y+conditionSize+4, fitText(weather.Condition, w, conditionSize), conditionSize, 650, "end")
	}
	temp := fitText(weather.Temperature, w, tempSize)
	tempW := estimatedTextWidth(temp, tempSize)
	if high, low := highLowLines(weather.HighLow); showHighLow && high != "" {
		labelRight := right - tempW - 8
		if labelRight > x+70 {
			highLowSize := clamp(h/6, 11, 14)
			textAnchor(b, labelRight, y+conditionSize+tempSize/2, fitText(high, 72, highLowSize), highLowSize, 650, "end")
			if low != "" {
				textAnchor(b, labelRight, y+conditionSize+tempSize/2+highLowSize+6, fitText(low, 72, highLowSize), highLowSize, 650, "end")
			}
		}
	}
	textAnchor(b, right, y+conditionSize+tempSize+8, temp, tempSize, 800, "end")
	if showMeta {
		boundedTextAnchor(b, right, y+h-4, w, weatherMeta(weather), metaSize, 500, "end")
	}
}

func highLowLines(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	high, low, ok := strings.Cut(value, "/")
	if !ok {
		return value, ""
	}
	return strings.TrimSpace(high), strings.TrimSpace(low)
}

func weatherMeta(weather dashboard.Weather) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(weather.Location) != "" {
		parts = append(parts, strings.TrimSpace(weather.Location))
	}
	if strings.TrimSpace(weather.Wind) != "" {
		parts = append(parts, strings.TrimSpace(weather.Wind))
	}
	return strings.Join(parts, " · ")
}

func headerDate(t time.Time, language string) string {
	if config.IsChinese(language) {
		return fmt.Sprintf("%d月%02d日 %s · 更新 %s", int(t.Month()), t.Day(), zhWeekday(t.Weekday()), t.Format("15:04:05"))
	}
	return t.Format("Mon, Jan 02") + " · Updated " + t.Format("15:04:05")
}

func label(key, language string) string {
	if config.IsChinese(language) {
		switch key {
		case "calendar":
			return "日程"
		case "weather":
			return "天气"
		case "ai_usage":
			return "AI 用量"
		case "notes":
			return "备注"
		}
	}
	switch key {
	case "calendar":
		return "Calendar"
	case "weather":
		return "Weather"
	case "ai_usage":
		return "AI Usage"
	case "notes":
		return "Notes"
	default:
		return key
	}
}

func zhWeekday(day time.Weekday) string {
	names := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return names[int(day)]
}

func renderSchedule(b *strings.Builder, x, y, w, h, radius int, events []dashboard.Event, language string, title string, maxItems int) {
	card(b, x, y, w, h, radius, title)
	yy := y + 62
	innerX := x + 20
	innerRight := x + w - 20
	timeW := 108
	gap := 12
	timeSize := 18
	titleSize := 20
	metaSize := 14
	if w < 460 {
		timeW = 96
		gap = 10
		timeSize = 16
		titleSize = 18
		metaSize = 13
	}
	titleX := innerX + timeW + gap
	titleW := innerRight - titleX
	if titleW < 80 {
		titleW = 80
	}

	currentDay := ""
	rendered := 0
	for i, ev := range events {
		if maxItems > 0 && rendered >= maxItems {
			break
		}
		day := eventDay(ev, language)
		if day != currentDay {
			if yy+26 > y+h {
				break
			}
			boundedText(b, innerX, yy, innerRight-innerX, day, 15, 800)
			line(b, innerX, yy+10, innerRight, yy+10, 1)
			yy += 30
			currentDay = day
		}

		if yy+48 > y+h {
			break
		}

		rowTitleX := titleX
		rowTitleW := titleW
		if strings.TrimSpace(ev.Time) == "" {
			rowTitleX = innerX
			rowTitleW = innerRight - innerX
		} else {
			boundedText(b, innerX, yy, timeW, ev.Time, timeSize, 800)
		}
		boundedText(b, rowTitleX, yy, rowTitleW, ev.Title, titleSize, 650)
		boundedText(b, rowTitleX, yy+24, rowTitleW, ev.Meta, metaSize, 450)
		if i < len(events)-1 {
			line(b, innerX, yy+40, innerRight, yy+40, 1)
		}
		yy += 60
		rendered++
	}
}

func eventDay(event dashboard.Event, language string) string {
	day := strings.TrimSpace(event.Day)
	if config.IsChinese(language) && strings.EqualFold(day, "Today") {
		return "今天"
	}
	if day != "" {
		return day
	}
	if config.IsChinese(language) {
		return "今天"
	}
	return "Today"
}

func renderRightColumn(b *strings.Builder, x, y, w, h, radius int, snap dashboard.Snapshot, language string) {
	renderUsage(b, x, y, w, h, radius, snap.Usage, language, label("ai_usage", language), 0)
}

func renderWeather(b *strings.Builder, x, y, w, h, radius int, weather dashboard.Weather, language string) {
	card(b, x, y, w, h, radius, label("weather", language))
	text(b, x+20, y+72, weather.Temperature, 42, 800)
	textAnchor(b, x+w-20, y+62, fitText(weather.Condition, w-180, 20), 20, 650, "end")
	boundedText(b, x+20, y+104, w-40, weather.Location, 16, 500)
	boundedText(b, x+20, y+130, w-40, weather.HighLow+" · "+weather.Wind, 15, 450)
}

func renderUsage(b *strings.Builder, x, y, w, h, radius int, usages []dashboard.Usage, language string, title string, maxItems int) {
	card(b, x, y, w, h, radius, title)
	yy := y + 62
	for idx, usage := range usages {
		if maxItems > 0 && idx >= maxItems {
			break
		}
		rowH := 64
		if len(usage.Windows) > 0 {
			rowH = 148
		}
		if yy+rowH-6 > y+h {
			break
		}
		boundedText(b, x+20, yy, w-160, usage.Name, 18, 700)
		textAnchor(b, x+w-20, yy, fitText(usage.Primary, 150, 17), 17, 700, "end")
		if len(usage.Windows) > 0 {
			renderUsageWindows(b, x+20, yy+20, w-40, usage.Windows, language)
		} else {
			barW := w - 40
			barY := yy + 16
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="10" fill="#ffffff" stroke="#111" stroke-width="1"/>`, x+20, barY, barW)
			fillW := barW * clamp(usage.Percent, 0, 100) / 100
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="10" fill="#111"/>`, x+20, barY, fillW)
			boundedText(b, x+20, yy+42, w-40, usage.Secondary, 13, 450)
		}
		yy += rowH
	}
}

func hasWindowUsage(usages []dashboard.Usage) bool {
	for _, usage := range usages {
		if len(usage.Windows) > 0 {
			return true
		}
	}
	return false
}

func renderUsageWindows(b *strings.Builder, x, y, w int, windows []dashboard.UsageWindow, language string) {
	count := len(windows)
	if count > 2 {
		count = 2
	}
	if count <= 0 {
		return
	}
	rowH := 58
	for idx := 0; idx < count; idx++ {
		window := windows[idx]
		rowY := y + idx*rowH
		primaryW := 64
		boundedText(b, x, rowY+14, w-primaryW-8, window.Label, 13, 700)
		textAnchor(b, x+w, rowY+14, fitText(window.Primary, primaryW, 12), 12, 650, "end")
		renderBlockBar(b, x, rowY+24, w, 12, window.Percent)
		reset := usageResetText(window.Reset, language)
		boundedText(b, x, rowY+54, w, reset, 11, 450)
	}
}

func renderBlockBar(b *strings.Builder, x, y, w, h int, percent int) {
	segments := 10
	gap := 2
	segW := (w - gap*(segments-1)) / segments
	if segW < 2 {
		segW = 2
	}
	filled := (clamp(percent, 0, 100)*segments + 99) / 100
	for idx := 0; idx < segments; idx++ {
		segX := x + idx*(segW+gap)
		fill := "#ffffff"
		if idx < filled {
			fill = "#111"
		}
		fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="#111" stroke-width="1"/>`, segX, y, segW, h, fill)
	}
}

func usageResetText(reset string, language string) string {
	reset = strings.TrimSpace(reset)
	if reset == "" {
		return ""
	}
	if config.IsChinese(language) {
		return "重置 " + reset
	}
	return "reset " + reset
}

func renderNotes(b *strings.Builder, x, y, w, h, radius int, notes []string, title string, maxItems int) {
	card(b, x, y, w, h, radius, title)
	yy := y + 50
	for idx, note := range notes {
		if maxItems > 0 && idx >= maxItems {
			break
		}
		if yy+34 > y+h {
			break
		}
		boundedText(b, x+20, yy, w-40, "· "+note, 14, 450)
		yy += 28
	}
}

func card(b *strings.Builder, x, y, w, h, radius int, title string) {
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" rx="%d" fill="#ffffff" stroke="#111" stroke-width="2"/>`, x, y, w, h, radius)
	text(b, x+18, y+32, title, 18, 800)
	line(b, x+18, y+44, x+w-18, y+44, 1)
}

func text(b *strings.Builder, x, y int, value string, size int, weight int) {
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="%s" font-size="%d" font-weight="%d" fill="#111">%s</text>`, x, y, svgFontFamily, size, weight, html.EscapeString(value))
}

func textAnchor(b *strings.Builder, x, y int, value string, size int, weight int, anchor string) {
	fmt.Fprintf(b, `<text x="%d" y="%d" text-anchor="%s" font-family="%s" font-size="%d" font-weight="%d" fill="#111">%s</text>`, x, y, anchor, svgFontFamily, size, weight, html.EscapeString(value))
}

func boundedText(b *strings.Builder, x, baselineY, w int, value string, size int, weight int) {
	if w <= 0 || value == "" {
		return
	}
	boxY := baselineY - size - 2
	boxH := size + 8
	fmt.Fprintf(
		b,
		`<svg x="%d" y="%d" width="%d" height="%d" overflow="hidden"><text x="0" y="%d" font-family="%s" font-size="%d" font-weight="%d" fill="#111">%s</text></svg>`,
		x,
		boxY,
		w,
		boxH,
		size+2,
		svgFontFamily,
		size,
		weight,
		html.EscapeString(fitText(value, w, size)),
	)
}

func boundedTextAnchor(b *strings.Builder, x, baselineY, w int, value string, size int, weight int, anchor string) {
	if w <= 0 || value == "" {
		return
	}
	boxY := baselineY - size - 2
	boxH := size + 8
	fmt.Fprintf(
		b,
		`<svg x="%d" y="%d" width="%d" height="%d" overflow="hidden"><text x="%d" y="%d" text-anchor="%s" font-family="%s" font-size="%d" font-weight="%d" fill="#111">%s</text></svg>`,
		x-w,
		boxY,
		w,
		boxH,
		w,
		size+2,
		anchor,
		svgFontFamily,
		size,
		weight,
		html.EscapeString(fitText(value, w, size)),
	)
}

func line(b *strings.Builder, x1, y1, x2, y2, width int) {
	fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#111" stroke-width="%d"/>`, x1, y1, x2, y2, width)
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

func fitText(value string, maxWidth int, fontSize int) string {
	if maxWidth <= 0 || value == "" {
		return ""
	}
	if estimatedTextWidth(value, fontSize) <= maxWidth {
		return value
	}

	ellipsis := "..."
	ellipsisW := estimatedTextWidth(ellipsis, fontSize)
	budget := maxWidth - ellipsisW
	if budget <= 0 {
		return ellipsis
	}

	var out strings.Builder
	width := 0
	for _, r := range value {
		rw := estimatedRuneWidth(r, fontSize)
		if width+rw > budget {
			break
		}
		out.WriteRune(r)
		width += rw
	}

	trimmed := strings.TrimSpace(out.String())
	if trimmed == "" {
		return ellipsis
	}
	return trimmed + ellipsis
}

func estimatedTextWidth(value string, fontSize int) int {
	width := 0
	for _, r := range value {
		width += estimatedRuneWidth(r, fontSize)
	}
	return width
}

func estimatedRuneWidth(r rune, fontSize int) int {
	switch {
	case r == ' ':
		return fontSize / 3
	case r < 128:
		return fontSize * 64 / 100
	default:
		return fontSize
	}
}

func cleanupOldScreens(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "inkdash-") || !strings.HasSuffix(entry.Name(), ".png") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}
