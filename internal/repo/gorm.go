package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		dir := filepath.Dir(cfg.DSN)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("failed to create data directory %s: %w", dir, err)
		}
		dialector = sqlite.Open(cfg.DSN + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.AutoMigrate(
		&model.Monitor{},
		&model.Heartbeat{},
		&model.Incident{},
		&model.StatusPage{},
		&model.EventLog{},
		&model.Tag{},
		&model.MonitorTag{},
		&model.Maintenance{},
		&model.MaintenanceMonitor{},
	); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info().Str("driver", cfg.Driver).Msg("Database initialized")
	return db, nil
}

func NewMonitorRepo(db *gorm.DB) MonitorRepository {
	return &monitorRepo{db: db}
}

func NewHeartbeatRepo(db *gorm.DB) HeartbeatRepository {
	return &heartbeatRepo{db: db}
}

func NewIncidentRepo(db *gorm.DB) IncidentRepository {
	return &incidentRepo{db: db}
}

func NewStatusPageRepo(db *gorm.DB) StatusPageRepository {
	return &statusPageRepo{db: db}
}

func NewEventLogRepo(db *gorm.DB) EventLogRepository {
	return &eventLogRepo{db: db}
}

func NewTagRepo(db *gorm.DB) TagRepository {
	return &tagRepo{db: db}
}

func NewMaintenanceRepo(db *gorm.DB) MaintenanceRepository {
	return &maintenanceRepo{db: db}
}
