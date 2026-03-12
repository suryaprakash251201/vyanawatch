package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/alert"
	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/db"
	"github.com/vyanawatch/vyanawatch/monitor"
	"gopkg.in/yaml.v3"
)

type testEmailRequest struct {
	Event      string `json:"event"`
	Monitor    string `json:"monitor_name"`
	URL        string `json:"url"`
	Hostname   string `json:"hostname"`
	Reason     string `json:"reason"`
	StatusCode int    `json:"status_code"`
}

// settingsResponse is the shape returned by GET /api/v1/settings.
type settingsResponse struct {
	Email    emailSettings    `json:"email"`
	Telegram telegramSettings `json:"telegram"`
	Discord  discordSettings  `json:"discord"`
	Webhook  webhookSettings  `json:"webhook"`
}

type emailSettings struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type telegramSettings struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
	ChatID  string `json:"chat_id"`
}

type discordSettings struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

type webhookSettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Method  string `json:"method"`
	Headers string `json:"headers"`
}

// handleGetSettings returns the current alerting configuration.
// GET /api/v1/settings
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	resp := settingsResponse{
		Email: emailSettings{
			Enabled:  cfg.Alerting.Email.Enabled,
			Host:     cfg.Alerting.Email.Host,
			Port:     cfg.Alerting.Email.Port,
			Username: cfg.Alerting.Email.Username,
			Password: cfg.Alerting.Email.Password,
			From:     cfg.Alerting.Email.From,
			To:       cfg.Alerting.Email.To,
		},
		Telegram: telegramSettings{
			Enabled: cfg.Alerting.Telegram.Enabled,
			Token:   cfg.Alerting.Telegram.Token,
			ChatID:  cfg.Alerting.Telegram.ChatID,
		},
		Discord: discordSettings{
			Enabled:    cfg.Alerting.Discord.Enabled,
			WebhookURL: cfg.Alerting.Discord.WebhookURL,
		},
		Webhook: webhookSettings{
			Enabled: cfg.Alerting.Webhook.Enabled,
			URL:     cfg.Alerting.Webhook.URL,
			Method:  cfg.Alerting.Webhook.Method,
			Headers: cfg.Alerting.Webhook.Headers,
		},
	}
	respondJSON(w, http.StatusOK, resp)
}

// handleUpdateSettings updates the alerting configuration and saves to config.yaml.
// PUT /api/v1/settings
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cfg := config.Get()

	// Update alerting config in memory
	cfg.Alerting.Email.Enabled = req.Email.Enabled
	cfg.Alerting.Email.Host = req.Email.Host
	cfg.Alerting.Email.Port = req.Email.Port
	cfg.Alerting.Email.Username = req.Email.Username
	if req.Email.Password != "" {
		cfg.Alerting.Email.Password = req.Email.Password
	}
	cfg.Alerting.Email.From = req.Email.From
	cfg.Alerting.Email.To = req.Email.To

	cfg.Alerting.Telegram.Enabled = req.Telegram.Enabled
	cfg.Alerting.Telegram.Token = req.Telegram.Token
	cfg.Alerting.Telegram.ChatID = req.Telegram.ChatID

	cfg.Alerting.Discord.Enabled = req.Discord.Enabled
	cfg.Alerting.Discord.WebhookURL = req.Discord.WebhookURL

	cfg.Alerting.Webhook.Enabled = req.Webhook.Enabled
	cfg.Alerting.Webhook.URL = req.Webhook.URL
	if req.Webhook.Method == "" {
		req.Webhook.Method = "POST"
	}
	cfg.Alerting.Webhook.Method = req.Webhook.Method
	cfg.Alerting.Webhook.Headers = req.Webhook.Headers

	// Persist to config.yaml
	if err := saveConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save settings: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Settings saved successfully"})
}

// saveConfig writes the current config to config.yaml.
func saveConfig(cfg *config.Config) error {
	// Build a clean YAML structure (only write what we manage)
	data := map[string]interface{}{
		"server": map[string]interface{}{
			"port":      cfg.Server.Port,
			"host":      cfg.Server.Host,
			"log_level": cfg.Server.LogLevel,
		},
		"database": map[string]interface{}{
			"driver": cfg.Database.Driver,
			"dsn":    cfg.Database.DSN,
		},
		"auth": map[string]interface{}{
			"enabled":  cfg.Auth.Enabled,
			"username": cfg.Auth.Username,
			"password": cfg.Auth.Password,
		},
		"alerting": map[string]interface{}{
			"email": map[string]interface{}{
				"enabled":  cfg.Alerting.Email.Enabled,
				"host":     cfg.Alerting.Email.Host,
				"port":     cfg.Alerting.Email.Port,
				"username": cfg.Alerting.Email.Username,
				"password": cfg.Alerting.Email.Password,
				"from":     cfg.Alerting.Email.From,
				"to":       cfg.Alerting.Email.To,
			},
			"telegram": map[string]interface{}{
				"enabled": cfg.Alerting.Telegram.Enabled,
				"token":   cfg.Alerting.Telegram.Token,
				"chat_id": cfg.Alerting.Telegram.ChatID,
			},
			"discord": map[string]interface{}{
				"enabled":     cfg.Alerting.Discord.Enabled,
				"webhook_url": cfg.Alerting.Discord.WebhookURL,
			},
			"webhook": map[string]interface{}{
				"enabled": cfg.Alerting.Webhook.Enabled,
				"url":     cfg.Alerting.Webhook.URL,
				"method":  cfg.Alerting.Webhook.Method,
				"headers": cfg.Alerting.Webhook.Headers,
			},
		},
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	configPath := "config.yaml"
	if p := os.Getenv("VYANAWATCH_CONFIG"); p != "" {
		configPath = p
	}

	dir := filepath.Dir(configPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}

	return os.WriteFile(configPath, out, 0600)
}

// handleSendTestEmail sends a test email using current SMTP settings.
// POST /api/v1/settings/test-email
func (s *Server) handleSendTestEmail(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if !cfg.Alerting.Email.Enabled {
		respondError(w, http.StatusBadRequest, "Email alert channel is disabled")
		return
	}

	req := testEmailRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	event := buildSampleEmailEvent(req)
	notifier := alert.NewEmailNotifier(cfg.Alerting.Email)
	if err := notifier.Send(event); err != nil {
		respondError(w, http.StatusBadGateway, "Failed to send test email: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Test email sent successfully"})
}

// handlePreviewEmailTemplate renders the HTML email template in browser.
// GET /api/v1/settings/email-preview?event=down&site=My%20Site
func (s *Server) handlePreviewEmailTemplate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := testEmailRequest{
		Event:      q.Get("event"),
		Monitor:    q.Get("site"),
		URL:        q.Get("url"),
		Hostname:   q.Get("host"),
		Reason:     q.Get("reason"),
		StatusCode: 502,
	}

	event := buildSampleEmailEvent(req)
	html, err := alert.RenderEmailHTML(event)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to render template")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func buildSampleEmailEvent(req testEmailRequest) monitor.Event {
	eventType := monitor.EventDown
	if strings.EqualFold(strings.TrimSpace(req.Event), string(monitor.EventRecovery)) {
		eventType = monitor.EventRecovery
	}

	name := strings.TrimSpace(req.Monitor)
	if name == "" {
		name = "My Website"
	}
	url := strings.TrimSpace(req.URL)
	if url == "" {
		url = "https://example.com"
	}
	host := strings.TrimSpace(req.Hostname)
	if host == "" {
		host = "example.com"
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "Expected status 200, got 502"
	}
	sc := req.StatusCode
	if sc <= 0 {
		sc = 502
	}

	event := monitor.Event{
		Type: eventType,
		Monitor: db.Monitor{
			ID:                 0,
			Name:               name,
			Type:               db.MonitorHTTP,
			URL:                url,
			Hostname:           host,
			ExpectedStatusCode: 200,
		},
		Result: monitor.CheckResult{
			Status:       db.StatusDown,
			ResponseTime: 850,
			StatusCode:   sc,
			Message:      reason,
		},
		Timestamp: time.Now().UTC(),
	}

	if eventType == monitor.EventRecovery {
		event.Result.Status = db.StatusUp
		event.Result.StatusCode = 200
		event.Result.Message = "HTTP 200 OK"
		event.Incident = &db.Incident{Duration: 305}
	}

	return event
}
