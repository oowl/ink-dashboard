package config

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Host            string
	Port            string
	ListenAddr      string
	PublicBaseURL   string
	ConfigFile      string
	APIKey          string
	RefreshSeconds  int
	ScreensDir      string
	DefaultWidth    int
	DefaultHeight   int
	DefaultOrient   string
	RSVGConvertPath string
	Timezone        string
	Language        string
	HTTPTimeoutSec  int

	WeatherEnabled   bool
	WeatherProvider  string
	WeatherLatitude  float64
	WeatherLongitude float64
	WeatherLocation  string
	CaiyunToken      string
	CaiyunLang       string
	CaiyunUnit       string

	CalendarICSURLs      []string
	CalendarLookaheadDay int
	CalendarMaxEvents    int
	CalendarMaxBytes     int64
	CalendarCacheSeconds int

	CodexEnabled       bool
	CodexCLIPath       string
	CodexQueryCWD      string
	CodexModel         string
	CodexJSONLRoot     string
	CodexJSONLStaleSec int
	CodexTimeoutSec    int
	CodexCacheSeconds  int
	CodexStatusWaitSec int
}

type AdminSettings struct {
	PublicBaseURL        string   `json:"public_base_url"`
	RefreshSeconds       int      `json:"refresh_seconds"`
	DefaultWidth         int      `json:"default_width"`
	DefaultHeight        int      `json:"default_height"`
	DefaultOrient        string   `json:"default_orientation"`
	Timezone             string   `json:"timezone"`
	Language             string   `json:"language"`
	WeatherProvider      string   `json:"weather_provider"`
	WeatherLatitude      float64  `json:"weather_latitude"`
	WeatherLongitude     float64  `json:"weather_longitude"`
	WeatherLocation      string   `json:"weather_location"`
	CaiyunToken          string   `json:"caiyun_token"`
	CaiyunLang           string   `json:"caiyun_lang"`
	CaiyunUnit           string   `json:"caiyun_unit"`
	CalendarICSURLs      []string `json:"calendar_ics_urls"`
	CalendarLookaheadDay int      `json:"calendar_lookahead_days"`
	CalendarMaxEvents    int      `json:"calendar_max_events"`
}

func Load() Config {
	loadDotEnv("../.env")
	loadDotEnv(".env")

	host := env("INKDASH_HOST", "0.0.0.0")
	port := env("INKDASH_PORT", "8787")
	publicBase := strings.TrimRight(env("INKDASH_PUBLIC_BASE_URL", ""), "/")
	configFile := env("INKDASH_CONFIG_FILE", "config.local.json")
	weatherProvider := strings.ToLower(env("INKDASH_WEATHER_PROVIDER", "openmeteo"))
	if weatherProvider == "open-meteo" {
		weatherProvider = "openmeteo"
	}

	cfg := Config{
		Host:            host,
		Port:            port,
		ListenAddr:      net.JoinHostPort(host, port),
		PublicBaseURL:   publicBase,
		ConfigFile:      configFile,
		APIKey:          env("INKDASH_API_KEY", "change-me"),
		RefreshSeconds:  envInt("INKDASH_REFRESH_SECONDS", 600),
		ScreensDir:      env("INKDASH_SCREENS_DIR", filepath.Join("public", "screens")),
		DefaultWidth:    envInt("INKDASH_DEFAULT_WIDTH", 600),
		DefaultHeight:   envInt("INKDASH_DEFAULT_HEIGHT", 800),
		DefaultOrient:   env("INKDASH_ORIENTATION", "auto"),
		RSVGConvertPath: env("INKDASH_RSVG_CONVERT", "rsvg-convert"),
		Timezone:        env("INKDASH_TIMEZONE", "Asia/Shanghai"),
		Language:        env("INKDASH_LANGUAGE", "zh-CN"),
		HTTPTimeoutSec:  envInt("INKDASH_HTTP_TIMEOUT_SECONDS", 10),

		WeatherProvider:  weatherProvider,
		WeatherLatitude:  envFloat("INKDASH_WEATHER_LAT", 0),
		WeatherLongitude: envFloat("INKDASH_WEATHER_LON", 0),
		WeatherLocation:  env("INKDASH_WEATHER_LOCATION", "Weather"),
		CaiyunToken:      env("INKDASH_CAIYUN_TOKEN", ""),
		CaiyunLang:       env("INKDASH_CAIYUN_LANG", "zh_CN"),
		CaiyunUnit:       env("INKDASH_CAIYUN_UNIT", "metric"),

		CalendarICSURLs:      calendarURLs(),
		CalendarLookaheadDay: envInt("INKDASH_CALENDAR_LOOKAHEAD_DAYS", 7),
		CalendarMaxEvents:    envInt("INKDASH_CALENDAR_MAX_EVENTS", 4),
		CalendarMaxBytes:     int64(envInt("INKDASH_CALENDAR_MAX_BYTES", 25*1024*1024)),
		CalendarCacheSeconds: envInt("INKDASH_CALENDAR_CACHE_SECONDS", 900),

		CodexEnabled:       envBool("INKDASH_CODEX_ENABLED", true),
		CodexCLIPath:       env("INKDASH_CODEX_CLI", "codex"),
		CodexQueryCWD:      env("INKDASH_CODEX_CWD", "."),
		CodexModel:         env("INKDASH_CODEX_MODEL", ""),
		CodexJSONLRoot:     env("INKDASH_CODEX_JSONL_ROOT", defaultCodexJSONLRoot()),
		CodexJSONLStaleSec: envInt("INKDASH_CODEX_JSONL_STALE_SECONDS", 300),
		CodexTimeoutSec:    envInt("INKDASH_CODEX_TIMEOUT_SECONDS", 60),
		CodexCacheSeconds:  envInt("INKDASH_CODEX_CACHE_SECONDS", 300),
		CodexStatusWaitSec: envInt("INKDASH_CODEX_STATUS_WAIT_SECONDS", 12),
	}

	if settings, err := LoadAdminSettings(configFile); err == nil {
		ApplyAdminSettings(&cfg, settings)
	}
	cfg.Normalize()

	return cfg
}

func (c *Config) Normalize() {
	c.PublicBaseURL = strings.TrimRight(c.PublicBaseURL, "/")
	c.DefaultOrient = normalizeOrientation(c.DefaultOrient)
	c.Language = NormalizeLanguage(c.Language)
	c.WeatherProvider = strings.ToLower(strings.TrimSpace(c.WeatherProvider))
	if c.WeatherProvider == "open-meteo" {
		c.WeatherProvider = "openmeteo"
	}
	if c.WeatherProvider == "" {
		c.WeatherProvider = "openmeteo"
	}
	if c.CaiyunLang == "" {
		c.CaiyunLang = "zh_CN"
	}
	if c.CaiyunUnit == "" {
		c.CaiyunUnit = "metric"
	}
	if c.CalendarLookaheadDay <= 0 {
		c.CalendarLookaheadDay = 7
	}
	if c.CalendarMaxEvents <= 0 {
		c.CalendarMaxEvents = 4
	}
	if c.CalendarCacheSeconds <= 0 {
		c.CalendarCacheSeconds = 900
	}
	if c.CodexCLIPath == "" {
		c.CodexCLIPath = "codex"
	}
	if c.CodexQueryCWD == "" {
		c.CodexQueryCWD = "."
	}
	c.CodexJSONLRoot = expandHomePath(c.CodexJSONLRoot)
	if c.CodexJSONLRoot == "" {
		c.CodexJSONLRoot = defaultCodexJSONLRoot()
	}
	if c.CodexJSONLStaleSec <= 0 {
		c.CodexJSONLStaleSec = 300
	}
	if c.CodexTimeoutSec <= 0 {
		c.CodexTimeoutSec = 60
	}
	if c.CodexCacheSeconds <= 0 {
		c.CodexCacheSeconds = 300
	}
	if c.CodexStatusWaitSec <= 0 {
		c.CodexStatusWaitSec = 12
	}
	c.WeatherEnabled = c.WeatherLatitude != 0 && c.WeatherLongitude != 0 &&
		(c.WeatherProvider == "openmeteo" || (c.WeatherProvider == "caiyun" && c.CaiyunToken != ""))
}

func LoadAdminSettings(path string) (AdminSettings, error) {
	var settings AdminSettings
	file, err := os.Open(path)
	if err != nil {
		return settings, err
	}
	defer file.Close()
	err = json.NewDecoder(file).Decode(&settings)
	return settings, err
}

func SaveAdminSettings(path string, settings AdminSettings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(settings)
}

func (c Config) AdminSettings() AdminSettings {
	return AdminSettings{
		PublicBaseURL:        c.PublicBaseURL,
		RefreshSeconds:       c.RefreshSeconds,
		DefaultWidth:         c.DefaultWidth,
		DefaultHeight:        c.DefaultHeight,
		DefaultOrient:        c.DefaultOrient,
		Timezone:             c.Timezone,
		Language:             c.Language,
		WeatherProvider:      c.WeatherProvider,
		WeatherLatitude:      c.WeatherLatitude,
		WeatherLongitude:     c.WeatherLongitude,
		WeatherLocation:      c.WeatherLocation,
		CaiyunToken:          c.CaiyunToken,
		CaiyunLang:           c.CaiyunLang,
		CaiyunUnit:           c.CaiyunUnit,
		CalendarICSURLs:      c.CalendarICSURLs,
		CalendarLookaheadDay: c.CalendarLookaheadDay,
		CalendarMaxEvents:    c.CalendarMaxEvents,
	}
}

func defaultCodexJSONLRoot() string {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Join(codexHome, "sessions")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex", "sessions")
	}
	return ""
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func ApplyAdminSettings(c *Config, settings AdminSettings) {
	c.PublicBaseURL = settings.PublicBaseURL
	if settings.RefreshSeconds > 0 {
		c.RefreshSeconds = settings.RefreshSeconds
	}
	if settings.DefaultWidth > 0 {
		c.DefaultWidth = settings.DefaultWidth
	}
	if settings.DefaultHeight > 0 {
		c.DefaultHeight = settings.DefaultHeight
	}
	if settings.DefaultOrient != "" {
		c.DefaultOrient = settings.DefaultOrient
	}
	if settings.Timezone != "" {
		c.Timezone = settings.Timezone
	}
	if settings.Language != "" {
		c.Language = settings.Language
	}
	if settings.WeatherProvider != "" {
		c.WeatherProvider = settings.WeatherProvider
	}
	c.WeatherLatitude = settings.WeatherLatitude
	c.WeatherLongitude = settings.WeatherLongitude
	c.WeatherLocation = settings.WeatherLocation
	c.CaiyunToken = settings.CaiyunToken
	if settings.CaiyunLang != "" {
		c.CaiyunLang = settings.CaiyunLang
	}
	if settings.CaiyunUnit != "" {
		c.CaiyunUnit = settings.CaiyunUnit
	}
	c.CalendarICSURLs = settings.CalendarICSURLs
	if settings.CalendarLookaheadDay > 0 {
		c.CalendarLookaheadDay = settings.CalendarLookaheadDay
	}
	if settings.CalendarMaxEvents > 0 {
		c.CalendarMaxEvents = settings.CalendarMaxEvents
	}
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func hasEnv(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) != ""
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func calendarURLs() []string {
	raw := strings.TrimSpace(os.Getenv("INKDASH_CALENDAR_ICS_URLS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("INKDASH_CALENDAR_ICS_URL"))
	}
	if raw == "" {
		return nil
	}

	var urls []string
	for _, part := range strings.Split(raw, ",") {
		url := strings.TrimSpace(part)
		if url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func normalizeOrientation(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "portrait", "landscape", "rotated":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func NormalizeLanguage(value string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	switch normalized {
	case "zh", "zh-cn", "zh-hans", "cn", "chinese":
		return "zh-CN"
	case "en", "en-us", "en-gb", "english":
		return "en"
	default:
		return "en"
	}
}

func IsChinese(language string) bool {
	return NormalizeLanguage(language) == "zh-CN"
}
