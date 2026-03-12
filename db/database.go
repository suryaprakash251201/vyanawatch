package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Init initializes the database connection and runs auto-migrations.
func Init(cfg *config.Config) error {
	var dialector gorm.Dialector

	switch cfg.Database.Driver {
	case "sqlite":
		// Ensure data directory exists
		dir := filepath.Dir(cfg.Database.DSN)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create data directory %s: %w", dir, err)
		}
		dialector = sqlite.Open(cfg.Database.DSN + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	case "postgres":
		dialector = postgres.Open(cfg.Database.DSN)
	default:
		return fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate models
	if err := db.AutoMigrate(
		&Monitor{},
		&Heartbeat{},
		&Incident{},
		&StatusPage{},
		&EventLog{},
	); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	DB = db
	log.Info().Str("driver", cfg.Database.Driver).Msg("Database initialized")
	return nil
}
