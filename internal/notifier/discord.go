package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/engine"
)

type DiscordNotifier struct {
	cfg    config.DiscordConfig
	client *http.Client
}

func NewDiscordNotifier(cfg config.DiscordConfig) *DiscordNotifier {
	return &DiscordNotifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (d *DiscordNotifier) Name() string { return "discord" }

func (d *DiscordNotifier) Send(event engine.Event) error {
	embed := buildDiscordEmbed(event)
	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequest("POST", d.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VyanaWatch/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discord webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func buildDiscordEmbed(event engine.Event) map[string]interface{} {
	m := event.Monitor
	r := event.Result

	var color int
	var title string
	switch event.Type {
	case engine.EventDown:
		color = 0xe74c3c
		title = fmt.Sprintf("🔴 DOWN — %s", m.Name)
	case engine.EventRecovery:
		color = 0x2ecc71
		title = fmt.Sprintf("🟢 RECOVERED — %s", m.Name)
	default:
		color = 0x3498db
		title = fmt.Sprintf("%s — %s", event.Type, m.Name)
	}

	fields := []map[string]interface{}{
		{"name": "Type", "value": string(m.Type), "inline": true},
		{"name": "Response Time", "value": fmt.Sprintf("%dms", r.ResponseTime), "inline": true},
	}

	if m.URL != "" {
		fields = append(fields, map[string]interface{}{
			"name": "URL", "value": m.URL, "inline": false,
		})
	}
	if m.Hostname != "" {
		fields = append(fields, map[string]interface{}{
			"name": "Host", "value": m.Hostname, "inline": true,
		})
	}
	if event.Type == engine.EventDown {
		fields = append(fields, map[string]interface{}{
			"name": "Reason", "value": r.Message, "inline": false,
		})
	}
	if event.Incident != nil && event.Incident.Duration > 0 {
		fields = append(fields, map[string]interface{}{
			"name": "Downtime", "value": formatDuration(event.Incident.Duration), "inline": true,
		})
	}

	return map[string]interface{}{
		"title":  title,
		"color":  color,
		"fields": fields,
		"footer": map[string]string{
			"text": "VyanaWatch",
		},
		"timestamp": event.Timestamp.Format(time.RFC3339),
	}
}
