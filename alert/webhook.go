package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/monitor"
)

// WebhookNotifier sends alerts to a custom webhook URL.
type WebhookNotifier struct {
	cfg    config.WebhookConfig
	client http.Client
}

func (w *WebhookNotifier) Name() string { return "webhook" }

func (w *WebhookNotifier) Send(event monitor.Event) error {
	if w.client.Timeout == 0 {
		w.client.Timeout = 10 * time.Second
	}

	payload := webhookPayload{
		Event:        string(event.Type),
		MonitorID:    event.Monitor.ID,
		MonitorName:  event.Monitor.Name,
		MonitorType:  string(event.Monitor.Type),
		URL:          event.Monitor.URL,
		Hostname:     event.Monitor.Hostname,
		Status:       string(event.Result.Status),
		Message:      event.Result.Message,
		ResponseTime: event.Result.ResponseTime,
		Timestamp:    event.Timestamp.Format(time.RFC3339),
	}
	if event.Incident != nil {
		payload.IncidentID = event.Incident.ID
		payload.Downtime = event.Incident.Duration
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	method := strings.ToUpper(w.cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, w.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VyanaWatch/1.0")

	// Apply custom headers from config (JSON string)
	if w.cfg.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(w.cfg.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

type webhookPayload struct {
	Event        string `json:"event"`
	MonitorID    uint   `json:"monitor_id"`
	MonitorName  string `json:"monitor_name"`
	MonitorType  string `json:"monitor_type"`
	URL          string `json:"url,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	ResponseTime int64  `json:"response_time_ms"`
	Timestamp    string `json:"timestamp"`
	IncidentID   uint   `json:"incident_id,omitempty"`
	Downtime     int64  `json:"downtime_seconds,omitempty"`
}
