package repo

import (
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type heartbeatRepo struct {
	db *gorm.DB
}

func (r *heartbeatRepo) Create(hb *model.Heartbeat) error {
	return r.db.Create(hb).Error
}

func (r *heartbeatRepo) GetByMonitorID(monitorID uint, limit int) ([]model.Heartbeat, error) {
	var heartbeats []model.Heartbeat
	q := r.db.Where("monitor_id = ?", monitorID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&heartbeats).Error; err != nil {
		return nil, err
	}
	return heartbeats, nil
}

func (r *heartbeatRepo) GetByMonitorIDSince(monitorID uint, since time.Time) ([]model.Heartbeat, error) {
	var heartbeats []model.Heartbeat
	if err := r.db.Where("monitor_id = ? AND created_at >= ?", monitorID, since).
		Order("created_at ASC").Find(&heartbeats).Error; err != nil {
		return nil, err
	}
	return heartbeats, nil
}

func (r *heartbeatRepo) GetLatest(monitorID uint) (*model.Heartbeat, error) {
	var hb model.Heartbeat
	if err := r.db.Where("monitor_id = ?", monitorID).
		Order("created_at DESC").First(&hb).Error; err != nil {
		return nil, err
	}
	return &hb, nil
}

func (r *heartbeatRepo) GetUptimeStats(monitorID uint) (model.UptimeStats, error) {
	now := time.Now()
	stats := model.UptimeStats{}

	var err error
	stats.Uptime24h, err = r.computeUptime(monitorID, now.Add(-24*time.Hour))
	if err != nil {
		return stats, err
	}

	stats.Uptime7d, err = r.computeUptime(monitorID, now.Add(-7*24*time.Hour))
	if err != nil {
		return stats, err
	}

	stats.Uptime30d, err = r.computeUptime(monitorID, now.Add(-30*24*time.Hour))
	if err != nil {
		return stats, err
	}

	stats.AvgLatency, err = r.computeAvgLatency(monitorID, now.Add(-24*time.Hour))
	if err != nil {
		return stats, err
	}

	return stats, nil
}

func (r *heartbeatRepo) computeUptime(monitorID uint, since time.Time) (float64, error) {
	var total int64
	if err := r.db.Model(&model.Heartbeat{}).
		Where("monitor_id = ? AND created_at >= ?", monitorID, since).
		Count(&total).Error; err != nil {
		return 0, err
	}

	if total == 0 {
		return 100.0, nil
	}

	var upCount int64
	if err := r.db.Model(&model.Heartbeat{}).
		Where("monitor_id = ? AND created_at >= ? AND status = ?", monitorID, since, model.StatusUp).
		Count(&upCount).Error; err != nil {
		return 0, err
	}

	return (float64(upCount) / float64(total)) * 100.0, nil
}

func (r *heartbeatRepo) computeAvgLatency(monitorID uint, since time.Time) (float64, error) {
	var result struct {
		Avg float64
	}
	if err := r.db.Model(&model.Heartbeat{}).
		Select("COALESCE(AVG(response_time), 0) as avg").
		Where("monitor_id = ? AND created_at >= ? AND status = ?", monitorID, since, model.StatusUp).
		Scan(&result).Error; err != nil {
		return 0, err
	}
	return result.Avg, nil
}

func (r *heartbeatRepo) GetResponseTimeHistory(monitorID uint, since time.Time, limit int) ([]model.Heartbeat, error) {
	var heartbeats []model.Heartbeat
	q := r.db.Where("monitor_id = ? AND created_at >= ?", monitorID, since).
		Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&heartbeats).Error; err != nil {
		return nil, err
	}
	return heartbeats, nil
}

func (r *heartbeatRepo) DeleteOlderThan(age time.Duration) (int64, error) {
	cutoff := time.Now().Add(-age)
	result := r.db.Where("created_at < ?", cutoff).Delete(&model.Heartbeat{})
	return result.RowsAffected, result.Error
}

func (r *heartbeatRepo) CountByMonitorID(monitorID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Heartbeat{}).Where("monitor_id = ?", monitorID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
