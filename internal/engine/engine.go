package engine

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/internal/model"
	"github.com/vyanawatch/vyanawatch/internal/repo"
)

type CheckResult struct {
	Status       model.MonitorStatus
	ResponseTime int64
	StatusCode   int
	Message      string
	Ping         float64
	CertExpiry   *time.Time
}

type Checker interface {
	Check(ctx context.Context, m *model.Monitor) CheckResult
}

type EventType string

const (
	EventDown     EventType = "down"
	EventUp       EventType = "up"
	EventRecovery EventType = "recovery"
)

type Event struct {
	Type      EventType
	Monitor   model.Monitor
	Incident  *model.Incident
	Result    CheckResult
	Timestamp time.Time
}

type EventHandler func(Event)

type Engine struct {
	repo     repo.MonitorRepository
	hbRepo   repo.HeartbeatRepository
	incRepo  repo.IncidentRepository
	maintRepo repo.MaintenanceRepository
	checkers map[model.MonitorType]Checker
	workers  map[uint]context.CancelFunc
	mu       sync.RWMutex
	handler  EventHandler
	proxyURL string
}

type Option func(*Engine)

func WithProxy(proxyURL string) Option {
	return func(e *Engine) {
		e.proxyURL = proxyURL
	}
}

func NewEngine(
	monitorRepo repo.MonitorRepository,
	heartbeatRepo repo.HeartbeatRepository,
	incidentRepo repo.IncidentRepository,
	maintenanceRepo repo.MaintenanceRepository,
	handler EventHandler,
	opts ...Option,
) *Engine {
	e := &Engine{
		repo:      monitorRepo,
		hbRepo:    heartbeatRepo,
		incRepo:   incidentRepo,
		maintRepo: maintenanceRepo,
		checkers:  make(map[model.MonitorType]Checker),
		workers:   make(map[uint]context.CancelFunc),
		handler:   handler,
	}

	e.checkers[model.MonitorHTTP] = NewHTTPChecker()
	e.checkers[model.MonitorPing] = &PingChecker{}
	e.checkers[model.MonitorTCP] = &TCPChecker{}
	e.checkers[model.MonitorDNS] = &DNSChecker{}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

func (e *Engine) SetEventHandler(h EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = h
}

func (e *Engine) Start(ctx context.Context) error {
	monitors, err := e.repo.GetActive()
	if err != nil {
		return err
	}

	log.Info().Int("count", len(monitors)).Msg("Starting monitoring engine")

	for i := range monitors {
		m := monitors[i]
		if m.Type == model.MonitorPush {
			continue
		}
		e.startWorker(ctx, m)
	}

	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for id, cancel := range e.workers {
		cancel()
		delete(e.workers, id)
	}
	log.Info().Msg("Monitoring engine stopped")
}

func (e *Engine) AddMonitor(ctx context.Context, m model.Monitor) {
	if m.Type == model.MonitorPush {
		return
	}
	e.stopWorker(m.ID)
	e.startWorker(ctx, m)
}

func (e *Engine) RemoveMonitor(id uint) {
	e.stopWorker(id)
}

func (e *Engine) RestartMonitor(ctx context.Context, m model.Monitor) {
	e.stopWorker(m.ID)
	if m.Active && m.Type != model.MonitorPush {
		e.startWorker(ctx, m)
	}
}

func (e *Engine) HandlePush(m *model.Monitor) error {
	now := time.Now()
	prevStatus := m.Status
	newStatus := model.StatusUp

	hb := &model.Heartbeat{
		MonitorID:    m.ID,
		Status:       newStatus,
		ResponseTime: 0,
		Message:      "Push received",
	}
	if err := e.hbRepo.Create(hb); err != nil {
		return err
	}

	if err := e.repo.UpdateStatus(m.ID, newStatus, 0, nil); err != nil {
		return err
	}

	if prevStatus == model.StatusDown {
		inc, err := e.incRepo.ResolveIncident(m.ID)
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

func (e *Engine) startWorker(ctx context.Context, m model.Monitor) {
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

func (e *Engine) stopWorker(id uint) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if cancel, ok := e.workers[id]; ok {
		cancel()
		delete(e.workers, id)
		log.Info().Uint("id", id).Msg("Stopped monitor worker")
	}
}

func (e *Engine) runWorker(ctx context.Context, m model.Monitor, checker Checker) {
	e.executeCheck(ctx, m, checker)

	ticker := time.NewTicker(time.Duration(m.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fresh, err := e.repo.GetByID(m.ID)
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

func (e *Engine) executeCheck(ctx context.Context, m model.Monitor, checker Checker) {
	inMaintenance, err := e.maintRepo.IsMonitorInMaintenance(m.ID)
	if err == nil && inMaintenance {
		log.Debug().Uint("id", m.ID).Msg("Monitor in maintenance window, skipping check")
		return
	}

	var result CheckResult

	retries := m.Retries
	if retries < 1 {
		retries = 1
	}

	for attempt := 0; attempt < retries; attempt++ {
		result = checker.Check(ctx, &m)
		if result.Status == model.StatusUp {
			break
		}
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

	hb := &model.Heartbeat{
		MonitorID:    m.ID,
		Status:       result.Status,
		ResponseTime: result.ResponseTime,
		StatusCode:   result.StatusCode,
		Message:      result.Message,
		Ping:         result.Ping,
		CertExpiry:   result.CertExpiry,
	}
	if err := e.hbRepo.Create(hb); err != nil {
		log.Error().Err(err).Uint("id", m.ID).Msg("Failed to save heartbeat")
	}

	if err := e.repo.UpdateStatus(m.ID, result.Status, result.ResponseTime, result.CertExpiry); err != nil {
		log.Error().Err(err).Uint("id", m.ID).Msg("Failed to update monitor status")
	}

	prevStatus := m.Status

	switch {
	case prevStatus != model.StatusDown && result.Status == model.StatusDown:
		log.Warn().Uint("id", m.ID).Str("name", m.Name).Str("reason", result.Message).
			Msg("Monitor is DOWN")

		inc, err := e.incRepo.OpenIncident(m.ID, result.Message)
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

	case prevStatus == model.StatusDown && result.Status == model.StatusUp:
		log.Info().Uint("id", m.ID).Str("name", m.Name).Msg("Monitor RECOVERED")

		inc, err := e.incRepo.ResolveIncident(m.ID)
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
