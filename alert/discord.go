package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/monitor"
)

// DiscordNotifier sends alerts via Discord webhook.
type DiscordNotifier struct {
	cfg    config.DiscordConfig
	client http.Client
}

func (d *DiscordNotifier) Name() string { return "discord" }

func (d *DiscordNotifier) Send(event monitor.Event) error {
	if d.client.Timeout == 0 {
		d.client.Timeout = 10 * time.Second
	}

	embed := buildDiscordEmbed(event)
	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	resp, err := d.client.Post(d.cfg.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord webhook request: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discord webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func buildDiscordEmbed(event monitor.Event) map[string]interface{} {
	m := event.Monitor
	r := event.Result

	var color int
	var title string
	switch event.Type {
	case monitor.EventDown:
		color = 0xe74c3c // red
		title = fmt.Sprintf("🔴 DOWN — %s", m.Name)
	case monitor.EventRecovery:
		color = 0x2ecc71 // green
		title = fmt.Sprintf("🟢 RECOVERED — %s", m.Name)
	default:
		color = 0x3498db // blue
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
	if event.Type == monitor.EventDown {
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
