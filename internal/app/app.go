package app

import (
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/api"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
	"net/http"
	"time"
)

type App struct {
	Config *config.Config
	DB     *sql.DB
	Router http.Handler
}

func New(c *config.Config) (*App, error) {
	db, e := repository.InitDB(c.DBPath)
	if e != nil {
		return nil, e
	}
	return &App{Config: c, DB: db, Router: api.SetupRouter(c, db, system.ExecRunner{Timeout: 20 * time.Second})}, nil
}
func (a *App) Run() error { return http.ListenAndServe(a.Config.ListenAddr, a.Router) }
