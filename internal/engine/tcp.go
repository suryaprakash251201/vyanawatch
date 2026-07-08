package engine

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
)

type TCPChecker struct{}

func (c *TCPChecker) Check(ctx context.Context, m *model.Monitor) CheckResult {
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
			Status:       model.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("TCP connection to %s failed: %s", addr, err),
		}
	}
	conn.Close()

	return CheckResult{
		Status:       model.StatusUp,
		ResponseTime: elapsed,
		Message:      fmt.Sprintf("TCP port %d open on %s", m.Port, m.Hostname),
	}
}
