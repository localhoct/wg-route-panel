package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/localhoct/wg-route-panel/internal/api"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/services"
)

type App struct {
	Config *config.Config
	DB     interface{} // Will be *sql.DB in full implementation
	Router http.Handler
	WG     *services.WireGuardService
}

func New(cfg *config.Config) (*App, error) {
	db, err := repository.InitDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("database initialization failed: %w", err)
	}

	wgService := services.NewWireGuardService(cfg.WireGuard.InterfaceName, cfg.WireGuard.ConfigPath)

	router := api.SetupRouter(cfg, db, wgService)

	return &App{
		Config: cfg,
		DB:     db,
		Router: router,
		WG:     wgService,
	}, nil
}

func (a *App) Run() error {
	log.Printf("Starting WG Route Panel on %s", a.Config.ListenAddr)
	return http.ListenAndServe(a.Config.ListenAddr, a.Router)
}
