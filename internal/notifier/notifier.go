package notifier

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/internal/config"
	"github.com/vyanawatch/vyanawatch/internal/engine"
)

type Notifier interface {
	Name() string
	Send(event engine.Event) error
}

type Dispatcher struct {
	notifiers map[string]Notifier
	mu        sync.RWMutex
}

func NewDispatcher(cfg *config.AlertingConfig) *Dispatcher {
	d := &Dispatcher{
		notifiers: make(map[string]Notifier),
	}

	if cfg.Email.Enabled {
		d.Register(NewEmailNotifier(cfg.Email))
		log.Info().Msg("Alert channel registered: email")
	}
	if cfg.Telegram.Enabled {
		d.Register(NewTelegramNotifier(cfg.Telegram))
		log.Info().Msg("Alert channel registered: telegram")
	}
	if cfg.Discord.Enabled {
		d.Register(NewDiscordNotifier(cfg.Discord))
		log.Info().Msg("Alert channel registered: discord")
	}
	if cfg.Webhook.Enabled {
		d.Register(NewWebhookNotifier(cfg.Webhook))
		log.Info().Msg("Alert channel registered: webhook")
	}

	log.Info().Int("channels", len(d.notifiers)).Msg("Alert dispatcher initialized")
	return d
}

func (d *Dispatcher) Register(n Notifier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notifiers[n.Name()] = n
}

func (d *Dispatcher) Dispatch(event engine.Event) {
	if event.Type != engine.EventDown && event.Type != engine.EventRecovery {
		return
	}

	m := &event.Monitor
	if !m.AlertEnabled {
		log.Debug().Uint("id", m.ID).Msg("Alert disabled for monitor, skipping")
		return
	}

	channels := m.GetAlertChannelList()

	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(channels) == 0 {
		for _, n := range d.notifiers {
			d.send(n, event)
		}
		return
	}

	for _, ch := range channels {
		n, ok := d.notifiers[ch]
		if !ok {
			log.Warn().Str("channel", ch).Uint("monitor_id", m.ID).
				Msg("Alert channel not configured or disabled")
			continue
		}
		d.send(n, event)
	}
}

func (d *Dispatcher) send(n Notifier, event engine.Event) {
	go func() {
		if err := n.Send(event); err != nil {
			log.Error().Err(err).
				Str("channel", n.Name()).
				Uint("monitor_id", event.Monitor.ID).
				Str("monitor", event.Monitor.Name).
				Msg("Failed to send alert")
		} else {
			log.Info().
				Str("channel", n.Name()).
				Uint("monitor_id", event.Monitor.ID).
				Str("event", string(event.Type)).
				Msg("Alert sent")
		}
	}()
}

func FormatSubject(event engine.Event) string {
	switch event.Type {
	case engine.EventDown:
		return fmt.Sprintf("[DOWN] %s is down", event.Monitor.Name)
	case engine.EventRecovery:
		return fmt.Sprintf("[RECOVERED] %s is back up", event.Monitor.Name)
	default:
		return fmt.Sprintf("[%s] %s", event.Type, event.Monitor.Name)
	}
}

func FormatPlainText(event engine.Event) string {
	m := event.Monitor
	r := event.Result

	switch event.Type {
	case engine.EventDown:
		msg := fmt.Sprintf("Monitor DOWN: %s\n\nType: %s\nReason: %s\nResponse Time: %dms\nTime: %s",
			m.Name, m.Type, r.Message, r.ResponseTime,
			event.Timestamp.Format("2006-01-02 15:04:05 UTC"))
		if m.URL != "" {
			msg += fmt.Sprintf("\nURL: %s", m.URL)
		}
		if m.Hostname != "" {
			msg += fmt.Sprintf("\nHost: %s", m.Hostname)
		}
		return msg

	case engine.EventRecovery:
		msg := fmt.Sprintf("Monitor RECOVERED: %s\n\nType: %s\nResponse Time: %dms\nTime: %s",
			m.Name, m.Type, r.ResponseTime,
			event.Timestamp.Format("2006-01-02 15:04:05 UTC"))
		if event.Incident != nil && event.Incident.Duration > 0 {
			msg += fmt.Sprintf("\nDowntime: %s", formatDuration(event.Incident.Duration))
		}
		return msg

	default:
		return fmt.Sprintf("Monitor: %s\nEvent: %s\nMessage: %s", m.Name, event.Type, r.Message)
	}
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
