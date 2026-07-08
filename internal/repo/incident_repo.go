package repo

import (
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type incidentRepo struct {
	db *gorm.DB
}

func (r *incidentRepo) Create(inc *model.Incident) error {
	return r.db.Create(inc).Error
}

func (r *incidentRepo) GetByID(id uint) (*model.Incident, error) {
	var inc model.Incident
	if err := r.db.First(&inc, id).Error; err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *incidentRepo) GetByMonitorID(monitorID uint, limit int) ([]model.Incident, error) {
	var incidents []model.Incident
	q := r.db.Where("monitor_id = ?", monitorID).Order("started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&incidents).Error; err != nil {
		return nil, err
	}
	return incidents, nil
}

func (r *incidentRepo) GetOpenByMonitorID(monitorID uint) (*model.Incident, error) {
	var inc model.Incident
	if err := r.db.Where("monitor_id = ? AND resolved = ?", monitorID, false).
		Order("started_at DESC").First(&inc).Error; err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *incidentRepo) OpenIncident(monitorID uint, rootCause string) (*model.Incident, error) {
	inc := &model.Incident{
		MonitorID: monitorID,
		StartedAt: time.Now(),
		RootCause: rootCause,
		Resolved:  false,
	}
	if err := r.db.Create(inc).Error; err != nil {
		return nil, err
	}
	return inc, nil
}

func (r *incidentRepo) ResolveIncident(monitorID uint) (*model.Incident, error) {
	inc, err := r.GetOpenByMonitorID(monitorID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	duration := int64(now.Sub(inc.StartedAt).Seconds())

	if err := r.db.Model(inc).Updates(map[string]interface{}{
		"resolved":    true,
		"resolved_at": now,
		"duration":    duration,
	}).Error; err != nil {
		return nil, err
	}

	inc.Resolved = true
	inc.ResolvedAt = &now
	inc.Duration = duration

	return inc, nil
}

func (r *incidentRepo) GetRecent(limit int) ([]model.Incident, error) {
	var incidents []model.Incident
	q := r.db.Order("started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&incidents).Error; err != nil {
		return nil, err
	}
	return incidents, nil
}

func (r *incidentRepo) GetOpenCount() (int64, error) {
	var count int64
	if err := r.db.Model(&model.Incident{}).Where("resolved = ?", false).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *incidentRepo) DeleteByMonitorID(monitorID uint) error {
	return r.db.Where("monitor_id = ?", monitorID).Delete(&model.Incident{}).Error
}
