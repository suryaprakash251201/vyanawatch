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

// TelegramNotifier sends alerts via Telegram Bot API.
type TelegramNotifier struct {
	cfg    config.TelegramConfig
	client http.Client
}

func (t *TelegramNotifier) Name() string { return "telegram" }

func (t *TelegramNotifier) Send(event monitor.Event) error {
	if t.client.Timeout == 0 {
		t.client.Timeout = 10 * time.Second
	}

	text := formatTelegramMessage(event)

	payload := map[string]interface{}{
		"chat_id":    t.cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.cfg.Token)
	resp, err := t.client.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func formatTelegramMessage(event monitor.Event) string {
	m := event.Monitor
	r := event.Result

	switch event.Type {
	case monitor.EventDown:
		msg := fmt.Sprintf(
			"🔴 <b>Monitor DOWN</b>\n\n"+
				"<b>%s</b>\n"+
				"Type: %s\n"+
				"Reason: %s\n"+
				"Response: %dms\n"+
				"Time: %s",
			m.Name, m.Type, r.Message, r.ResponseTime,
			event.Timestamp.Format("2006-01-02 15:04:05"))
		if m.URL != "" {
			msg += fmt.Sprintf("\nURL: %s", m.URL)
		}
		if m.Hostname != "" {
			msg += fmt.Sprintf("\nHost: %s", m.Hostname)
		}
		return msg

	case monitor.EventRecovery:
		msg := fmt.Sprintf(
			"🟢 <b>Monitor RECOVERED</b>\n\n"+
				"<b>%s</b>\n"+
				"Type: %s\n"+
				"Response: %dms\n"+
				"Time: %s",
			m.Name, m.Type, r.ResponseTime,
			event.Timestamp.Format("2006-01-02 15:04:05"))
		if event.Incident != nil && event.Incident.Duration > 0 {
			msg += fmt.Sprintf("\nDowntime: %s", formatDuration(event.Incident.Duration))
		}
		return msg

	default:
		return fmt.Sprintf("<b>%s</b>: %s — %s", event.Type, m.Name, r.Message)
	}
}
