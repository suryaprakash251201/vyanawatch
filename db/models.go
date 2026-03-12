package db

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

// MonitorType defines the type of check to perform.
type MonitorType string

const (
	MonitorHTTP      MonitorType = "http"
	MonitorPing      MonitorType = "ping"
	MonitorTCP       MonitorType = "tcp"
	MonitorDNS       MonitorType = "dns"
	MonitorPush      MonitorType = "push"
)

// MonitorStatus represents the current status of a monitor.
type MonitorStatus string

const (
	StatusUp      MonitorStatus = "up"
	StatusDown    MonitorStatus = "down"
	StatusPending MonitorStatus = "pending"
	StatusPaused  MonitorStatus = "paused"
)

// Monitor represents a monitoring target.
type Monitor struct {
	ID        uint          `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`

	// Basic info
	Name   string      `gorm:"not null" json:"name"`
	Type   MonitorType `gorm:"not null;default:'http'" json:"type"`
	Active bool        `gorm:"not null;default:true" json:"active"`

	// Target configuration
	URL      string `json:"url,omitempty"`       // For HTTP monitors
	Hostname string `json:"hostname,omitempty"`  // For ping/TCP/DNS
	Port     int    `json:"port,omitempty"`      // For TCP monitors
	DNSType  string `json:"dns_type,omitempty"`  // A, AAAA, CNAME, MX, etc.

	// Check configuration
	Interval       int `gorm:"not null;default:60" json:"interval"`          // seconds
	Timeout        int `gorm:"not null;default:10" json:"timeout"`           // seconds
	Retries        int `gorm:"not null;default:3" json:"retries"`            // number of retries before alert
	RetryInterval  int `gorm:"not null;default:10" json:"retry_interval"`    // seconds between retries

	// HTTP-specific
	Method             string `gorm:"default:'GET'" json:"method,omitempty"`
	ExpectedStatusCode int    `gorm:"default:200" json:"expected_status_code,omitempty"`
	KeywordCheck       string `json:"keyword_check,omitempty"`       // keyword to look for in response body
	KeywordPresent     bool   `gorm:"default:true" json:"keyword_present"` // true=must contain, false=must not contain
	Headers            string `json:"headers,omitempty"`              // JSON string of custom headers
	Body               string `json:"body,omitempty"`                 // request body for POST/PUT

	// SSL check
	SSLCheck          bool `gorm:"default:false" json:"ssl_check"`
	SSLExpiryDays     int  `gorm:"default:30" json:"ssl_expiry_days"`         // alert X days before expiry

	// Push/Heartbeat
	PushToken string `gorm:"uniqueIndex" json:"push_token,omitempty"` // unique token for push monitor

	// Current state (denormalized for fast reads)
	Status       MonitorStatus `gorm:"not null;default:'pending'" json:"status"`
	LastCheckAt  *time.Time    `json:"last_check_at,omitempty"`
	ResponseTime int64         `json:"response_time,omitempty"` // milliseconds

	// Alert settings
	AlertEnabled bool   `gorm:"default:true" json:"alert_enabled"`
	AlertChannels string `gorm:"default:''" json:"alert_channels"` // comma-separated: email,telegram,discord,webhook

	// Status Page
	StatusPageID *uint `json:"status_page_id,omitempty"`

	// Relationships
	Heartbeats []Heartbeat `gorm:"foreignKey:MonitorID;constraint:OnDelete:CASCADE" json:"heartbeats,omitempty"`
	Incidents  []Incident  `gorm:"foreignKey:MonitorID;constraint:OnDelete:CASCADE" json:"incidents,omitempty"`
}

// Heartbeat records a single check result.
type Heartbeat struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`

	MonitorID    uint          `gorm:"not null;index" json:"monitor_id"`
	Status       MonitorStatus `gorm:"not null" json:"status"`
	ResponseTime int64         `json:"response_time"` // milliseconds
	StatusCode   int           `json:"status_code,omitempty"`
	Message      string        `json:"message,omitempty"` // error message or status text
	Ping         float64       `json:"ping,omitempty"`    // for ICMP ping (ms)
}

// Incident records a downtime event.
type Incident struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	MonitorID  uint       `gorm:"not null;index" json:"monitor_id"`
	StartedAt  time.Time  `gorm:"not null" json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Duration   int64      `json:"duration,omitempty"`  // seconds
	RootCause  string     `json:"root_cause,omitempty"`
	Resolved   bool       `gorm:"default:false" json:"resolved"`
}

// StatusPage represents a public status page.
type StatusPage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name        string `gorm:"not null" json:"name"`
	Slug        string `gorm:"uniqueIndex;not null" json:"slug"` // URL-friendly name
	Description string `json:"description,omitempty"`
	Published   bool   `gorm:"default:false" json:"published"`
	CustomCSS   string `json:"custom_css,omitempty"`

	// Relationships
	Monitors []Monitor `gorm:"foreignKey:StatusPageID" json:"monitors,omitempty"`
}

// EventLog records a persistent event (state changes, actions, etc.).
type EventLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	MonitorID   uint      `gorm:"index" json:"monitor_id"`
	MonitorName string    `json:"monitor_name"`
	Status      string    `json:"status"`  // up, down, paused, pending
	EventType   string    `json:"event_type"` // down, recovery, created, deleted, paused, resumed
	Message     string    `json:"message"`
}

// UptimeStats holds computed uptime statistics.
type UptimeStats struct {
	Uptime24h  float64 `json:"uptime_24h"`
	Uptime7d   float64 `json:"uptime_7d"`
	Uptime30d  float64 `json:"uptime_30d"`
	AvgLatency float64 `json:"avg_latency"`
}

// MonitorWithStats combines a monitor with its computed uptime statistics.
type MonitorWithStats struct {
	Monitor
	UptimeStats UptimeStats `json:"uptime_stats"`
}

// Validate checks that a Monitor has valid configuration for its type.
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
		// Push token is auto-generated
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

// BeforeCreate generates a unique push token for every monitor.
// Push monitors use this token for check-in; other types get one for uniqueness.
func (m *Monitor) BeforeCreate(tx *gorm.DB) error {
	if m.PushToken == "" {
		token, err := generateToken(16)
		if err != nil {
			return fmt.Errorf("failed to generate push token: %w", err)
		}
		m.PushToken = token
	}
	return nil
}

// generateToken creates a cryptographically random hex token.
func generateToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IsDown returns true if the monitor's current status is down.
func (m *Monitor) IsDown() bool {
	return m.Status == StatusDown
}

// IsUp returns true if the monitor's current status is up.
func (m *Monitor) IsUp() bool {
	return m.Status == StatusUp
}

// GetAlertChannelList returns the alert channels as a string slice.
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
