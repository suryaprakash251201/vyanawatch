package repo

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vyanawatch/vyanawatch/internal/model"
	"gorm.io/gorm"
)

type statusPageRepo struct {
	db *gorm.DB
}

var nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := strings.ToLower(strings.TrimSpace(s))
	slug = nonAlphanumRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func (r *statusPageRepo) Create(sp *model.StatusPage) error {
	if sp.Slug == "" {
		sp.Slug = slugify(sp.Name)
	}
	return r.db.Create(sp).Error
}

func (r *statusPageRepo) GetByID(id uint) (*model.StatusPage, error) {
	var sp model.StatusPage
	if err := r.db.Preload("Monitors").First(&sp, id).Error; err != nil {
		return nil, err
	}
	return &sp, nil
}

func (r *statusPageRepo) GetBySlug(slug string) (*model.StatusPage, error) {
	var sp model.StatusPage
	if err := r.db.Preload("Monitors").
		Where("slug = ? AND published = ?", slug, true).
		First(&sp).Error; err != nil {
		return nil, err
	}
	return &sp, nil
}

func (r *statusPageRepo) GetAll() ([]model.StatusPage, error) {
	var pages []model.StatusPage
	if err := r.db.Order("name ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *statusPageRepo) Update(sp *model.StatusPage) error {
	return r.db.Save(sp).Error
}

func (r *statusPageRepo) Delete(id uint) error {
	if err := r.db.Model(&model.Monitor{}).Where("status_page_id = ?", id).
		Update("status_page_id", nil).Error; err != nil {
		return fmt.Errorf("failed to unlink monitors: %w", err)
	}
	return r.db.Delete(&model.StatusPage{}, id).Error
}

func (r *statusPageRepo) AddMonitor(pageID, monitorID uint) error {
	return r.db.Model(&model.Monitor{}).Where("id = ?", monitorID).
		Update("status_page_id", pageID).Error
}

func (r *statusPageRepo) RemoveMonitor(monitorID uint) error {
	return r.db.Model(&model.Monitor{}).Where("id = ?", monitorID).
		Update("status_page_id", nil).Error
}
