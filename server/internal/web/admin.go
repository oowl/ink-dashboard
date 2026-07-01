package web

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ink-dashboard/server/internal/config"
)

type adminPageData struct {
	Config             config.Config
	Text               adminText
	Token              string
	Saved              bool
	Error              string
	CalendarSummary    string
	CalendarFeeds      []calendarFeedView
	CalendarConfigured bool
	CaiyunConfigured   bool
	PreviewURL         string
	PreviewLandscape   bool
	CaiyunSelected     bool
	OpenSelected       bool
	AutoSelected       bool
	PortraitSelected   bool
	LandscapeSelected  bool
	RotatedSelected    bool
	EnglishSelected    bool
	ChineseSelected    bool
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

	settings := config.AdminSettings{
		PublicBaseURL:        strings.TrimSpace(r.FormValue("public_base_url")),
		RefreshSeconds:       formInt(r, "refresh_seconds", cfg.RefreshSeconds),
		DefaultWidth:         formInt(r, "default_width", cfg.DefaultWidth),
		DefaultHeight:        formInt(r, "default_height", cfg.DefaultHeight),
		DefaultOrient:        strings.TrimSpace(r.FormValue("default_orientation")),
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
		Config:             cfg,
		Text:               text,
		Token:              token,
		Saved:              saved,
		Error:              errMsg,
		CalendarSummary:    secretListSummary(cfg.CalendarICSURLs, cfg.Language),
		CalendarFeeds:      calendarFeedViews(cfg.CalendarICSURLs, cfg.Language),
		CalendarConfigured: len(cfg.CalendarICSURLs) > 0,
		CaiyunConfigured:   cfg.CaiyunToken != "",
		PreviewURL:         preview,
		PreviewLandscape:   previewLandscape,
		CaiyunSelected:     cfg.WeatherProvider == "caiyun",
		OpenSelected:       cfg.WeatherProvider == "openmeteo",
		AutoSelected:       cfg.DefaultOrient == "auto",
		PortraitSelected:   cfg.DefaultOrient == "portrait",
		LandscapeSelected:  cfg.DefaultOrient == "landscape",
		RotatedSelected:    cfg.DefaultOrient == "rotated",
		EnglishSelected:    cfg.Language == "en",
		ChineseSelected:    config.IsChinese(cfg.Language),
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
main{display:grid;grid-template-columns:minmax(360px,520px) 1fr;gap:24px;padding:24px;align-items:start}
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
	.preview{position:sticky;top:24px}
	.preview-frame{overflow:auto}
	.preview-frame img{width:min(100%,600px);height:auto;border:1px solid #111;background:#fff}
	.preview-frame.landscape img{width:min(100%,900px)}
.hint{font-size:12px;color:#555;margin-top:4px;line-height:1.4}
@media(max-width:900px){main{grid-template-columns:1fr}.preview{position:static}}
</style>
</head>
	<body>
	<header>
	<h1>{{.Text.HeaderTitle}}</h1>
	<div class="sub">{{.Text.ConfigFile}}: {{.Config.ConfigFile}}</div>
	</header>
	<main>
	<form method="post" action="/admin/config">
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
	
	<section>
	<h2>{{.Text.Weather}}</h2>
	<label>{{.Text.Provider}}</label>
	<select name="weather_provider">
	<option value="openmeteo" {{if .OpenSelected}}selected{{end}}>Open-Meteo</option>
	<option value="caiyun" {{if .CaiyunSelected}}selected{{end}}>Caiyun</option>
	</select>
	<div class="row">
	<div><label>{{.Text.Latitude}}</label><input name="weather_latitude" value="{{printf "%.6f" .Config.WeatherLatitude}}"></div>
	<div><label>{{.Text.Longitude}}</label><input name="weather_longitude" value="{{printf "%.6f" .Config.WeatherLongitude}}"></div>
	</div>
	<label>{{.Text.LocationLabel}}</label>
	<input name="weather_location" value="{{.Config.WeatherLocation}}" placeholder="Shanghai">
	<label>{{.Text.CaiyunToken}}</label>
	<div class="secret">{{if .CaiyunConfigured}}{{.Text.ConfiguredKeepToken}}{{else}}{{.Text.NotConfiguredPasteToken}}{{end}}</div>
	<input name="caiyun_token" type="password" value="" placeholder="{{if .CaiyunConfigured}}{{.Text.LeaveBlankKeepToken}}{{else}}{{.Text.PasteCaiyunToken}}{{end}}" autocomplete="off">
	{{if .CaiyunConfigured}}<label><input style="width:auto" type="checkbox" name="clear_caiyun_token" value="1"> {{.Text.ClearSavedCaiyunToken}}</label>{{end}}
	<div class="row">
	<div><label>{{.Text.CaiyunLang}}</label><input name="caiyun_lang" value="{{.Config.CaiyunLang}}"></div>
	<div><label>{{.Text.CaiyunUnit}}</label><input name="caiyun_unit" value="{{.Config.CaiyunUnit}}"></div>
	</div>
	</section>
	
	<section>
	<h2>{{.Text.GoogleCalendar}}</h2>
	<div class="secret">{{.CalendarSummary}}. {{.Text.CalendarSecretHint}}</div>
	{{if .CalendarConfigured}}
	<label>{{.Text.ConfiguredFeeds}}</label>
	{{range .CalendarFeeds}}
	<div class="feed">
	<div>
	<strong>{{.Label}}</strong>
		<span>{{.Host}} · {{$.Text.Fingerprint}} {{.Fingerprint}}</span>
	</div>
	<label class="delete-toggle"><input type="checkbox" name="delete_calendar_indices" value="{{.Index}}"> {{$.Text.DeleteOnSave}}</label>
	</div>
	{{end}}
	{{end}}
	<label>{{.Text.AddPrivateICalURLs}}</label>
	<textarea name="calendar_ics_urls" spellcheck="false" placeholder="{{.Text.ICalPlaceholder}}"></textarea>
	<div class="hint">{{.Text.CalendarHint}}</div>
	<div class="row">
	<div><label>{{.Text.LookaheadDays}}</label><input name="calendar_lookahead_days" type="number" min="1" value="{{.Config.CalendarLookaheadDay}}"></div>
	<div><label>{{.Text.MaxEvents}}</label><input name="calendar_max_events" type="number" min="1" value="{{.Config.CalendarMaxEvents}}"></div>
	</div>
	</section>
	
	<div class="actions">
	<button type="submit">{{.Text.SaveAndReload}}</button>
	</div>
	</form>
	
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
</body>
</html>`))
