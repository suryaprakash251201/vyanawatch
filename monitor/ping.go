package monitor

import (
	"context"
	"fmt"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/vyanawatch/vyanawatch/db"
)

// PingChecker performs ICMP ping checks.
type PingChecker struct{}

// Check sends ICMP ping packets and measures latency/packet loss.
func (c *PingChecker) Check(ctx context.Context, m *db.Monitor) CheckResult {
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	pinger, err := probing.NewPinger(m.Hostname)
	if err != nil {
		return CheckResult{
			Status:  db.StatusDown,
			Message: fmt.Sprintf("Failed to create pinger: %s", err),
		}
	}

	pinger.Count = 3
	pinger.Timeout = timeout
	pinger.SetPrivileged(true) // Requires NET_RAW capability in Docker

	start := time.Now()
	err = pinger.RunWithContext(ctx)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("Ping failed: %s", err),
		}
	}

	stats := pinger.Statistics()

	// If all packets lost, host is down
	if stats.PacketLoss == 100 {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Ping:         0,
			Message:      fmt.Sprintf("100%% packet loss to %s", m.Hostname),
		}
	}

	avgMs := float64(stats.AvgRtt.Microseconds()) / 1000.0

	return CheckResult{
		Status:       db.StatusUp,
		ResponseTime: int64(avgMs),
		Ping:         avgMs,
		Message: fmt.Sprintf("Ping OK: %.1fms avg, %.0f%% loss (%d/%d)",
			avgMs, stats.PacketLoss, stats.PacketsRecv, stats.PacketsSent),
	}
}
