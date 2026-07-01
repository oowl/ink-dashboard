package main

import (
	"log"
	"net/http"

	"ink-dashboard/server/internal/config"
	"ink-dashboard/server/internal/dashboard"
	"ink-dashboard/server/internal/render"
	"ink-dashboard/server/internal/web"
)

func main() {
	cfg := config.Load()

	renderer := render.NewRenderer(cfg)
	provider := dashboard.NewProvider(cfg)
	app := web.NewApp(cfg, provider, renderer)

	addr := cfg.ListenAddr
	log.Printf("ink-dashboard server listening on http://%s", addr)
	log.Printf("expected KOReader base URL: http://<this-machine-ip>:%s", cfg.Port)

	if err := http.ListenAndServe(addr, app.Router()); err != nil {
		log.Fatal(err)
	}
}
