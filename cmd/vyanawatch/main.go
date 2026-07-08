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
	"github.com/vyanawatch/vyanawatch/internal/api"
	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/engine"
	"github.com/vyanawatch/vyanawatch/internal/model"
	"github.com/vyanawatch/vyanawatch/internal/notifier"
	"github.com/vyanawatch/vyanawatch/internal/repo"
)

var Version = "dev"

func main() {
	configPath := flag.String("config", "", "Path to config.yaml file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("VyanaWatch %s\n", Version)
		os.Exit(0)
	}

	printBanner()

	cfgProvider, err := config.NewProvider(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}
	cfg := cfgProvider.Get()

	log.Info().
		Str("version", Version).
		Int("port", cfg.Server.Port).
		Str("db_driver", cfg.Database.Driver).
		Msg("Starting VyanaWatch")

	db, err := repo.InitDB(&cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	log.Info().Msg("Database ready")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repos := &api.Repositories{
		Monitors:    repo.NewMonitorRepo(db),
		Heartbeats:  repo.NewHeartbeatRepo(db),
		Incidents:   repo.NewIncidentRepo(db),
		StatusPages: repo.NewStatusPageRepo(db),
		EventLogs:   repo.NewEventLogRepo(db),
		Tags:        repo.NewTagRepo(db),
		Maintenance: repo.NewMaintenanceRepo(db),
	}

	alertDispatcher := notifier.NewDispatcher(&cfg.Alerting)

	eng := engine.NewEngine(
		repos.Monitors,
		repos.Heartbeats,
		repos.Incidents,
		repos.Maintenance,
		func(event engine.Event) {
			alertDispatcher.Dispatch(event)
		},
	)

	if err := eng.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start monitoring engine")
	}

	notifierMgr := &api.NotifierManager{
		Dispatch: func(event engine.Event) {
			alertDispatcher.Dispatch(event)
		},
	}

	srv := api.NewServer(repos, eng, cfgProvider, ctx, notifierMgr)

	eng.SetEventHandler(func(event engine.Event) {
		log.Info().
			Str("event", string(event.Type)).
			Str("monitor", event.Monitor.Name).
			Str("status", string(event.Result.Status)).
			Str("message", event.Result.Message).
			Msg("Monitor state changed")

		alertDispatcher.Dispatch(event)

		repos.EventLogs.Create(&model.EventLog{
			MonitorID:   event.Monitor.ID,
			MonitorName: event.Monitor.Name,
			Status:      string(event.Result.Status),
			EventType:   string(event.Type),
			Message:     event.Result.Message,
		})

		srv.Broadcast(api.SSEEvent{
			Event: string(event.Type),
			Data:  event,
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Info().Msgf("VyanaWatch is ready on http://%s", addr)

	go func() {
		if err := srv.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("Shutting down gracefully...")
	cancel()

	eng.Stop()

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
