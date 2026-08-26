package main

import (
	"fmt"
	"log"
	"os"

	"github.com/yourusername/wg-route-panel/internal/app"
	"github.com/yourusername/wg-route-panel/internal/config"
)

func main() {
	cfgPath := os.Getenv("PANEL_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/etc/wg-route-panel/panel.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	fmt.Printf("Starting WG Route Panel on %s\n", cfg.ListenAddr)
	if err := application.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
