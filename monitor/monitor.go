package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/db"
)

// CheckResult holds the outcome of a single monitor check.
type CheckResult struct {
	Status       db.MonitorStatus
	ResponseTime int64   // milliseconds
	StatusCode   int     // HTTP status code (0 for non-HTTP)
	Message      string  // error message or status text
	Ping         float64 // ICMP ping in ms (0 for non-ping)
}

// Checker is the interface each monitor type must implement.
type Checker interface {
	Check(ctx context.Context, m *db.Monitor) CheckResult
}

// EventType describes what happened to a monitor.
type EventType string

const (
	EventDown     EventType = "down"
	EventUp       EventType = "up"
	EventRecovery EventType = "recovery"
)

// Event is emitted when a monitor changes state.
type Event struct {
	Type      EventType
	Monitor   db.Monitor
	Incident  *db.Incident
	Result    CheckResult
	Timestamp time.Time
}

// EventHandler is called when a monitor state change occurs.
type EventHandler func(Event)

// Engine orchestrates all monitor goroutines.
type Engine struct {
	repos    *db.Repos
	checkers map[db.MonitorType]Checker
	workers  map[uint]context.CancelFunc // monitorID -> cancel function
	mu       sync.RWMutex
	handler  EventHandler
}

// SetEventHandler updates the event handler (e.g., to wire SSE broadcast after server init).
func (e *Engine) SetEventHandler(h EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = h
}

// NewEngine creates a new monitoring engine.
func NewEngine(repos *db.Repos, handler EventHandler) *Engine {
	e := &Engine{
		repos:    repos,
		checkers: make(map[db.MonitorType]Checker),
		workers:  make(map[uint]context.CancelFunc),
		handler:  handler,
	}

	// Register checker implementations
	e.checkers[db.MonitorHTTP] = &HTTPChecker{}
	e.checkers[db.MonitorPing] = &PingChecker{}
	e.checkers[db.MonitorTCP] = &TCPChecker{}
	e.checkers[db.MonitorDNS] = &DNSChecker{}
	// Push monitors don't have a checker — they receive heartbeats passively.

	return e
}

// Start loads all active monitors from the DB and starts a goroutine for each.
func (e *Engine) Start(ctx context.Context) error {
	monitors, err := e.repos.Monitors.GetActive()
	if err != nil {
		return err
	}

	log.Info().Int("count", len(monitors)).Msg("Starting monitoring engine")

	for i := range monitors {
		m := monitors[i]
		if m.Type == db.MonitorPush {
			continue // Push monitors are passive
		}
		e.startWorker(ctx, m)
	}

	return nil
}

// Stop cancels all running monitor goroutines.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for id, cancel := range e.workers {
		cancel()
		delete(e.workers, id)
	}
	log.Info().Msg("Monitoring engine stopped")
}

// AddMonitor starts monitoring a new or reactivated monitor.
func (e *Engine) AddMonitor(ctx context.Context, m db.Monitor) {
	if m.Type == db.MonitorPush {
		return
	}
	e.stopWorker(m.ID)
	e.startWorker(ctx, m)
}

// RemoveMonitor stops a monitor's goroutine.
func (e *Engine) RemoveMonitor(id uint) {
	e.stopWorker(id)
}

// RestartMonitor restarts a monitor (e.g. after config change).
func (e *Engine) RestartMonitor(ctx context.Context, m db.Monitor) {
	e.stopWorker(m.ID)
	if m.Active && m.Type != db.MonitorPush {
		e.startWorker(ctx, m)
	}
}

// HandlePush processes an incoming push/heartbeat check-in.
func (e *Engine) HandlePush(m *db.Monitor) error {
	now := time.Now()
	prevStatus := m.Status
	newStatus := db.StatusUp

	// Record heartbeat
	hb := &db.Heartbeat{
		MonitorID:    m.ID,
		Status:       newStatus,
		ResponseTime: 0,
		Message:      "Push received",
	}
	if err := e.repos.Heartbeats.Create(hb); err != nil {
		return err
	}

	// Update monitor status
	if err := e.repos.Monitors.UpdateStatus(m.ID, newStatus, 0); err != nil {
		return err
	}

	// Handle state transition: DOWN → UP (recovery)
	if prevStatus == db.StatusDown {
		inc, err := e.repos.Incidents.ResolveIncident(m.ID)
		if err == nil && e.handler != nil {
			e.handler(Event{
				Type:      EventRecovery,
				Monitor:   *m,
				Incident:  inc,
				Result:    CheckResult{Status: newStatus, Message: "Push received"},
				Timestamp: now,
			})
		}
	}

	return nil
}

// startWorker launches a goroutine that checks a monitor at its configured interval.
func (e *Engine) startWorker(ctx context.Context, m db.Monitor) {
	checker, ok := e.checkers[m.Type]
	if !ok {
		log.Error().Str("type", string(m.Type)).Uint("id", m.ID).Msg("No checker for monitor type")
		return
	}

	workerCtx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.workers[m.ID] = cancel
	e.mu.Unlock()

	log.Info().Uint("id", m.ID).Str("name", m.Name).Str("type", string(m.Type)).
		Int("interval", m.Interval).Msg("Starting monitor worker")

	go e.runWorker(workerCtx, m, checker)
}

// stopWorker cancels a running monitor goroutine.
func (e *Engine) stopWorker(id uint) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if cancel, ok := e.workers[id]; ok {
		cancel()
		delete(e.workers, id)
		log.Info().Uint("id", id).Msg("Stopped monitor worker")
	}
}

// runWorker is the main loop for a single monitor goroutine.
func (e *Engine) runWorker(ctx context.Context, m db.Monitor, checker Checker) {
	// Run first check immediately
	e.executeCheck(ctx, m, checker)

	ticker := time.NewTicker(time.Duration(m.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-fetch monitor from DB in case config changed
			fresh, err := e.repos.Monitors.GetByID(m.ID)
			if err != nil {
				log.Error().Err(err).Uint("id", m.ID).Msg("Failed to refresh monitor config")
				continue
			}
			if !fresh.Active {
				log.Info().Uint("id", m.ID).Msg("Monitor deactivated, stopping worker")
				return
			}
			m = *fresh
			e.executeCheck(ctx, m, checker)
		}
	}
}

// executeCheck runs the checker with retries, records results, and handles state transitions.
func (e *Engine) executeCheck(ctx context.Context, m db.Monitor, checker Checker) {
	var result CheckResult

	// Try check with retries
	retries := m.Retries
	if retries < 1 {
		retries = 1
	}

	for attempt := 0; attempt < retries; attempt++ {
		result = checker.Check(ctx, &m)
		if result.Status == db.StatusUp {
			break
		}
		// If not the last attempt, wait before retrying
		if attempt < retries-1 {
			retryWait := time.Duration(m.RetryInterval) * time.Second
			if retryWait <= 0 {
				retryWait = 10 * time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryWait):
			}
		}
	}

	// Record heartbeat
	hb := &db.Heartbeat{
		MonitorID:    m.ID,
		Status:       result.Status,
		ResponseTime: result.ResponseTime,
		StatusCode:   result.StatusCode,
		Message:      result.Message,
		Ping:         result.Ping,
	}
	if err := e.repos.Heartbeats.Create(hb); err != nil {
		log.Error().Err(err).Uint("id", m.ID).Msg("Failed to save heartbeat")
	}

	// Update denormalized status on monitor
	if err := e.repos.Monitors.UpdateStatus(m.ID, result.Status, result.ResponseTime); err != nil {
		log.Error().Err(err).Uint("id", m.ID).Msg("Failed to update monitor status")
	}

	prevStatus := m.Status

	// Handle state transitions
	switch {
	case prevStatus != db.StatusDown && result.Status == db.StatusDown:
		// Transition to DOWN — open incident
		log.Warn().Uint("id", m.ID).Str("name", m.Name).Str("reason", result.Message).
			Msg("Monitor is DOWN")

		inc, err := e.repos.Incidents.OpenIncident(m.ID, result.Message)
		if err != nil {
			log.Error().Err(err).Uint("id", m.ID).Msg("Failed to open incident")
		}
		if e.handler != nil {
			e.handler(Event{
				Type:      EventDown,
				Monitor:   m,
				Incident:  inc,
				Result:    result,
				Timestamp: time.Now(),
			})
		}

	case prevStatus == db.StatusDown && result.Status == db.StatusUp:
		// Transition to UP — resolve incident (recovery)
		log.Info().Uint("id", m.ID).Str("name", m.Name).Msg("Monitor RECOVERED")

		inc, err := e.repos.Incidents.ResolveIncident(m.ID)
		if err != nil {
			log.Error().Err(err).Uint("id", m.ID).Msg("Failed to resolve incident")
		}
		if e.handler != nil {
			e.handler(Event{
				Type:      EventRecovery,
				Monitor:   m,
				Incident:  inc,
				Result:    result,
				Timestamp: time.Now(),
			})
		}
	}

	log.Debug().Uint("id", m.ID).Str("name", m.Name).
		Str("status", string(result.Status)).Int64("ms", result.ResponseTime).
		Msg("Check completed")
}
