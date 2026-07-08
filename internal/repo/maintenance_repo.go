package repo

import (
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type maintenanceRepo struct {
	db *gorm.DB
}

func (r *maintenanceRepo) Create(m *model.Maintenance) error {
	return r.db.Create(m).Error
}

func (r *maintenanceRepo) GetByID(id uint) (*model.Maintenance, error) {
	var m model.Maintenance
	if err := r.db.Preload("Monitors").First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *maintenanceRepo) GetAll() ([]model.Maintenance, error) {
	var items []model.Maintenance
	if err := r.db.Order("start_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *maintenanceRepo) GetActive() ([]model.Maintenance, error) {
	now := time.Now()
	var items []model.Maintenance
	if err := r.db.Where("start_at <= ? AND end_at >= ?", now, now).
		Order("start_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *maintenanceRepo) Update(m *model.Maintenance) error {
	return r.db.Save(m).Error
}

func (r *maintenanceRepo) Delete(id uint) error {
	return r.db.Delete(&model.Maintenance{}, id).Error
}

func (r *maintenanceRepo) IsMonitorInMaintenance(monitorID uint) (bool, error) {
	now := time.Now()
	var count int64
	err := r.db.Model(&model.MaintenanceMonitor{}).
		Joins("JOIN maintenances ON maintenances.id = maintenance_monitors.maintenance_id").
		Where("maintenance_monitors.monitor_id = ? AND maintenances.start_at <= ? AND maintenances.end_at >= ?",
			monitorID, now, now).
		Count(&count).Error
	return count > 0, err
}
