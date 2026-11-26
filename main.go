package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"mmonitor/internal/config"
	"mmonitor/internal/monitor"
	"mmonitor/internal/server"
	"mmonitor/internal/storage"
)

//go:embed LICENSE assets/logo.svg all:web/dist
var embeddedAssets embed.FS

var collectorShutdownTimeout = 5 * time.Second

type application struct {
	cfg           config.Config
	db            *sql.DB
	svc           *monitor.Service
	httpServer    *http.Server
	listener      net.Listener
	ctx           context.Context
	cancel        context.CancelFunc
	collectorDone chan struct{}
	serverErr     chan error
	shutdownOnce  sync.Once
	shutdownErr   error
}

func main() {
	if err := runPlatform(); err != nil {
		reportPlatformFatal(err)
		os.Exit(1)
	}
}

func newApplication(cfg config.Config, dbPath string) (*application, error) {
	if cfg.DBPath != "" {
		dbPath = cfg.DBPath
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, err
	}

	closeDBOnError := func(cause error) (*application, error) {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("close database after failed startup: %v", closeErr)
		}
		return nil, cause
	}

	svc, err := monitor.NewService(db, dbPath, cfg)
	if err != nil {
		return closeDBOnError(err)
	}

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return closeDBOnError(fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err))
	}

	staticFS, err := fs.Sub(embeddedAssets, "web/dist")
	if err != nil {
		_ = listener.Close()
		return closeDBOnError(fmt.Errorf("prepare embedded static assets: %w", err))
	}
	licenseRaw, _ := embeddedAssets.ReadFile("LICENSE")
	logoSVG, _ := embeddedAssets.ReadFile("assets/logo.svg")

	ctx, cancel := context.WithCancel(context.Background())
	return &application{
		cfg: cfg,
		db:  db,
		svc: svc,
		httpServer: &http.Server{
			Handler:           server.New(svc, staticFS, licenseRaw, logoSVG),
			ReadHeaderTimeout: 5 * time.Second,
		},
		listener:      listener,
		ctx:           ctx,
		cancel:        cancel,
		collectorDone: make(chan struct{}),
		serverErr:     make(chan error, 1),
	}, nil
}

func (a *application) start() {
	go func() {
		defer close(a.collectorDone)
		a.svc.RunCollector(a.ctx)
	}()

	go func() {
		log.Printf("mmonitor v%s listening on %s", config.AppVersion, a.listener.Addr().String())
		if err := a.httpServer.Serve(a.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case a.serverErr <- fmt.Errorf("http server: %w", err):
			default:
			}
		}
	}()
}

func (a *application) errors() <-chan error {
	return a.serverErr
}

func (a *application) shutdown() error {
	a.shutdownOnce.Do(func() {
		a.shutdownErr = a.shutdownApplication()
	})
	return a.shutdownErr
}

func (a *application) shutdownApplication() error {
	a.cancel()

	var errs []error
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown http server: %v", err)
		errs = append(errs, fmt.Errorf("shutdown server: %w", err))
	}

	select {
	case <-a.collectorDone:
	case <-time.After(collectorShutdownTimeout):
		log.Printf("collector shutdown timed out")
		errs = append(errs, errors.New("collector shutdown timed out"))
	}

	if len(errs) > 0 {
		errs = append(errs, errors.New("skipping final flush and database close: shutdown deadline exceeded"))
		return errors.Join(errs...)
	}

	if err := a.svc.FlushAllBuckets(); err != nil {
		log.Printf("flush aggregate buffer on shutdown: %v", err)
		errs = append(errs, fmt.Errorf("flush aggregate buffer: %w", err))
	}
	if err := storage.BackfillSummary(a.db, time.Now().UnixMilli()); err != nil {
		log.Printf("build traffic summary on shutdown: %v", err)
		errs = append(errs, fmt.Errorf("build traffic summary: %w", err))
	}
	if err := a.db.Close(); err != nil {
		log.Printf("close database on shutdown: %v", err)
		errs = append(errs, fmt.Errorf("close database: %w", err))
	}
	return errors.Join(errs...)
}
