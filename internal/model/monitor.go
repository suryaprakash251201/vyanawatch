package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type MonitorType string

const (
	MonitorHTTP MonitorType = "http"
	MonitorPing MonitorType = "ping"
	MonitorTCP  MonitorType = "tcp"
	MonitorDNS  MonitorType = "dns"
	MonitorPush MonitorType = "push"
)

type MonitorStatus string

const (
	StatusUp      MonitorStatus = "up"
	StatusDown    MonitorStatus = "down"
	StatusPending MonitorStatus = "pending"
	StatusPaused  MonitorStatus = "paused"
)

type Monitor struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name   string        `gorm:"not null" json:"name"`
	Type   MonitorType   `gorm:"not null;default:'http'" json:"type"`
	Active bool          `gorm:"not null;default:true" json:"active"`

	URL      string `json:"url,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Port     int    `json:"port,omitempty"`
	DNSType  string `json:"dns_type,omitempty"`

	Interval       int `gorm:"not null;default:60" json:"interval"`
	Timeout        int `gorm:"not null;default:10" json:"timeout"`
	Retries        int `gorm:"not null;default:3" json:"retries"`
	RetryInterval  int `gorm:"not null;default:10" json:"retry_interval"`

	Method             string `gorm:"default:'GET'" json:"method,omitempty"`
	ExpectedStatusCode int    `gorm:"default:200" json:"expected_status_code,omitempty"`
	KeywordCheck       string `json:"keyword_check,omitempty"`
	KeywordPresent     bool   `gorm:"default:true" json:"keyword_present"`
	Headers            string `json:"headers,omitempty"`
	Body               string `json:"body,omitempty"`

	SSLCheck      bool `gorm:"default:false" json:"ssl_check"`
	SSLExpiryDays int  `gorm:"default:30" json:"ssl_expiry_days"`

	PushToken string `gorm:"uniqueIndex" json:"push_token,omitempty"`

	Status       MonitorStatus `gorm:"not null;default:'pending'" json:"status"`
	LastCheckAt  *time.Time    `json:"last_check_at,omitempty"`
	ResponseTime int64         `json:"response_time,omitempty"`
	CertExpiry   *time.Time    `json:"cert_expiry,omitempty"`

	AlertEnabled  bool   `gorm:"default:true" json:"alert_enabled"`
	AlertChannels string `gorm:"default:''" json:"alert_channels"`

	Tags []MonitorTag `gorm:"foreignKey:MonitorID;constraint:OnDelete:CASCADE" json:"tags,omitempty"`

	StatusPageID *uint `json:"status_page_id,omitempty"`

	Heartbeats []Heartbeat `gorm:"foreignKey:MonitorID;constraint:OnDelete:CASCADE" json:"heartbeats,omitempty"`
	Incidents  []Incident  `gorm:"foreignKey:MonitorID;constraint:OnDelete:CASCADE" json:"incidents,omitempty"`
}

type Tag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	Color     string    `gorm:"default:'#6b7280'" json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

type MonitorTag struct {
	MonitorID uint    `gorm:"primaryKey" json:"monitor_id"`
	TagID     uint    `gorm:"primaryKey" json:"tag_id"`
	Tag       Tag     `gorm:"foreignKey:TagID;constraint:OnDelete:CASCADE" json:"tag,omitempty"`
}

type Maintenance struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"not null" json:"title"`
	Description string    `json:"description,omitempty"`
	StartAt     time.Time `gorm:"not null" json:"start_at"`
	EndAt       time.Time `gorm:"not null" json:"end_at"`
	Active      bool      `gorm:"default:false" json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Monitors []MaintenanceMonitor `gorm:"foreignKey:MaintenanceID;constraint:OnDelete:CASCADE" json:"monitors,omitempty"`
}

type MaintenanceMonitor struct {
	MaintenanceID uint `gorm:"primaryKey" json:"maintenance_id"`
	MonitorID     uint `gorm:"primaryKey" json:"monitor_id"`
}

type Heartbeat struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`

	MonitorID    uint           `gorm:"not null;index" json:"monitor_id"`
	Status       MonitorStatus  `gorm:"not null" json:"status"`
	ResponseTime int64          `json:"response_time"`
	StatusCode   int            `json:"status_code,omitempty"`
	Message      string         `json:"message,omitempty"`
	Ping         float64        `json:"ping,omitempty"`
	CertExpiry   *time.Time     `json:"cert_expiry,omitempty"`
}

type Incident struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	MonitorID  uint       `gorm:"not null;index" json:"monitor_id"`
	StartedAt  time.Time  `gorm:"not null" json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Duration   int64      `json:"duration,omitempty"`
	RootCause  string     `json:"root_cause,omitempty"`
	Resolved   bool       `gorm:"default:false" json:"resolved"`
}

type StatusPage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name        string    `gorm:"not null" json:"name"`
	Slug        string    `gorm:"uniqueIndex;not null" json:"slug"`
	Description string    `json:"description,omitempty"`
	Published   bool      `gorm:"default:false" json:"published"`
	CustomCSS   string    `json:"custom_css,omitempty"`

	Monitors []Monitor `gorm:"foreignKey:StatusPageID" json:"monitors,omitempty"`
}

type EventLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	MonitorID   uint      `gorm:"index" json:"monitor_id"`
	MonitorName string    `json:"monitor_name"`
	Status      string    `json:"status"`
	EventType   string    `json:"event_type"`
	Message     string    `json:"message"`
}

type UptimeStats struct {
	Uptime24h  float64 `json:"uptime_24h"`
	Uptime7d   float64 `json:"uptime_7d"`
	Uptime30d  float64 `json:"uptime_30d"`
	AvgLatency float64 `json:"avg_latency"`
}

type MonitorWithStats struct {
	Monitor
	UptimeStats     UptimeStats `json:"uptime_stats"`
	RecentHeartbeats []Heartbeat `json:"recent_heartbeats,omitempty"`
}

func (m *Monitor) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("monitor name is required")
	}

	switch m.Type {
	case MonitorHTTP:
		if m.URL == "" {
			return fmt.Errorf("URL is required for HTTP monitors")
		}
		u, err := url.Parse(m.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("invalid URL: must start with http:// or https://")
		}
	case MonitorPing:
		if m.Hostname == "" {
			return fmt.Errorf("hostname is required for ping monitors")
		}
	case MonitorTCP:
		if m.Hostname == "" {
			return fmt.Errorf("hostname is required for TCP monitors")
		}
		if m.Port <= 0 || m.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535 for TCP monitors")
		}
	case MonitorDNS:
		if m.Hostname == "" {
			return fmt.Errorf("hostname is required for DNS monitors")
		}
	case MonitorPush:
	default:
		return fmt.Errorf("unsupported monitor type: %s", m.Type)
	}

	if m.Interval < 10 {
		return fmt.Errorf("interval must be at least 10 seconds")
	}
	if m.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second")
	}

	return nil
}

func GenerateToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Monitor) IsDown() bool {
	return m.Status == StatusDown
}

func (m *Monitor) IsUp() bool {
	return m.Status == StatusUp
}

func (m *Monitor) GetAlertChannelList() []string {
	if m.AlertChannels == "" {
		return nil
	}
	channels := strings.Split(m.AlertChannels, ",")
	result := make([]string, 0, len(channels))
	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if ch != "" {
			result = append(result, ch)
		}
	}
	return result
}
