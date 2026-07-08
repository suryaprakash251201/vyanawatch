package repo

import (
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
)

type MonitorRepository interface {
	Create(m *model.Monitor) error
	GetByID(id uint) (*model.Monitor, error)
	GetAll() ([]model.Monitor, error)
	GetActive() ([]model.Monitor, error)
	Update(m *model.Monitor) error
	Delete(id uint) error
	UpdateStatus(id uint, status model.MonitorStatus, responseTime int64, certExpiry *time.Time) error
	GetByPushToken(token string) (*model.Monitor, error)
	GetByStatusPageID(pageID uint) ([]model.Monitor, error)
	Count() (int64, error)
	CountByStatus(status model.MonitorStatus) (int64, error)
	GetAllWithStats() ([]model.MonitorWithStats, error)
	GetByIDWithStats(id uint) (*model.MonitorWithStats, error)
	GetSummary() (*Summary, error)
}

type HeartbeatRepository interface {
	Create(hb *model.Heartbeat) error
	GetByMonitorID(monitorID uint, limit int) ([]model.Heartbeat, error)
	GetByMonitorIDSince(monitorID uint, since time.Time) ([]model.Heartbeat, error)
	GetLatest(monitorID uint) (*model.Heartbeat, error)
	GetUptimeStats(monitorID uint) (model.UptimeStats, error)
	GetResponseTimeHistory(monitorID uint, since time.Time, limit int) ([]model.Heartbeat, error)
	DeleteOlderThan(age time.Duration) (int64, error)
	CountByMonitorID(monitorID uint) (int64, error)
}

type IncidentRepository interface {
	Create(inc *model.Incident) error
	GetByID(id uint) (*model.Incident, error)
	GetByMonitorID(monitorID uint, limit int) ([]model.Incident, error)
	GetOpenByMonitorID(monitorID uint) (*model.Incident, error)
	OpenIncident(monitorID uint, rootCause string) (*model.Incident, error)
	ResolveIncident(monitorID uint) (*model.Incident, error)
	GetRecent(limit int) ([]model.Incident, error)
	GetOpenCount() (int64, error)
	DeleteByMonitorID(monitorID uint) error
}

type StatusPageRepository interface {
	Create(sp *model.StatusPage) error
	GetByID(id uint) (*model.StatusPage, error)
	GetBySlug(slug string) (*model.StatusPage, error)
	GetAll() ([]model.StatusPage, error)
	Update(sp *model.StatusPage) error
	Delete(id uint) error
	AddMonitor(pageID, monitorID uint) error
	RemoveMonitor(monitorID uint) error
}

type EventLogRepository interface {
	Create(e *model.EventLog) error
	GetRecent(limit int) ([]model.EventLog, error)
	GetByMonitor(monitorID uint, limit int) ([]model.EventLog, error)
	DeleteAll() error
	DeleteOlderThan(retention time.Duration) (int64, error)
}

type TagRepository interface {
	Create(t *model.Tag) error
	GetAll() ([]model.Tag, error)
	GetByID(id uint) (*model.Tag, error)
	Update(t *model.Tag) error
	Delete(id uint) error
	GetByMonitorID(monitorID uint) ([]model.Tag, error)
	SetMonitorTags(monitorID uint, tagIDs []uint) error
}

type MaintenanceRepository interface {
	Create(m *model.Maintenance) error
	GetByID(id uint) (*model.Maintenance, error)
	GetAll() ([]model.Maintenance, error)
	GetActive() ([]model.Maintenance, error)
	Update(m *model.Maintenance) error
	Delete(id uint) error
	IsMonitorInMaintenance(monitorID uint) (bool, error)
}

type Summary struct {
	Total   int64 `json:"total"`
	Up      int64 `json:"up"`
	Down    int64 `json:"down"`
	Paused  int64 `json:"paused"`
	Pending int64 `json:"pending"`
}

type DBInit interface {
	Init() error
	GetDB() interface{}
}
