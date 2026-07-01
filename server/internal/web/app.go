package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ink-dashboard/server/internal/config"
	"ink-dashboard/server/internal/dashboard"
	"ink-dashboard/server/internal/render"
)

type App struct {
	mu       sync.RWMutex
	cfg      config.Config
	provider dashboard.Provider
	renderer *render.Renderer
}

type DisplayResponse struct {
	ImageURL    string `json:"image_url"`
	Filename    string `json:"filename"`
	RefreshRate int    `json:"refresh_rate"`
	LocalClock  bool   `json:"local_clock"`
	Language    string `json:"language"`
	UpdatedAt   string `json:"updated_at"`
}

func NewApp(cfg config.Config, provider dashboard.Provider, renderer *render.Renderer) *App {
	return &App{cfg: cfg, provider: provider, renderer: renderer}
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/admin", a.admin)
	mux.HandleFunc("/admin/config", a.saveAdminConfig)
	mux.HandleFunc("/api/display", a.display)
	mux.HandleFunc("/preview.svg", a.previewSVG)
	mux.Handle("/screens/", noCacheHandler(http.StripPrefix("/screens/", http.FileServer(http.Dir(a.cfg.ScreensDir)))))
	return logRequests(mux)
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *App) display(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	noCache(w)

	req, err := a.renderRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	_, _, renderer := a.current()
	screen, err := renderer.RenderPNG(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, DisplayResponse{
		ImageURL:    a.publicURL(r, "/screens/"+screen.Filename),
		Filename:    screen.Filename,
		RefreshRate: a.cfg.RefreshSeconds,
		LocalClock:  req.LocalClock,
		Language:    req.Language,
		UpdatedAt:   req.Snapshot.GeneratedAt.Format("15:04:05"),
	})
}

func (a *App) previewSVG(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	noCache(w)
	req, err := a.renderRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	fmt.Fprint(w, a.renderer.PreviewSVG(req))
}

func (a *App) renderRequest(r *http.Request) (render.RenderRequest, error) {
	cfg, provider, _ := a.current()
	snapshot, err := provider.Current()
	if err != nil {
		return render.RenderRequest{}, err
	}

	width := intFromHeaderOrQuery(r, "png-width", "width", cfg.DefaultWidth)
	height := intFromHeaderOrQuery(r, "png-height", "height", cfg.DefaultHeight)
	orientation := strings.TrimSpace(r.Header.Get("orientation"))
	if orientation == "" {
		orientation = strings.TrimSpace(r.URL.Query().Get("orientation"))
	}
	if orientation == "" {
		orientation = cfg.DefaultOrient
	}

	if width < 240 || width > 3000 || height < 240 || height > 3000 {
		return render.RenderRequest{}, fmt.Errorf("invalid screen size %dx%d", width, height)
	}

	return render.RenderRequest{
		Width:       width,
		Height:      height,
		Orientation: orientation,
		Language:    cfg.Language,
		Layout:      cfg.Layout,
		LocalClock:  boolFromHeaderOrQuery(r, "local-clock", "local_clock"),
		Snapshot:    snapshot,
	}, nil
}

func (a *App) authorized(r *http.Request) bool {
	cfg, _, _ := a.current()
	if cfg.APIKey == "" {
		return true
	}

	token := strings.TrimSpace(r.Header.Get("access-token"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	return token == cfg.APIKey
}

func (a *App) publicURL(r *http.Request, path string) string {
	cfg, _, _ := a.current()
	if cfg.PublicBaseURL != "" {
		return cfg.PublicBaseURL + path
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = net.JoinHostPort("127.0.0.1", cfg.Port)
	}
	return scheme + "://" + host + path
}

func (a *App) current() (config.Config, dashboard.Provider, *render.Renderer) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg, a.provider, a.renderer
}

func (a *App) setConfig(cfg config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
	a.provider = dashboard.NewProvider(cfg)
	a.renderer = render.NewRenderer(cfg)
}

func intFromHeaderOrQuery(r *http.Request, header, query string, fallback int) int {
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get(query))
	}
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func boolFromHeaderOrQuery(r *http.Request, header, query string) bool {
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get(query))
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
}

func noCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		noCache(w)
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, filepath.Clean(r.URL.Path))
		next.ServeHTTP(w, r)
	})
}
