package db

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// StatusPageRepo provides CRUD operations for status pages.
type StatusPageRepo struct {
	db *gorm.DB
}

// NewStatusPageRepo creates a new StatusPageRepo using the global DB.
func NewStatusPageRepo() *StatusPageRepo {
	return &StatusPageRepo{db: DB}
}

// Create inserts a new status page, auto-generating a slug if empty.
func (r *StatusPageRepo) Create(sp *StatusPage) error {
	if sp.Slug == "" {
		sp.Slug = slugify(sp.Name)
	}
	return r.db.Create(sp).Error
}

// GetByID retrieves a status page by ID with its monitors.
func (r *StatusPageRepo) GetByID(id uint) (*StatusPage, error) {
	var sp StatusPage
	if err := r.db.Preload("Monitors").First(&sp, id).Error; err != nil {
		return nil, err
	}
	return &sp, nil
}

// GetBySlug retrieves a published status page by its URL slug.
func (r *StatusPageRepo) GetBySlug(slug string) (*StatusPage, error) {
	var sp StatusPage
	if err := r.db.Preload("Monitors").
		Where("slug = ? AND published = ?", slug, true).
		First(&sp).Error; err != nil {
		return nil, err
	}
	return &sp, nil
}

// GetAll retrieves all status pages.
func (r *StatusPageRepo) GetAll() ([]StatusPage, error) {
	var pages []StatusPage
	if err := r.db.Order("name ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

// Update saves changes to an existing status page.
func (r *StatusPageRepo) Update(sp *StatusPage) error {
	return r.db.Save(sp).Error
}

// Delete removes a status page. Monitors are NOT deleted — their status_page_id is set to NULL.
func (r *StatusPageRepo) Delete(id uint) error {
	// Clear status_page_id on associated monitors
	if err := r.db.Model(&Monitor{}).Where("status_page_id = ?", id).
		Update("status_page_id", nil).Error; err != nil {
		return fmt.Errorf("failed to unlink monitors: %w", err)
	}
	return r.db.Delete(&StatusPage{}, id).Error
}

// AddMonitor assigns a monitor to a status page.
func (r *StatusPageRepo) AddMonitor(pageID, monitorID uint) error {
	return r.db.Model(&Monitor{}).Where("id = ?", monitorID).
		Update("status_page_id", pageID).Error
}

// RemoveMonitor removes a monitor from a status page.
func (r *StatusPageRepo) RemoveMonitor(monitorID uint) error {
	return r.db.Model(&Monitor{}).Where("id = ?", monitorID).
		Update("status_page_id", nil).Error
}

// slugify converts a name to a URL-friendly slug.
var nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := strings.ToLower(strings.TrimSpace(s))
	slug = nonAlphanumRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}
