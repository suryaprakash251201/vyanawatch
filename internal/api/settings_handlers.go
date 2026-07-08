package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/engine"
	"github.com/vyanawatch/vyanawatch/internal/model"
	"github.com/vyanawatch/vyanawatch/internal/notifier"
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

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
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

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cfg := s.cfg.Get()

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

	if err := saveConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save settings: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Settings saved successfully"})
}

func saveConfig(cfg *config.Config) error {
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

func (s *Server) handleSendTestEmail(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
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
	notifier := notifier.NewEmailNotifier(cfg.Alerting.Email)
	if err := notifier.Send(event); err != nil {
		respondError(w, http.StatusBadGateway, "Failed to send test email: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Test email sent successfully"})
}

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
	html, err := notifier.RenderEmailHTML(event)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to render template")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleSendTestTelegram(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	if !cfg.Alerting.Telegram.Enabled {
		respondError(w, http.StatusBadRequest, "Telegram alert channel is disabled")
		return
	}

	event := buildSampleEvent("telegram")
	n := notifier.NewTelegramNotifier(cfg.Alerting.Telegram)
	if err := n.Send(event); err != nil {
		log.Error().Err(err).Msg("Test Telegram send failed")
		respondError(w, http.StatusBadGateway, "Failed to send test Telegram message: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Test Telegram message sent successfully"})
}

func (s *Server) handleSendTestDiscord(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	if !cfg.Alerting.Discord.Enabled {
		respondError(w, http.StatusBadRequest, "Discord alert channel is disabled")
		return
	}

	event := buildSampleEvent("discord")
	n := notifier.NewDiscordNotifier(cfg.Alerting.Discord)
	if err := n.Send(event); err != nil {
		log.Error().Err(err).Msg("Test Discord send failed")
		respondError(w, http.StatusBadGateway, "Failed to send test Discord message: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Test Discord message sent successfully"})
}

func (s *Server) handleSendTestWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	if !cfg.Alerting.Webhook.Enabled {
		respondError(w, http.StatusBadRequest, "Webhook alert channel is disabled")
		return
	}

	event := buildSampleEvent("webhook")
	n := notifier.NewWebhookNotifier(cfg.Alerting.Webhook)
	if err := n.Send(event); err != nil {
		log.Error().Err(err).Msg("Test webhook send failed")
		respondError(w, http.StatusBadGateway, "Failed to send test webhook: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Test webhook sent successfully"})
}

func buildSampleEvent(channel string) engine.Event {
	return engine.Event{
		Type: engine.EventDown,
		Monitor: model.Monitor{
			ID:                 0,
			Name:               "VyanaWatch Test",
			Type:               model.MonitorHTTP,
			URL:                "https://example.com",
			Hostname:           "example.com",
			ExpectedStatusCode: 200,
		},
		Result: engine.CheckResult{
			Status:       model.StatusDown,
			ResponseTime: 850,
			StatusCode:   502,
			Message:      "Test notification from VyanaWatch settings page — expected status 200, got 502",
		},
		Timestamp: time.Now().UTC(),
	}
}

func buildSampleEmailEvent(req testEmailRequest) engine.Event {
	eventType := engine.EventDown
	if strings.EqualFold(strings.TrimSpace(req.Event), string(engine.EventRecovery)) {
		eventType = engine.EventRecovery
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

	event := engine.Event{
		Type: eventType,
		Monitor: model.Monitor{
			ID:                 0,
			Name:               name,
			Type:               model.MonitorHTTP,
			URL:                url,
			Hostname:           host,
			ExpectedStatusCode: 200,
		},
		Result: engine.CheckResult{
			Status:       model.StatusDown,
			ResponseTime: 850,
			StatusCode:   sc,
			Message:      reason,
		},
		Timestamp: time.Now().UTC(),
	}

	if eventType == engine.EventRecovery {
		event.Result.Status = model.StatusUp
		event.Result.StatusCode = 200
		event.Result.Message = "HTTP 200 OK"
		event.Incident = &model.Incident{Duration: 305}
	}

	return event
}
