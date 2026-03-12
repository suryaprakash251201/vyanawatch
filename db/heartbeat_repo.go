package db

import (
	"time"

	"gorm.io/gorm"
)

// HeartbeatRepo provides query operations for heartbeat records.
type HeartbeatRepo struct {
	db *gorm.DB
}

// NewHeartbeatRepo creates a new HeartbeatRepo using the global DB.
func NewHeartbeatRepo() *HeartbeatRepo {
	return &HeartbeatRepo{db: DB}
}

// Create inserts a new heartbeat record.
func (r *HeartbeatRepo) Create(hb *Heartbeat) error {
	return r.db.Create(hb).Error
}

// GetByMonitorID retrieves heartbeats for a monitor, ordered newest first.
// limit=0 returns all records.
func (r *HeartbeatRepo) GetByMonitorID(monitorID uint, limit int) ([]Heartbeat, error) {
	var heartbeats []Heartbeat
	q := r.db.Where("monitor_id = ?", monitorID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&heartbeats).Error; err != nil {
		return nil, err
	}
	return heartbeats, nil
}

// GetByMonitorIDSince retrieves heartbeats for a monitor since a given time.
func (r *HeartbeatRepo) GetByMonitorIDSince(monitorID uint, since time.Time) ([]Heartbeat, error) {
	var heartbeats []Heartbeat
	if err := r.db.Where("monitor_id = ? AND created_at >= ?", monitorID, since).
		Order("created_at ASC").Find(&heartbeats).Error; err != nil {
		return nil, err
	}
	return heartbeats, nil
}

// GetLatest retrieves the most recent heartbeat for a monitor.
func (r *HeartbeatRepo) GetLatest(monitorID uint) (*Heartbeat, error) {
	var hb Heartbeat
	if err := r.db.Where("monitor_id = ?", monitorID).
		Order("created_at DESC").First(&hb).Error; err != nil {
		return nil, err
	}
	return &hb, nil
}

// GetUptimeStats computes uptime percentages and average latency for a monitor.
func (r *HeartbeatRepo) GetUptimeStats(monitorID uint) (UptimeStats, error) {
	now := time.Now()
	stats := UptimeStats{}

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

// computeUptime calculates the percentage of "up" heartbeats since a given time.
func (r *HeartbeatRepo) computeUptime(monitorID uint, since time.Time) (float64, error) {
	var total int64
	if err := r.db.Model(&Heartbeat{}).
		Where("monitor_id = ? AND created_at >= ?", monitorID, since).
		Count(&total).Error; err != nil {
		return 0, err
	}

	if total == 0 {
		return 100.0, nil // No data = assume 100%
	}

	var upCount int64
	if err := r.db.Model(&Heartbeat{}).
		Where("monitor_id = ? AND created_at >= ? AND status = ?", monitorID, since, StatusUp).
		Count(&upCount).Error; err != nil {
		return 0, err
	}

	return (float64(upCount) / float64(total)) * 100.0, nil
}

// computeAvgLatency calculates average response time (ms) since a given time.
func (r *HeartbeatRepo) computeAvgLatency(monitorID uint, since time.Time) (float64, error) {
	var result struct {
		Avg float64
	}
	if err := r.db.Model(&Heartbeat{}).
		Select("COALESCE(AVG(response_time), 0) as avg").
		Where("monitor_id = ? AND created_at >= ? AND status = ?", monitorID, since, StatusUp).
		Scan(&result).Error; err != nil {
		return 0, err
	}
	return result.Avg, nil
}

// GetResponseTimeHistory retrieves response times for charting.
// Returns heartbeats in chronological order for a given time window.
func (r *HeartbeatRepo) GetResponseTimeHistory(monitorID uint, since time.Time, limit int) ([]Heartbeat, error) {
	var heartbeats []Heartbeat
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

// DeleteOlderThan removes heartbeat records older than the given duration.
// Used for data retention/cleanup.
func (r *HeartbeatRepo) DeleteOlderThan(age time.Duration) (int64, error) {
	cutoff := time.Now().Add(-age)
	result := r.db.Where("created_at < ?", cutoff).Delete(&Heartbeat{})
	return result.RowsAffected, result.Error
}

// CountByMonitorID returns the total number of heartbeats for a monitor.
func (r *HeartbeatRepo) CountByMonitorID(monitorID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&Heartbeat{}).Where("monitor_id = ?", monitorID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
