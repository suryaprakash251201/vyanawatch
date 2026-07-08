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

type TelegramNotifier struct {
	cfg    config.TelegramConfig
	client *http.Client
}

func NewTelegramNotifier(cfg config.TelegramConfig) *TelegramNotifier {
	return &TelegramNotifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
	}
}

func (t *TelegramNotifier) Name() string { return "telegram" }

func (t *TelegramNotifier) Send(event engine.Event) error {
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
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VyanaWatch/1.0")

	resp, err := t.client.Do(req)
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

func formatTelegramMessage(event engine.Event) string {
	m := event.Monitor
	r := event.Result

	switch event.Type {
	case engine.EventDown:
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

	case engine.EventRecovery:
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
