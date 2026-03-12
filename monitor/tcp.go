package monitor

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/vyanawatch/vyanawatch/db"
)

// TCPChecker performs TCP port connectivity checks.
type TCPChecker struct{}

// Check attempts a TCP connection to the target host:port.
func (c *TCPChecker) Check(ctx context.Context, m *db.Monitor) CheckResult {
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", m.Hostname, m.Port)

	start := time.Now()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("TCP connection to %s failed: %s", addr, err),
		}
	}
	conn.Close()

	return CheckResult{
		Status:       db.StatusUp,
		ResponseTime: elapsed,
		Message:      fmt.Sprintf("TCP port %d open on %s", m.Port, m.Hostname),
	}
}
