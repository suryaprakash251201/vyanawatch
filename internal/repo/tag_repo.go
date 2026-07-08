package repo

import (
	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type tagRepo struct {
	db *gorm.DB
}

func (r *tagRepo) Create(t *model.Tag) error {
	return r.db.Create(t).Error
}

func (r *tagRepo) GetAll() ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Order("name ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *tagRepo) GetByID(id uint) (*model.Tag, error) {
	var t model.Tag
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *tagRepo) Update(t *model.Tag) error {
	return r.db.Save(t).Error
}

func (r *tagRepo) Delete(id uint) error {
	return r.db.Delete(&model.Tag{}, id).Error
}

func (r *tagRepo) GetByMonitorID(monitorID uint) ([]model.Tag, error) {
	var tags []model.Tag
	err := r.db.Joins("JOIN monitor_tags ON monitor_tags.tag_id = tags.id").
		Where("monitor_tags.monitor_id = ?", monitorID).
		Order("tags.name ASC").
		Find(&tags).Error
	return tags, err
}

func (r *tagRepo) SetMonitorTags(monitorID uint, tagIDs []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("monitor_id = ?", monitorID).Delete(&model.MonitorTag{}).Error; err != nil {
			return err
		}
		for _, tagID := range tagIDs {
			mt := model.MonitorTag{
				MonitorID: monitorID,
				TagID:     tagID,
			}
			if err := tx.Create(&mt).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
