package db

import (
	"time"

	"github.com/rs/zerolog/log"
)

// Repos holds all repository instances for easy access.
type Repos struct {
	Monitors    *MonitorRepo
	Heartbeats  *HeartbeatRepo
	Incidents   *IncidentRepo
	StatusPages *StatusPageRepo
	EventLogs   *EventLogRepo
}

// NewRepos creates a Repos instance with all repositories initialized.
// Must be called after db.Init().
func NewRepos() *Repos {
	return &Repos{
		Monitors:    NewMonitorRepo(),
		Heartbeats:  NewHeartbeatRepo(),
		Incidents:   NewIncidentRepo(),
		StatusPages: NewStatusPageRepo(),
		EventLogs:   NewEventLogRepo(),
	}
}

// CleanupOldHeartbeats removes heartbeat records older than the given duration.
// Intended to be run periodically (e.g., daily) to keep DB size in check.
func CleanupOldHeartbeats(retention time.Duration) {
	repo := NewHeartbeatRepo()
	deleted, err := repo.DeleteOlderThan(retention)
	if err != nil {
		log.Error().Err(err).Msg("Failed to cleanup old heartbeats")
		return
	}
	if deleted > 0 {
		log.Info().Int64("deleted", deleted).Str("retention", retention.String()).
			Msg("Cleaned up old heartbeat records")
	}
}
