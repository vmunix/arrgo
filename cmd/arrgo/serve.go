package main

import (
	"fmt"

	"github.com/arrgo/arrgo/internal/config"
)

func runServe(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	fmt.Printf("arrgo starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	return nil
}
