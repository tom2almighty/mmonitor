//go:build !windows

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"mmonitor/internal/config"
)

func runPlatform() error {
	cfg := config.Load()

	app, err := newApplication(cfg, config.DefaultDatabasePath())
	if err != nil {
		return err
	}

	app.start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-app.errors():
		log.Printf("server error: %v", err)
		if shutdownErr := app.shutdown(); shutdownErr != nil {
			log.Printf("shutdown after server error: %v", shutdownErr)
		}
		return err
	case <-sigCh:
	}

	return app.shutdown()
}

func reportPlatformFatal(err error) {
	log.Printf("fatal: %v", err)
}
