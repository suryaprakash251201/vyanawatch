package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/alert"
	"github.com/vyanawatch/vyanawatch/api"
	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/db"
	"github.com/vyanawatch/vyanawatch/monitor"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to config.yaml file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("VyanaWatch %s\n", Version)
		os.Exit(0)
	}

	// Banner
	printBanner()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().
		Str("version", Version).
		Int("port", cfg.Server.Port).
		Str("db_driver", cfg.Database.Driver).
		Msg("Starting VyanaWatch")

	// Initialize database
	if err := db.Init(cfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	log.Info().Msg("Database ready")

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize repositories
	repos := db.NewRepos()

	// Initialize alert dispatcher
	alertDispatcher := alert.NewDispatcher(cfg)

	// Initialize and start monitoring engine
	engine := monitor.NewEngine(repos, func(event monitor.Event) {
		log.Info().
			Str("event", string(event.Type)).
			Str("monitor", event.Monitor.Name).
			Str("status", string(event.Result.Status)).
			Str("message", event.Result.Message).
			Msg("Monitor state changed")

		// Send alerts
		alertDispatcher.Dispatch(event)
	})

	if err := engine.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start monitoring engine")
	}

	// Initialize HTTP server (API + UI)
	srv := api.NewServer(repos, engine, ctx)

	// Wire engine events to SSE broadcast + alerts + event log persistence
	engine.SetEventHandler(func(event monitor.Event) {
		log.Info().
			Str("event", string(event.Type)).
			Str("monitor", event.Monitor.Name).
			Str("status", string(event.Result.Status)).
			Str("message", event.Result.Message).
			Msg("Monitor state changed")

		// Send alerts
		alertDispatcher.Dispatch(event)

		// Persist event log
		repos.EventLogs.Create(&db.EventLog{
			MonitorID:   event.Monitor.ID,
			MonitorName: event.Monitor.Name,
			Status:      string(event.Result.Status),
			EventType:   string(event.Type),
			Message:     event.Result.Message,
		})

		// Broadcast to SSE clients
		srv.Broadcast(api.SSEEvent{
			Event: string(event.Type),
			Data:  event,
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Info().Msgf("VyanaWatch is ready on http://%s", addr)

	// Start HTTP server in a goroutine
	go func() {
		if err := srv.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("Shutting down gracefully...")
	cancel()

	// Stop monitoring engine
	engine.Stop()

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	log.Info().Msg("VyanaWatch stopped")
}

func printBanner() {
	banner := `
 __     __                    __        __    _       _     
 \ \   / /   _  __ _ _ __   __ \ \      / /_ _| |_ ___| |__  
  \ \ / / | | |/ _' | '_ \ / _' \ \ /\ / / _' | __/ __| '_ \ 
   \ V /| |_| | (_| | | | | (_| |\ V  V / (_| | || (__| | | |
    \_/  \__, |\__,_|_| |_|\__,_| \_/\_/ \__,_|\__\___|_| |_|
         |___/                                                 
    Lightweight Self-Hosted Uptime Monitor
`
	fmt.Print(banner)
}
