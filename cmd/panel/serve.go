package main

import (
	"context"
	"errors"
	"github.com/localhoct/wg-route-panel/internal/api"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func load() (*config.Config, error) {
	p := os.Getenv("PANEL_CONFIG_PATH")
	if p == "" {
		p = "/etc/wg-route-panel/panel.yaml"
	}
	return config.Load(p)
}
func serve() error {
	c, e := load()
	if e != nil {
		return e
	}
	db, e := repository.InitDB(c.DBPath)
	if e != nil {
		return e
	}
	defer db.Close()
	if p := os.Getenv("PANEL_ADMIN_PASSWORD_INIT"); p != "" {
		log.Print("PANEL_ADMIN_PASSWORD_INIT is only honored by create-admin; ignoring it during serve")
	}
	s := &http.Server{Addr: c.ListenAddr, Handler: api.SetupRouter(c, db, system.ExecRunner{Timeout: 20 * time.Second}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.Shutdown(ctx)
	}()
	log.Printf("WG Route Panel listening on %s", c.ListenAddr)
	e = s.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
