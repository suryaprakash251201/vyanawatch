package repo

import (
	"fmt"
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type monitorRepo struct {
	db *gorm.DB
}

func (r *monitorRepo) Create(m *model.Monitor) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if m.PushToken == "" {
		token, err := model.GenerateToken(16)
		if err != nil {
			return fmt.Errorf("failed to generate push token: %w", err)
		}
		m.PushToken = token
	}
	return r.db.Create(m).Error
}

func (r *monitorRepo) GetByID(id uint) (*model.Monitor, error) {
	var m model.Monitor
	if err := r.db.Preload("Tags").First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *monitorRepo) GetAll() ([]model.Monitor, error) {
	var monitors []model.Monitor
	if err := r.db.Preload("Tags").Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

func (r *monitorRepo) GetActive() ([]model.Monitor, error) {
	var monitors []model.Monitor
	if err := r.db.Where("active = ?", true).Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

func (r *monitorRepo) Update(m *model.Monitor) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.db.Save(m).Error
}

func (r *monitorRepo) Delete(id uint) error {
	return r.db.Delete(&model.Monitor{}, id).Error
}

func (r *monitorRepo) UpdateStatus(id uint, status model.MonitorStatus, responseTime int64, certExpiry *time.Time) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":        status,
		"response_time": responseTime,
		"last_check_at": now,
	}
	if certExpiry != nil {
		updates["cert_expiry"] = *certExpiry
	}
	return r.db.Model(&model.Monitor{}).Where("id = ?", id).Updates(updates).Error
}

func (r *monitorRepo) GetByPushToken(token string) (*model.Monitor, error) {
	var m model.Monitor
	if err := r.db.Where("type = ? AND push_token = ?", model.MonitorPush, token).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *monitorRepo) GetByStatusPageID(pageID uint) ([]model.Monitor, error) {
	var monitors []model.Monitor
	if err := r.db.Where("status_page_id = ?", pageID).Order("name ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

func (r *monitorRepo) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&model.Monitor{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *monitorRepo) CountByStatus(status model.MonitorStatus) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Monitor{}).Where("status = ?", status).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *monitorRepo) getAllWithStats() ([]model.MonitorWithStats, error) {
	monitors, err := r.GetAll()
	if err != nil {
		return nil, err
	}

	hbRepo := &heartbeatRepo{db: r.db}
	result := make([]model.MonitorWithStats, len(monitors))
	for i, m := range monitors {
		stats, err := hbRepo.GetUptimeStats(m.ID)
		if err != nil {
			stats = model.UptimeStats{}
		}
		hbs, _ := hbRepo.GetByMonitorID(m.ID, 30)
		if hbs == nil {
			hbs = []model.Heartbeat{}
		}
		result[i] = model.MonitorWithStats{
			Monitor:          m,
			UptimeStats:      stats,
			RecentHeartbeats: hbs,
		}
	}
	return result, nil
}

func (r *monitorRepo) GetAllWithStats() ([]model.MonitorWithStats, error) {
	return r.getAllWithStats()
}

func (r *monitorRepo) GetByIDWithStats(id uint) (*model.MonitorWithStats, error) {
	m, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}

	hbRepo := &heartbeatRepo{db: r.db}
	stats, err := hbRepo.GetUptimeStats(m.ID)
	if err != nil {
		stats = model.UptimeStats{}
	}
	hbs, _ := hbRepo.GetByMonitorID(m.ID, 30)
	if hbs == nil {
		hbs = []model.Heartbeat{}
	}

	return &model.MonitorWithStats{
		Monitor:          *m,
		UptimeStats:      stats,
		RecentHeartbeats: hbs,
	}, nil
}

func (r *monitorRepo) GetSummary() (*Summary, error) {
	total, err := r.Count()
	if err != nil {
		return nil, err
	}
	up, err := r.CountByStatus(model.StatusUp)
	if err != nil {
		return nil, err
	}
	down, err := r.CountByStatus(model.StatusDown)
	if err != nil {
		return nil, err
	}
	paused, err := r.CountByStatus(model.StatusPaused)
	if err != nil {
		return nil, err
	}
	pending, err := r.CountByStatus(model.StatusPending)
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
