package db

import "time"

// EventLogRepo provides CRUD operations for event logs.
type EventLogRepo struct{}

// NewEventLogRepo creates a new EventLogRepo.
func NewEventLogRepo() *EventLogRepo {
	return &EventLogRepo{}
}

// Create inserts a new event log record.
func (r *EventLogRepo) Create(e *EventLog) error {
	return DB.Create(e).Error
}

// GetRecent returns the most recent event logs up to the given limit.
func (r *EventLogRepo) GetRecent(limit int) ([]EventLog, error) {
	var logs []EventLog
	err := DB.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// GetByMonitor returns recent event logs for a specific monitor.
func (r *EventLogRepo) GetByMonitor(monitorID uint, limit int) ([]EventLog, error) {
	var logs []EventLog
	err := DB.Where("monitor_id = ?", monitorID).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// DeleteAll removes all event logs.
func (r *EventLogRepo) DeleteAll() error {
	return DB.Where("1 = 1").Delete(&EventLog{}).Error
}

// DeleteOlderThan removes event log records older than the given duration.
func (r *EventLogRepo) DeleteOlderThan(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result := DB.Where("created_at < ?", cutoff).Delete(&EventLog{})
	return result.RowsAffected, result.Error
}
