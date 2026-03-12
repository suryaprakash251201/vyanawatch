package db

import (
	"time"

	"gorm.io/gorm"
)

// IncidentRepo provides query operations for incident records.
type IncidentRepo struct {
	db *gorm.DB
}

// NewIncidentRepo creates a new IncidentRepo using the global DB.
func NewIncidentRepo() *IncidentRepo {
	return &IncidentRepo{db: DB}
}

// Create inserts a new incident record.
func (r *IncidentRepo) Create(inc *Incident) error {
	return r.db.Create(inc).Error
}

// GetByID retrieves a single incident by ID.
func (r *IncidentRepo) GetByID(id uint) (*Incident, error) {
	var inc Incident
	if err := r.db.First(&inc, id).Error; err != nil {
		return nil, err
	}
	return &inc, nil
}

// GetByMonitorID retrieves incidents for a monitor, ordered newest first.
func (r *IncidentRepo) GetByMonitorID(monitorID uint, limit int) ([]Incident, error) {
	var incidents []Incident
	q := r.db.Where("monitor_id = ?", monitorID).Order("started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&incidents).Error; err != nil {
		return nil, err
	}
	return incidents, nil
}

// GetOpenByMonitorID retrieves the currently open (unresolved) incident for a monitor.
func (r *IncidentRepo) GetOpenByMonitorID(monitorID uint) (*Incident, error) {
	var inc Incident
	if err := r.db.Where("monitor_id = ? AND resolved = ?", monitorID, false).
		Order("started_at DESC").First(&inc).Error; err != nil {
		return nil, err
	}
	return &inc, nil
}

// OpenIncident creates a new incident when a monitor goes down.
func (r *IncidentRepo) OpenIncident(monitorID uint, rootCause string) (*Incident, error) {
	inc := &Incident{
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

// ResolveIncident marks an open incident as resolved and computes duration.
func (r *IncidentRepo) ResolveIncident(monitorID uint) (*Incident, error) {
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

// GetRecent retrieves the most recent incidents across all monitors.
func (r *IncidentRepo) GetRecent(limit int) ([]Incident, error) {
	var incidents []Incident
	q := r.db.Order("started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&incidents).Error; err != nil {
		return nil, err
	}
	return incidents, nil
}

// GetOpenCount returns the number of currently unresolved incidents.
func (r *IncidentRepo) GetOpenCount() (int64, error) {
	var count int64
	if err := r.db.Model(&Incident{}).Where("resolved = ?", false).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// DeleteByMonitorID removes all incidents for a given monitor.
func (r *IncidentRepo) DeleteByMonitorID(monitorID uint) error {
	return r.db.Where("monitor_id = ?", monitorID).Delete(&Incident{}).Error
}
