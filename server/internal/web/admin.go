package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ink-dashboard/server/internal/config"
	"ink-dashboard/server/internal/layout"
)

type adminPageData struct {
	Config               config.Config
	Text                 adminText
	Token                string
	Saved                bool
	Error                string
	CalendarSummary      string
	CalendarFeeds        []calendarFeedView
	CalendarConfigured   bool
	CaiyunConfigured     bool
	PreviewURL           string
	PreviewLandscape     bool
	LayoutJSON           template.JS
	ComponentCatalogJSON template.JS
	LayoutArtboard       string
	CaiyunSelected       bool
	OpenSelected         bool
	AutoSelected         bool
	PortraitSelected     bool
	LandscapeSelected    bool
	RotatedSelected      bool
	EnglishSelected      bool
	ChineseSelected      bool
}

type calendarFeedView struct {
	Index       int
	Label       string
	Host        string
	Fingerprint string
}

type adminText struct {
	HTMLLang                string
	PageTitle               string
	HeaderTitle             string
	ConfigFile              string
	SavedMessage            string
	Display                 string
	PublicBaseURL           string
	PublicBaseURLHint       string
	RefreshSeconds          string
	Timezone                string
	Language                string
	PreviewWidth            string
	PreviewHeight           string
	Orientation             string
	Layout                  string
	LayoutHint              string
	ComponentLibrary        string
	View                    string
	ConfigWeather           string
	ConfigCalendar          string
	SelectedComponent       string
	DeleteComponent         string
	NoComponentSelected     string
	Weather                 string
	Provider                string
	Latitude                string
	Longitude               string
	LocationLabel           string
	CaiyunToken             string
	ConfiguredKeepToken     string
	NotConfiguredPasteToken string
	LeaveBlankKeepToken     string
	PasteCaiyunToken        string
	ClearSavedCaiyunToken   string
	CaiyunLang              string
	CaiyunUnit              string
	GoogleCalendar          string
	CalendarSecretHint      string
	ConfiguredFeeds         string
	Fingerprint             string
	DeleteOnSave            string
	AddPrivateICalURLs      string
	ICalPlaceholder         string
	CalendarHint            string
	LookaheadDays           string
	MaxEvents               string
	SaveAndReload           string
	Preview                 string
	PreviewAlt              string
	PreviewHint             string
	English                 string
	Chinese                 string
}

func (a *App) admin(w http.ResponseWriter, r *http.Request) {
	cfg, _, _ := a.current()
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if cfg.APIKey != "" && token != cfg.APIKey {
		renderAdminLogin(w, r)
		return
	}

	data := adminData(cfg, token, r.URL.Query().Get("saved") == "1", "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminTemplate.Execute(w, data)
}

func (a *App) saveAdminConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _, _ := a.current()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	if cfg.APIKey != "" && token != cfg.APIKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	caiyunToken := strings.TrimSpace(r.FormValue("caiyun_token"))
	if caiyunToken == "" {
		caiyunToken = cfg.CaiyunToken
	}
	if r.FormValue("clear_caiyun_token") == "1" {
		caiyunToken = ""
	}

	calendarURLs := mergeCalendarURLs(cfg.CalendarICSURLs, selectedCalendarDeletes(r), splitCalendarURLs(r.FormValue("calendar_ics_urls")))
	layoutDoc := cfg.Layout
	if rawLayout := strings.TrimSpace(r.FormValue("layout_json")); rawLayout != "" {
		if err := json.Unmarshal([]byte(rawLayout), &layoutDoc); err != nil {
			data := adminData(cfg, token, false, "invalid layout: "+err.Error())
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = adminTemplate.Execute(w, data)
			return
		}
		layoutDoc = layout.NormalizeDocument(layoutDoc)
	}

	settings := config.AdminSettings{
		PublicBaseURL:        strings.TrimSpace(r.FormValue("public_base_url")),
		RefreshSeconds:       formInt(r, "refresh_seconds", cfg.RefreshSeconds),
		DefaultWidth:         formInt(r, "default_width", cfg.DefaultWidth),
		DefaultHeight:        formInt(r, "default_height", cfg.DefaultHeight),
		DefaultOrient:        strings.TrimSpace(r.FormValue("default_orientation")),
		Layout:               layoutDoc,
		Timezone:             strings.TrimSpace(r.FormValue("timezone")),
		Language:             strings.TrimSpace(r.FormValue("language")),
		WeatherProvider:      strings.TrimSpace(r.FormValue("weather_provider")),
		WeatherLatitude:      formFloat(r, "weather_latitude", cfg.WeatherLatitude),
		WeatherLongitude:     formFloat(r, "weather_longitude", cfg.WeatherLongitude),
		WeatherLocation:      strings.TrimSpace(r.FormValue("weather_location")),
		CaiyunToken:          caiyunToken,
		CaiyunLang:           strings.TrimSpace(r.FormValue("caiyun_lang")),
		CaiyunUnit:           strings.TrimSpace(r.FormValue("caiyun_unit")),
		CalendarICSURLs:      calendarURLs,
		CalendarLookaheadDay: formInt(r, "calendar_lookahead_days", cfg.CalendarLookaheadDay),
		CalendarMaxEvents:    formInt(r, "calendar_max_events", cfg.CalendarMaxEvents),
	}

	next := cfg
	config.ApplyAdminSettings(&next, settings)
	next.Normalize()

	if err := config.SaveAdminSettings(next.ConfigFile, next.AdminSettings()); err != nil {
		data := adminData(cfg, token, false, err.Error())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = adminTemplate.Execute(w, data)
		return
	}

	a.setConfig(next)
	http.Redirect(w, r, "/admin?token="+url.QueryEscape(token)+"&saved=1", http.StatusSeeOther)
}

func adminData(cfg config.Config, token string, saved bool, errMsg string) adminPageData {
	text := adminTextFor(cfg.Language)
	previewWidth, previewHeight, previewOrient, previewLandscape := adminPreviewGeometry(cfg)
	preview := "/preview.svg?token=" + url.QueryEscape(token) +
		"&width=" + strconv.Itoa(previewWidth) +
		"&height=" + strconv.Itoa(previewHeight) +
		"&orientation=" + url.QueryEscape(previewOrient) +
		"&t=" + strconv.FormatInt(time.Now().Unix(), 10)
	return adminPageData{
		Config:               cfg,
		Text:                 text,
		Token:                token,
		Saved:                saved,
		Error:                errMsg,
		CalendarSummary:      secretListSummary(cfg.CalendarICSURLs, cfg.Language),
		CalendarFeeds:        calendarFeedViews(cfg.CalendarICSURLs, cfg.Language),
		CalendarConfigured:   len(cfg.CalendarICSURLs) > 0,
		CaiyunConfigured:     cfg.CaiyunToken != "",
		PreviewURL:           preview,
		PreviewLandscape:     previewLandscape,
		LayoutJSON:           adminJSON(layout.NormalizeDocument(cfg.Layout)),
		ComponentCatalogJSON: adminJSON(componentCatalog(text.HTMLLang)),
		LayoutArtboard:       previewArtboard(previewLandscape),
		CaiyunSelected:       cfg.WeatherProvider == "caiyun",
		OpenSelected:         cfg.WeatherProvider == "openmeteo",
		AutoSelected:         cfg.DefaultOrient == "auto",
		PortraitSelected:     cfg.DefaultOrient == "portrait",
		LandscapeSelected:    cfg.DefaultOrient == "landscape",
		RotatedSelected:      cfg.DefaultOrient == "rotated",
		EnglishSelected:      cfg.Language == "en",
		ChineseSelected:      config.IsChinese(cfg.Language),
	}
}

func adminPreviewGeometry(cfg config.Config) (int, int, string, bool) {
	width := cfg.DefaultWidth
	height := cfg.DefaultHeight
	orientation := cfg.DefaultOrient
	if width <= 0 {
		width = 600
	}
	if height <= 0 {
		height = 800
	}

	landscape := width > height || orientation == "landscape" || orientation == "rotated"
	if landscape && width < height {
		width, height = height, width
	}
	if orientation == "rotated" {
		orientation = "landscape"
	}
	return width, height, orientation, landscape
}

type componentCatalogItem struct {
	Type     string `json:"type"`
	Label    string `json:"label"`
	DefaultW int    `json:"default_w"`
	DefaultH int    `json:"default_h"`
	MinW     int    `json:"min_w"`
	MinH     int    `json:"min_h"`
}

func componentCatalog(htmlLang string) []componentCatalogItem {
	labels := map[string]string{
		"clock":    "Clock",
		"weather":  "Weather",
		"calendar": "Calendar",
		"ai_usage": "AI Usage",
		"notes":    "Notes",
	}
	if strings.HasPrefix(strings.ToLower(htmlLang), "zh") {
		labels = map[string]string{
			"clock":    "时间",
			"weather":  "天气",
			"calendar": "日程",
			"ai_usage": "AI 用量",
			"notes":    "备注",
		}
	}

	defs := layout.ComponentDefs()
	out := make([]componentCatalogItem, 0, len(defs))
	for _, def := range defs {
		out = append(out, componentCatalogItem{
			Type:     def.Type,
			Label:    labels[def.Type],
			DefaultW: def.DefaultW,
			DefaultH: def.DefaultH,
			MinW:     def.MinW,
			MinH:     def.MinH,
		})
	}
	return out
}

func previewArtboard(landscape bool) string {
	if landscape {
		return "landscape"
	}
	return "portrait"
}

func adminJSON(value any) template.JS {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return template.JS(raw)
}

func renderAdminLogin(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminLoginTemplate.Execute(w, nil)
}

func formInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func formFloat(r *http.Request, key string, fallback float64) float64 {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCalendarURLs(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func selectedCalendarDeletes(r *http.Request) map[int]bool {
	out := map[int]bool{}
	for _, value := range r.Form["delete_calendar_indices"] {
		idx, err := strconv.Atoi(value)
		if err == nil && idx >= 0 {
			out[idx] = true
		}
	}
	return out
}

func mergeCalendarURLs(existing []string, deleteSet map[int]bool, added []string) []string {
	var out []string
	for idx, value := range existing {
		if !deleteSet[idx] {
			out = append(out, value)
		}
	}
	out = append(out, added...)
	return out
}

func secretListSummary(values []string, language string) string {
	if config.IsChinese(language) {
		switch len(values) {
		case 0:
			return "未配置"
		case 1:
			return "已配置 1 个私密 iCal URL"
		default:
			return "已配置 " + strconv.Itoa(len(values)) + " 个私密 iCal URL"
		}
	}
	if len(values) == 0 {
		return "Not configured"
	}
	if len(values) == 1 {
		return "1 private iCal URL configured"
	}
	return strconv.Itoa(len(values)) + " private iCal URLs configured"
}

func calendarFeedViews(values []string, language string) []calendarFeedView {
	views := make([]calendarFeedView, 0, len(values))
	for idx, value := range values {
		host := "private feed"
		labelPrefix := "Calendar "
		if config.IsChinese(language) {
			host = "私密日历源"
			labelPrefix = "日历 "
		}
		if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
		sum := sha256.Sum256([]byte(value))
		views = append(views, calendarFeedView{
			Index:       idx,
			Label:       labelPrefix + strconv.Itoa(idx+1),
			Host:        host,
			Fingerprint: hex.EncodeToString(sum[:])[:10],
		})
	}
	return views
}

func adminTextFor(language string) adminText {
	if config.IsChinese(language) {
		return adminText{
			HTMLLang:                "zh-CN",
			PageTitle:               "Ink Dashboard 管理后台",
			HeaderTitle:             "Ink Dashboard 管理后台",
			ConfigFile:              "配置文件",
			SavedMessage:            "已保存，运行时配置已重新加载。",
			Display:                 "显示",
			PublicBaseURL:           "公开访问地址",
			PublicBaseURLHint:       "用于返回给 KOReader 的 image_url。留空时使用当前请求的主机名。",
			RefreshSeconds:          "刷新秒数",
			Timezone:                "时区",
			Language:                "语言",
			PreviewWidth:            "预览宽度",
			PreviewHeight:           "预览高度",
			Orientation:             "方向",
			Layout:                  "布局画布",
			LayoutHint:              "拖动组件调整位置，拖右下角调整大小。保存后，Kindle 渲染会使用这份布局。",
			ComponentLibrary:        "组件库",
			View:                    "视图",
			ConfigWeather:           "配置天气",
			ConfigCalendar:          "配置日历",
			SelectedComponent:       "选中组件",
			DeleteComponent:         "删除组件",
			NoComponentSelected:     "未选中组件",
			Weather:                 "天气",
			Provider:                "数据源",
			Latitude:                "纬度",
			Longitude:               "经度",
			LocationLabel:           "地点名称",
			CaiyunToken:             "彩云 token",
			ConfiguredKeepToken:     "已配置。留空会保留已保存的 token。",
			NotConfiguredPasteToken: "未配置。粘贴 token 以启用彩云天气。",
			LeaveBlankKeepToken:     "留空保留现有 token",
			PasteCaiyunToken:        "粘贴彩云 token",
			ClearSavedCaiyunToken:   "清除已保存的彩云 token",
			CaiyunLang:              "彩云语言",
			CaiyunUnit:              "彩云单位",
			GoogleCalendar:          "Google 日历",
			CalendarSecretHint:      "已保存的 URL 不会显示，因为每个 URL 都包含私密 token。",
			ConfiguredFeeds:         "已配置日历源",
			Fingerprint:             "指纹",
			DeleteOnSave:            "保存时删除",
			AddPrivateICalURLs:      "添加私密 iCal URL",
			ICalPlaceholder:         "粘贴新的 Google 私密 iCal URL，每行一个",
			CalendarHint:            "每行一个 URL。未勾选删除的现有日历源会保留；新粘贴的 URL 会保存，但不会回显到表单里。",
			LookaheadDays:           "向后查看天数",
			MaxEvents:               "最多事件数",
			SaveAndReload:           "保存并重新加载",
			Preview:                 "预览",
			PreviewAlt:              "Dashboard 预览",
			PreviewHint:             "预览使用当前已保存的运行时配置。保存后会刷新。",
			English:                 "English",
			Chinese:                 "中文",
		}
	}
	return adminText{
		HTMLLang:                "en",
		PageTitle:               "Ink Dashboard Admin",
		HeaderTitle:             "Ink Dashboard Admin",
		ConfigFile:              "Configuration file",
		SavedMessage:            "Saved. Runtime configuration has been reloaded.",
		Display:                 "Display",
		PublicBaseURL:           "Public base URL",
		PublicBaseURLHint:       "Used for image_url returned to KOReader. Leave empty to use request host.",
		RefreshSeconds:          "Refresh seconds",
		Timezone:                "Timezone",
		Language:                "Language",
		PreviewWidth:            "Preview width",
		PreviewHeight:           "Preview height",
		Orientation:             "Orientation",
		Layout:                  "Layout Canvas",
		LayoutHint:              "Drag components to move them. Drag the bottom-right handle to resize. Saved layouts are used by Kindle rendering.",
		ComponentLibrary:        "Components",
		View:                    "View",
		ConfigWeather:           "Config Weather",
		ConfigCalendar:          "Config Calendar",
		SelectedComponent:       "Selected component",
		DeleteComponent:         "Delete component",
		NoComponentSelected:     "No component selected",
		Weather:                 "Weather",
		Provider:                "Provider",
		Latitude:                "Latitude",
		Longitude:               "Longitude",
		LocationLabel:           "Location label",
		CaiyunToken:             "Caiyun token",
		ConfiguredKeepToken:     "Configured. Leave blank to keep the saved token.",
		NotConfiguredPasteToken: "Not configured. Paste a token to enable Caiyun.",
		LeaveBlankKeepToken:     "leave blank to keep existing token",
		PasteCaiyunToken:        "paste Caiyun token",
		ClearSavedCaiyunToken:   "Clear saved Caiyun token",
		CaiyunLang:              "Caiyun lang",
		CaiyunUnit:              "Caiyun unit",
		GoogleCalendar:          "Google Calendar",
		CalendarSecretHint:      "Saved URLs are not shown because each one contains a secret token.",
		ConfiguredFeeds:         "Configured feeds",
		Fingerprint:             "fingerprint",
		DeleteOnSave:            "Delete on save",
		AddPrivateICalURLs:      "Add private iCal URL(s)",
		ICalPlaceholder:         "Paste new Google private iCal URL(s), one per line",
		CalendarHint:            "One URL per line. Existing feeds stay configured unless checked for deletion. Newly pasted URLs are saved but never echoed back into this form.",
		LookaheadDays:           "Lookahead days",
		MaxEvents:               "Max events",
		SaveAndReload:           "Save And Reload",
		Preview:                 "Preview",
		PreviewAlt:              "Dashboard preview",
		PreviewHint:             "Preview uses the current saved runtime config. Save changes to refresh it.",
		English:                 "English",
		Chinese:                 "中文",
	}
}

var adminLoginTemplate = template.Must(template.New("admin-login").Parse(`<!doctype html>
	<html lang="zh-CN">
	<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Ink Dashboard 管理后台</title>
	<style>
	body{font-family:"Noto Sans CJK SC","Microsoft YaHei","PingFang SC","WenQuanYi Micro Hei",Arial,sans-serif;margin:0;background:#f6f6f1;color:#111}
	main{max-width:360px;margin:12vh auto;padding:24px}
	label{display:block;font-weight:700;margin-bottom:8px}
	input{box-sizing:border-box;width:100%;padding:10px;border:1px solid #222;background:#fff;font-size:15px}
button{margin-top:12px;padding:10px 14px;border:1px solid #111;background:#111;color:#fff;font-weight:700}
</style>
</head>
	<body>
	<main>
	<h1>Ink Dashboard 管理后台</h1>
	<form action="/admin" method="get">
	<label for="token">管理口令 / Admin token</label>
	<input id="token" name="token" type="password" autocomplete="current-password">
	<button type="submit">打开管理后台</button>
	</form>
	</main>
	</body>
	</html>`))

var adminTemplate = template.Must(template.New("admin").Parse(`<!doctype html>
	<html lang="{{.Text.HTMLLang}}">
	<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.Text.PageTitle}}</title>
	<style>
	:root{color-scheme:light}
	body{font-family:"Noto Sans CJK SC","Microsoft YaHei","PingFang SC","WenQuanYi Micro Hei",Arial,sans-serif;margin:0;background:#f6f6f1;color:#111}
	header{border-bottom:2px solid #111;padding:18px 24px;background:#fff}
	h1{font-size:24px;margin:0}
.sub{font-size:13px;margin-top:4px;color:#444}
main{display:grid;grid-template-columns:minmax(300px,360px) minmax(1100px,1fr);grid-template-areas:"settings layout" "settings preview";gap:24px;padding:24px;align-items:start}
.settings-form{grid-area:settings}
section{border:1px solid #111;background:#fff;padding:16px;margin-bottom:16px}
h2{font-size:16px;margin:0 0 12px}
label{display:block;font-size:13px;font-weight:700;margin:12px 0 5px}
input,select,textarea{box-sizing:border-box;width:100%;padding:8px;border:1px solid #333;background:#fff;font-size:14px}
textarea{min-height:92px;font-family:monospace}
.row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.actions{position:sticky;bottom:0;background:#fff;border-top:1px solid #111;padding-top:12px}
button{padding:10px 14px;border:1px solid #111;background:#111;color:#fff;font-weight:700}
.ok{border:1px solid #111;background:#e8f2e8;padding:8px;margin-bottom:12px}
.err{border:1px solid #111;background:#f4dddd;padding:8px;margin-bottom:12px}
.secret{border:1px dashed #333;background:#fafafa;padding:8px;font-size:13px}
.feed{display:grid;grid-template-columns:1fr auto;gap:10px;align-items:center;border:1px solid #333;padding:10px;margin-top:8px;background:#fafafa}
.feed strong{display:block;font-size:13px}
.feed span{display:block;font-size:12px;color:#555;margin-top:2px}
.delete-toggle{display:inline-flex;align-items:center;gap:6px;border:1px solid #111;padding:7px 9px;background:#fff;font-size:12px;font-weight:700;white-space:nowrap}
.delete-toggle input{width:auto;margin:0}
.feed:has(.delete-toggle input:checked){background:#f4dddd;border-width:2px}
.feed:has(.delete-toggle input:checked) .delete-toggle{background:#111;color:#fff}
.layout-panel{grid-area:layout}
.layout-editor{display:grid;grid-template-columns:132px minmax(360px,680px) minmax(620px,1fr);gap:14px;align-items:start}
.palette{display:flex;flex-direction:column;gap:8px}
.palette button{width:100%;background:#fff;color:#111}
.layout-workspace{min-width:0}
.layout-canvas{position:relative;width:100%;max-width:680px;aspect-ratio:600/800;background-color:#f8f8f3;background-image:linear-gradient(to right,rgba(17,17,17,.12) 1px,transparent 1px),linear-gradient(to bottom,rgba(17,17,17,.12) 1px,transparent 1px);background-size:var(--grid-x,4%) var(--grid-y,3%),var(--grid-x,4%) var(--grid-y,3%);border:2px solid #111;overflow:hidden;touch-action:none}
.layout-node{position:absolute;box-sizing:border-box;border:2px solid #111;background:#fff;cursor:move;user-select:none;overflow:hidden;font-size:12px;font-weight:800;display:flex;align-items:flex-start;justify-content:space-between;padding:7px;color:#111}
.layout-node.clock,.layout-node.weather{background:#f8f8f3}
.layout-node.selected{outline:3px solid #111;outline-offset:2px;background:#e8f2e8}
.layout-node small{font-size:10px;font-weight:600;color:#555;margin-top:2px}
.snap-guide{position:absolute;z-index:4;pointer-events:none;background:#111}
.snap-guide.vertical{top:0;bottom:0;width:2px;transform:translateX(-1px)}
.snap-guide.horizontal{left:0;right:0;height:2px;transform:translateY(-1px)}
.resize-handle{position:absolute;right:0;bottom:0;width:18px;height:18px;border-left:2px solid #111;border-top:2px solid #111;background:#fff;cursor:nwse-resize}
.layout-props{border-left:1px solid #111;padding-left:14px;align-self:start;min-width:0}
.layout-props strong{display:block;font-size:13px;margin-bottom:6px}
.layout-props button{margin-top:8px;background:#fff;color:#111}
.config-panels{display:grid;grid-template-columns:minmax(260px,1fr) minmax(300px,1.15fr);gap:14px;align-items:start}
.config-box{border:1px solid #111;background:#fff;padding:12px;min-width:0}
.config-box h3{font-size:16px;margin:0 0 10px}
.component-kind{font-size:12px;color:#555;margin:-4px 0 10px}
.prop-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px}
.prop-grid label{margin-top:0}
.component-settings{display:grid;grid-template-columns:1fr;gap:8px;border-top:1px solid #111;margin-top:10px;padding-top:10px}
.component-settings label{margin-top:0}
.component-settings .check{display:flex;align-items:center;gap:8px;border:1px solid #333;padding:8px;background:#fafafa}
.component-settings .check input{width:auto;margin:0}
.source-settings[hidden]{display:none}
	.preview{grid-area:preview}
	.preview-frame{overflow:auto}
	.preview-frame img{width:min(100%,600px);height:auto;border:1px solid #111;background:#fff}
	.preview-frame.landscape img{width:min(100%,900px)}
.hint{font-size:12px;color:#555;margin-top:4px;line-height:1.4}
@media(max-width:1280px){main{grid-template-columns:1fr;grid-template-areas:"layout" "settings" "preview"}.layout-editor{grid-template-columns:1fr}.layout-props{border-left:0;border-top:1px solid #111;padding-left:0;padding-top:10px}.config-panels{grid-template-columns:1fr}.palette{display:grid;grid-template-columns:repeat(2,1fr)}}
</style>
</head>
	<body>
	<header>
	<h1>{{.Text.HeaderTitle}}</h1>
	<div class="sub">{{.Text.ConfigFile}}: {{.Config.ConfigFile}}</div>
	</header>
	<main>
	<form id="admin-form" class="settings-form" method="post" action="/admin/config">
	<input type="hidden" name="token" value="{{.Token}}">
	{{if .Saved}}<div class="ok">{{.Text.SavedMessage}}</div>{{end}}
	{{if .Error}}<div class="err">{{.Error}}</div>{{end}}
	
	<section>
	<h2>{{.Text.Display}}</h2>
	<label>{{.Text.PublicBaseURL}}</label>
	<input name="public_base_url" value="{{.Config.PublicBaseURL}}" placeholder="http://192.168.1.20:8787">
	<div class="hint">{{.Text.PublicBaseURLHint}}</div>
	<div class="row">
	<div><label>{{.Text.RefreshSeconds}}</label><input name="refresh_seconds" type="number" min="60" value="{{.Config.RefreshSeconds}}"></div>
	<div><label>{{.Text.Timezone}}</label><input name="timezone" value="{{.Config.Timezone}}"></div>
	</div>
	<div class="row">
	<div><label>{{.Text.PreviewWidth}}</label><input name="default_width" type="number" value="{{.Config.DefaultWidth}}"></div>
	<div><label>{{.Text.PreviewHeight}}</label><input name="default_height" type="number" value="{{.Config.DefaultHeight}}"></div>
	</div>
	<label>{{.Text.Language}}</label>
	<select name="language">
	<option value="zh-CN" {{if .ChineseSelected}}selected{{end}}>{{.Text.Chinese}}</option>
	<option value="en" {{if .EnglishSelected}}selected{{end}}>{{.Text.English}}</option>
	</select>
	<label>{{.Text.Orientation}}</label>
	<select name="default_orientation">
	<option value="auto" {{if .AutoSelected}}selected{{end}}>auto</option>
	<option value="portrait" {{if .PortraitSelected}}selected{{end}}>portrait</option>
<option value="landscape" {{if .LandscapeSelected}}selected{{end}}>landscape</option>
<option value="rotated" {{if .RotatedSelected}}selected{{end}}>rotated</option>
</select>
	</section>

	<div class="actions">
	<button type="submit">{{.Text.SaveAndReload}}</button>
	</div>
	</form>

	<section class="layout-panel">
	<h2>{{.Text.Layout}}</h2>
	<input id="layout-json" type="hidden" name="layout_json" form="admin-form">
	<div id="layout-editor" class="layout-editor" data-artboard="{{.LayoutArtboard}}">
	<div>
	<strong>{{.Text.ComponentLibrary}}</strong>
	<div id="component-palette" class="palette"></div>
	</div>
	<div class="layout-workspace">
	<div id="layout-canvas" class="layout-canvas"></div>
	</div>
	<div class="layout-props">
	<div id="component-empty" class="hint">{{.Text.NoComponentSelected}}</div>
	<div id="component-props" hidden>
	<div class="config-panels">
	<div class="view-settings config-box">
	<h3>{{.Text.View}}</h3>
	<div id="component-title" class="component-kind">{{.Text.SelectedComponent}}</div>
	<div class="prop-grid">
	<label>x<input id="prop-x" type="number"></label>
	<label>y<input id="prop-y" type="number"></label>
	<label>w<input id="prop-w" type="number"></label>
	<label>h<input id="prop-h" type="number"></label>
	</div>
	<div id="component-settings" class="component-settings"></div>
	<button id="delete-component" type="button">{{.Text.DeleteComponent}}</button>
	</div>
	<div class="source-settings config-box" data-component-source="weather" hidden>
	<h3>{{.Text.ConfigWeather}}</h3>
	<label>{{.Text.Provider}}</label>
	<select form="admin-form" name="weather_provider">
	<option value="openmeteo" {{if .OpenSelected}}selected{{end}}>Open-Meteo</option>
	<option value="caiyun" {{if .CaiyunSelected}}selected{{end}}>Caiyun</option>
	</select>
	<div class="row">
	<div><label>{{.Text.Latitude}}</label><input form="admin-form" name="weather_latitude" value="{{printf "%.6f" .Config.WeatherLatitude}}"></div>
	<div><label>{{.Text.Longitude}}</label><input form="admin-form" name="weather_longitude" value="{{printf "%.6f" .Config.WeatherLongitude}}"></div>
	</div>
	<label>{{.Text.LocationLabel}}</label>
	<input form="admin-form" name="weather_location" value="{{.Config.WeatherLocation}}" placeholder="Shanghai">
	<label>{{.Text.CaiyunToken}}</label>
	<div class="secret">{{if .CaiyunConfigured}}{{.Text.ConfiguredKeepToken}}{{else}}{{.Text.NotConfiguredPasteToken}}{{end}}</div>
	<input form="admin-form" name="caiyun_token" type="password" value="" placeholder="{{if .CaiyunConfigured}}{{.Text.LeaveBlankKeepToken}}{{else}}{{.Text.PasteCaiyunToken}}{{end}}" autocomplete="off">
	{{if .CaiyunConfigured}}<label><input form="admin-form" style="width:auto" type="checkbox" name="clear_caiyun_token" value="1"> {{.Text.ClearSavedCaiyunToken}}</label>{{end}}
	<div class="row">
	<div><label>{{.Text.CaiyunLang}}</label><input form="admin-form" name="caiyun_lang" value="{{.Config.CaiyunLang}}"></div>
	<div><label>{{.Text.CaiyunUnit}}</label><input form="admin-form" name="caiyun_unit" value="{{.Config.CaiyunUnit}}"></div>
	</div>
	</div>
	<div class="source-settings config-box" data-component-source="calendar" hidden>
	<h3>{{.Text.ConfigCalendar}}</h3>
	<div class="secret">{{.CalendarSummary}}. {{.Text.CalendarSecretHint}}</div>
	{{if .CalendarConfigured}}
	<label>{{.Text.ConfiguredFeeds}}</label>
	{{range .CalendarFeeds}}
	<div class="feed">
	<div>
	<strong>{{.Label}}</strong>
	<span>{{.Host}} · {{$.Text.Fingerprint}} {{.Fingerprint}}</span>
	</div>
	<label class="delete-toggle"><input form="admin-form" type="checkbox" name="delete_calendar_indices" value="{{.Index}}"> {{$.Text.DeleteOnSave}}</label>
	</div>
	{{end}}
	{{end}}
	<label>{{.Text.AddPrivateICalURLs}}</label>
	<textarea form="admin-form" name="calendar_ics_urls" spellcheck="false" placeholder="{{.Text.ICalPlaceholder}}"></textarea>
	<div class="hint">{{.Text.CalendarHint}}</div>
	<div class="row">
	<div><label>{{.Text.LookaheadDays}}</label><input form="admin-form" name="calendar_lookahead_days" type="number" min="1" value="{{.Config.CalendarLookaheadDay}}"></div>
	<div><label>{{.Text.MaxEvents}}</label><input form="admin-form" name="calendar_max_events" type="number" min="1" value="{{.Config.CalendarMaxEvents}}"></div>
	</div>
	</div>
	</div>
	</div>
	</div>
	</div>
	<div class="hint">{{.Text.LayoutHint}}</div>
	</section>
	
	<aside class="preview">
	<section>
	<h2>{{.Text.Preview}}</h2>
	<div class="preview-frame {{if .PreviewLandscape}}landscape{{end}}">
	<img src="{{.PreviewURL}}" alt="{{.Text.PreviewAlt}}">
	</div>
	<div class="hint">{{.Text.PreviewHint}}</div>
	</section>
	</aside>
</main>
<script type="application/json" id="layout-data">{{.LayoutJSON}}</script>
<script type="application/json" id="component-catalog-data">{{.ComponentCatalogJSON}}</script>
<script>
(function(){
var dataEl=document.getElementById("layout-data");
var catalogEl=document.getElementById("component-catalog-data");
var editor=document.getElementById("layout-editor");
var canvas=document.getElementById("layout-canvas");
var hidden=document.getElementById("layout-json");
var palette=document.getElementById("component-palette");
var props=document.getElementById("component-props");
var empty=document.getElementById("component-empty");
var title=document.getElementById("component-title");
var deleteButton=document.getElementById("delete-component");
var inputs={x:document.getElementById("prop-x"),y:document.getElementById("prop-y"),w:document.getElementById("prop-w"),h:document.getElementById("prop-h")};
var componentSettings=document.getElementById("component-settings");
var sourceSettings=document.querySelectorAll("[data-component-source]");
if(!dataEl||!catalogEl||!editor||!canvas||!hidden||!componentSettings){return;}
var doc=JSON.parse(dataEl.textContent||"{}");
var catalog=JSON.parse(catalogEl.textContent||"[]");
var artboardKey=editor.getAttribute("data-artboard")||"portrait";
var selectedId="";
var drag=null;
var activeGuides=[];
var GRID=12;
var SNAP=8;
var defs={};
catalog.forEach(function(item){defs[item.type]=item;});
var isChinese=(document.documentElement.lang||"").toLowerCase().indexOf("zh")===0;
function ui(en,zh){
  return isChinese ? zh : en;
}
var settingsByType={
  clock:[
    {key:"format",label:ui("Time format","时间格式"),type:"select",defaultValue:"15:04",options:[["15:04","24h"],["3:04 PM","12h"]]},
    {key:"show_date",label:ui("Show date","显示日期"),type:"checkbox",defaultValue:"true"}
  ],
  weather:[
    {key:"show_condition",label:ui("Show condition","显示天气状况"),type:"checkbox",defaultValue:"true"},
    {key:"show_high_low",label:ui("Show high/low","显示高低温"),type:"checkbox",defaultValue:"true"},
    {key:"show_meta",label:ui("Show location/wind","显示地点和风速"),type:"checkbox",defaultValue:"true"}
  ],
  calendar:[
    {key:"title",label:ui("Title","标题"),type:"text",defaultValue:ui("Calendar","日程")},
    {key:"max_items",label:ui("Max events","最多事件"),type:"number",defaultValue:"",min:"1",max:"20"}
  ],
  ai_usage:[
    {key:"title",label:ui("Title","标题"),type:"text",defaultValue:ui("AI Usage","AI 用量")},
    {key:"max_items",label:ui("Max rows","最多行数"),type:"number",defaultValue:"",min:"1",max:"10"}
  ],
  notes:[
    {key:"title",label:ui("Title","标题"),type:"text",defaultValue:ui("Notes","备注")},
    {key:"max_items",label:ui("Max notes","最多备注"),type:"number",defaultValue:"",min:"1",max:"20"}
  ]
};
function ensureBoard(){
  if(!doc.version){doc.version=1;}
  if(!doc.artboards){doc.artboards={};}
  if(!doc.artboards[artboardKey]){
    doc.artboards[artboardKey]={width:artboardKey==="landscape"?800:600,height:artboardKey==="landscape"?600:800,components:[]};
  }
  if(!doc.artboards[artboardKey].components){doc.artboards[artboardKey].components=[];}
  return doc.artboards[artboardKey];
}
function labelFor(type){
  return defs[type] ? defs[type].label : type;
}
function displayName(component){
  return component.props && component.props.title ? component.props.title : labelFor(component.type);
}
function clamp(value,min,max){
  value=Math.round(value);
  if(value<min){return min;}
  if(value>max){return max;}
  return value;
}
function snapToGrid(value){
  return Math.round(value/GRID)*GRID;
}
function alignmentStops(board, ignoreId, axis){
  var limit=axis==="x"?board.width:board.height;
  var stops=[0,limit,limit/2];
  board.components.forEach(function(component){
    if(component.id===ignoreId){return;}
    var start=axis==="x"?component.x:component.y;
    var size=axis==="x"?component.w:component.h;
    stops.push(start,start+size/2,start+size);
  });
  return stops;
}
function edgeSnap(value, stops){
  var best=null;
  var bestDistance=SNAP+1;
  stops.forEach(function(stop){
    var distance=Math.abs(stop-value);
    if(distance<=SNAP&&distance<bestDistance){
      bestDistance=distance;
      best=stop;
    }
  });
  return best;
}
function axisSnap(origin, offsets, stops){
  var bestDelta=0;
  var bestGuide=null;
  var bestDistance=SNAP+1;
  offsets.forEach(function(offset){
    var snapped=edgeSnap(origin+offset,stops);
    if(snapped===null){return;}
    var distance=Math.abs(snapped-(origin+offset));
    if(distance<bestDistance){
      bestDistance=distance;
      bestDelta=snapped-(origin+offset);
      bestGuide=snapped;
    }
  });
  return {value:origin+bestDelta,guide:bestGuide};
}
function snapMove(board, component, x, y){
  x=clamp(snapToGrid(x),0,board.width-component.w);
  y=clamp(snapToGrid(y),0,board.height-component.h);
  var xSnap=axisSnap(x,[0,component.w/2,component.w],alignmentStops(board,component.id,"x"));
  var ySnap=axisSnap(y,[0,component.h/2,component.h],alignmentStops(board,component.id,"y"));
  activeGuides=[];
  if(xSnap.guide!==null){activeGuides.push({axis:"x",value:xSnap.guide});}
  if(ySnap.guide!==null){activeGuides.push({axis:"y",value:ySnap.guide});}
  return {
    x:clamp(xSnap.value,0,board.width-component.w),
    y:clamp(ySnap.value,0,board.height-component.h)
  };
}
function snapResize(board, component, w, h, def){
  w=clamp(snapToGrid(w),def.min_w,board.width-component.x);
  h=clamp(snapToGrid(h),def.min_h,board.height-component.y);
  activeGuides=[];
  var right=edgeSnap(component.x+w,alignmentStops(board,component.id,"x"));
  if(right!==null){
    w=clamp(right-component.x,def.min_w,board.width-component.x);
    activeGuides.push({axis:"x",value:right});
  }
  var bottom=edgeSnap(component.y+h,alignmentStops(board,component.id,"y"));
  if(bottom!==null){
    h=clamp(bottom-component.y,def.min_h,board.height-component.y);
    activeGuides.push({axis:"y",value:bottom});
  }
  return {w:w,h:h};
}
function selectedComponent(){
  var board=ensureBoard();
  return board.components.find(function(component){return component.id===selectedId;})||null;
}
function ensureProps(component){
  if(!component.props){component.props={};}
  return component.props;
}
function propValue(component, setting){
  var props=component.props||{};
  if(props[setting.key]!==undefined&&props[setting.key]!==null&&props[setting.key]!==""){
    return props[setting.key];
  }
  return setting.defaultValue||"";
}
function writeProp(component, setting, value){
  var props=ensureProps(component);
  if(value===""||value===setting.defaultValue){
    delete props[setting.key];
  }else{
    props[setting.key]=String(value);
  }
  if(Object.keys(props).length===0){
    delete component.props;
  }
  updateHidden();
}
function renderComponentSettings(component){
  componentSettings.innerHTML="";
  var settings=settingsByType[component.type]||[];
  settings.forEach(function(setting){
    var label=document.createElement("label");
    if(setting.type==="checkbox"){
      label.className="check";
      var checkbox=document.createElement("input");
      checkbox.type="checkbox";
      checkbox.checked=propValue(component,setting)!=="false";
      checkbox.addEventListener("change",function(){
        writeProp(component,setting,checkbox.checked?"true":"false");
      });
      label.appendChild(checkbox);
      label.appendChild(document.createTextNode(setting.label));
      componentSettings.appendChild(label);
      return;
    }
    label.textContent=setting.label;
    var input=setting.type==="select" ? document.createElement("select") : document.createElement("input");
    if(setting.type==="select"){
      setting.options.forEach(function(option){
        var item=document.createElement("option");
        item.value=option[0];
        item.textContent=option[1];
        input.appendChild(item);
      });
    }else{
      input.type=setting.type;
      if(setting.min){input.min=setting.min;}
      if(setting.max){input.max=setting.max;}
    }
    input.value=propValue(component,setting);
    input.addEventListener("change",function(){
      writeProp(component,setting,input.value.trim());
      if(setting.key==="title"){render();}
    });
    label.appendChild(input);
    componentSettings.appendChild(label);
  });
}
function renderSourceSettings(component){
  sourceSettings.forEach(function(panel){
    panel.hidden=!component||panel.getAttribute("data-component-source")!==component.type;
  });
}
function updateHidden(){
  hidden.value=JSON.stringify(doc);
}
function updateProps(){
  var component=selectedComponent();
  empty.hidden=!!component;
  props.hidden=!component;
  if(!component){
    componentSettings.innerHTML="";
    renderSourceSettings(null);
    return;
  }
  title.textContent=labelFor(component.type);
  inputs.x.value=component.x;
  inputs.y.value=component.y;
  inputs.w.value=component.w;
  inputs.h.value=component.h;
  renderComponentSettings(component);
  renderSourceSettings(component);
}
function render(){
  var board=ensureBoard();
  canvas.style.aspectRatio=board.width+" / "+board.height;
  canvas.style.setProperty("--grid-x",(GRID/board.width*100)+"%");
  canvas.style.setProperty("--grid-y",(GRID/board.height*100)+"%");
  canvas.innerHTML="";
  board.components.forEach(function(component){
    var node=document.createElement("div");
    node.className="layout-node "+component.type+(component.id===selectedId?" selected":"");
    node.style.left=(component.x/board.width*100)+"%";
    node.style.top=(component.y/board.height*100)+"%";
    node.style.width=(component.w/board.width*100)+"%";
    node.style.height=(component.h/board.height*100)+"%";
    node.dataset.id=component.id;
    var text=document.createElement("div");
    text.textContent=displayName(component);
    var meta=document.createElement("small");
    meta.textContent=component.w+"x"+component.h;
    text.appendChild(document.createElement("br"));
    text.appendChild(meta);
    node.appendChild(text);
    var handle=document.createElement("span");
    handle.className="resize-handle";
    handle.dataset.resize="1";
    node.appendChild(handle);
    canvas.appendChild(node);
  });
  activeGuides.forEach(function(guide){
    var node=document.createElement("span");
    node.className="snap-guide "+(guide.axis==="x"?"vertical":"horizontal");
    if(guide.axis==="x"){
      node.style.left=(guide.value/board.width*100)+"%";
    }else{
      node.style.top=(guide.value/board.height*100)+"%";
    }
    canvas.appendChild(node);
  });
  updateProps();
  updateHidden();
}
function beginDrag(event, mode, component){
  var board=ensureBoard();
  var rect=canvas.getBoundingClientRect();
  drag={mode:mode,id:component.id,startX:event.clientX,startY:event.clientY,originX:component.x,originY:component.y,originW:component.w,originH:component.h,scaleX:board.width/rect.width,scaleY:board.height/rect.height};
  selectedId=component.id;
  activeGuides=[];
  render();
  event.preventDefault();
}
canvas.addEventListener("pointerdown",function(event){
  var node=event.target.closest(".layout-node");
  if(!node){selectedId="";render();return;}
  var board=ensureBoard();
  var component=board.components.find(function(item){return item.id===node.dataset.id;});
  if(!component){return;}
  beginDrag(event,event.target.dataset.resize==="1"?"resize":"move",component);
});
window.addEventListener("pointermove",function(event){
  if(!drag){return;}
  var board=ensureBoard();
  var component=board.components.find(function(item){return item.id===drag.id;});
  if(!component){return;}
  var dx=(event.clientX-drag.startX)*drag.scaleX;
  var dy=(event.clientY-drag.startY)*drag.scaleY;
  var def=defs[component.type]||{min_w:80,min_h:60};
  if(drag.mode==="resize"){
    var size=snapResize(board,component,drag.originW+dx,drag.originH+dy,def);
    component.w=size.w;
    component.h=size.h;
  }else{
    var position=snapMove(board,component,drag.originX+dx,drag.originY+dy);
    component.x=position.x;
    component.y=position.y;
  }
  render();
});
window.addEventListener("pointerup",function(){drag=null;activeGuides=[];render();});
catalog.forEach(function(item){
  var button=document.createElement("button");
  button.type="button";
  button.textContent=item.label;
  button.addEventListener("click",function(){
    var board=ensureBoard();
    var n=board.components.length+1;
    var component={id:item.type+"-"+Date.now().toString(36),type:item.type,x:clamp(24+n*14,0,board.width-item.default_w),y:clamp(24+n*14,0,board.height-item.default_h),w:item.default_w,h:item.default_h};
    board.components.push(component);
    selectedId=component.id;
    render();
  });
  palette.appendChild(button);
});
Object.keys(inputs).forEach(function(key){
  inputs[key].addEventListener("input",function(){
    var component=selectedComponent();
    if(!component){return;}
    var board=ensureBoard();
    var value=parseInt(inputs[key].value,10);
    if(!Number.isFinite(value)){return;}
    if(key==="x"){component.x=clamp(value,0,board.width-component.w);}
    if(key==="y"){component.y=clamp(value,0,board.height-component.h);}
    if(key==="w"){component.w=clamp(value,(defs[component.type]||{}).min_w||80,board.width-component.x);}
    if(key==="h"){component.h=clamp(value,(defs[component.type]||{}).min_h||60,board.height-component.y);}
    render();
  });
});
deleteButton.addEventListener("click",function(){
  var board=ensureBoard();
  board.components=board.components.filter(function(component){return component.id!==selectedId;});
  selectedId="";
  render();
});
document.querySelector("form").addEventListener("submit",updateHidden);
render();
})();
</script>
</body>
</html>`))
