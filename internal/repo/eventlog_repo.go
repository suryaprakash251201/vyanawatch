package repo

import (
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type eventLogRepo struct {
	db *gorm.DB
}

func (r *eventLogRepo) Create(e *model.EventLog) error {
	return r.db.Create(e).Error
}

func (r *eventLogRepo) GetRecent(limit int) ([]model.EventLog, error) {
	var logs []model.EventLog
	err := r.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *eventLogRepo) GetByMonitor(monitorID uint, limit int) ([]model.EventLog, error) {
	var logs []model.EventLog
	err := r.db.Where("monitor_id = ?", monitorID).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *eventLogRepo) DeleteAll() error {
	return r.db.Where("1 = 1").Delete(&model.EventLog{}).Error
}

func (r *eventLogRepo) DeleteOlderThan(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result := r.db.Where("created_at < ?", cutoff).Delete(&model.EventLog{})
	return result.RowsAffected, result.Error
}
