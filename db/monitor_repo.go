package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// MonitorRepo provides CRUD and query operations for monitors.
type MonitorRepo struct {
	db *gorm.DB
}

// NewMonitorRepo creates a new MonitorRepo using the global DB.
func NewMonitorRepo() *MonitorRepo {
	return &MonitorRepo{db: DB}
}

// Create inserts a new monitor after validation.
func (r *MonitorRepo) Create(m *Monitor) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.db.Create(m).Error
}

// GetByID retrieves a single monitor by ID.
func (r *MonitorRepo) GetByID(id uint) (*Monitor, error) {
	var m Monitor
	if err := r.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// GetAll retrieves all monitors, ordered by name.
func (r *MonitorRepo) GetAll() ([]Monitor, error) {
	var monitors []Monitor
	if err := r.db.Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// GetActive retrieves all active (non-paused) monitors.
func (r *MonitorRepo) GetActive() ([]Monitor, error) {
	var monitors []Monitor
	if err := r.db.Where("active = ?", true).Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// Update saves changes to an existing monitor after validation.
func (r *MonitorRepo) Update(m *Monitor) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.db.Save(m).Error
}

// Delete removes a monitor and its associated heartbeats/incidents (via CASCADE).
func (r *MonitorRepo) Delete(id uint) error {
	return r.db.Delete(&Monitor{}, id).Error
}

// UpdateStatus updates the denormalized status fields on a monitor.
func (r *MonitorRepo) UpdateStatus(id uint, status MonitorStatus, responseTime int64) error {
	now := time.Now()
	return r.db.Model(&Monitor{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        status,
		"response_time": responseTime,
		"last_check_at": now,
	}).Error
}

// GetByPushToken finds a push monitor by its unique token.
func (r *MonitorRepo) GetByPushToken(token string) (*Monitor, error) {
	var m Monitor
	if err := r.db.Where("type = ? AND push_token = ?", MonitorPush, token).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// GetByStatusPageID retrieves all monitors belonging to a status page.
func (r *MonitorRepo) GetByStatusPageID(pageID uint) ([]Monitor, error) {
	var monitors []Monitor
	if err := r.db.Where("status_page_id = ?", pageID).Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// Count returns the total number of monitors.
func (r *MonitorRepo) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&Monitor{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByStatus returns the number of monitors with the given status.
func (r *MonitorRepo) CountByStatus(status MonitorStatus) (int64, error) {
	var count int64
	if err := r.db.Model(&Monitor{}).Where("status = ?", status).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetAllWithStats retrieves all monitors with their computed uptime statistics.
func (r *MonitorRepo) GetAllWithStats() ([]MonitorWithStats, error) {
	monitors, err := r.GetAll()
	if err != nil {
		return nil, err
	}

	hbRepo := NewHeartbeatRepo()
	result := make([]MonitorWithStats, len(monitors))
	for i, m := range monitors {
		stats, err := hbRepo.GetUptimeStats(m.ID)
		if err != nil {
			stats = UptimeStats{}
		}
		result[i] = MonitorWithStats{
			Monitor:     m,
			UptimeStats: stats,
		}
	}
	return result, nil
}

// GetByIDWithStats retrieves a single monitor with its uptime statistics.
func (r *MonitorRepo) GetByIDWithStats(id uint) (*MonitorWithStats, error) {
	m, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}

	hbRepo := NewHeartbeatRepo()
	stats, err := hbRepo.GetUptimeStats(m.ID)
	if err != nil {
		stats = UptimeStats{}
	}

	return &MonitorWithStats{
		Monitor:     *m,
		UptimeStats: stats,
	}, nil
}

// Summary holds dashboard summary counts.
type Summary struct {
	Total   int64 `json:"total"`
	Up      int64 `json:"up"`
	Down    int64 `json:"down"`
	Paused  int64 `json:"paused"`
	Pending int64 `json:"pending"`
}

// GetSummary returns aggregate counts of monitors by status.
func (r *MonitorRepo) GetSummary() (*Summary, error) {
	total, err := r.Count()
	if err != nil {
		return nil, err
	}
	up, err := r.CountByStatus(StatusUp)
	if err != nil {
		return nil, err
	}
	down, err := r.CountByStatus(StatusDown)
	if err != nil {
		return nil, err
	}
	paused, err := r.CountByStatus(StatusPaused)
	if err != nil {
		return nil, err
	}
	pending, err := r.CountByStatus(StatusPending)
	if err != nil {
		return nil, err
	}

	return &Summary{
		Total:   total,
		Up:      up,
		Down:    down,
		Paused:  paused,
		Pending: pending,
	}, nil
}
