package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/db"
)

// HTTPChecker performs HTTP/HTTPS endpoint checks.
type HTTPChecker struct{}

// Check executes an HTTP request and validates the response.
func (c *HTTPChecker) Check(ctx context.Context, m *db.Monitor) CheckResult {
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Build HTTP client with timeout — skip TLS verify only if not doing SSL checks
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	// Build request
	method := m.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if m.Body != "" {
		bodyReader = strings.NewReader(m.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, m.URL, bodyReader)
	if err != nil {
		return CheckResult{
			Status:  db.StatusDown,
			Message: fmt.Sprintf("Failed to create request: %s", err),
		}
	}

	// Set custom headers
	if m.Headers != "" {
		headers := make(map[string]string)
		if err := json.Unmarshal([]byte(m.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	// Set default User-Agent
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "VyanaWatch/1.0")
	}

	// Execute request
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("Request failed: %s", err),
		}
	}
	defer resp.Body.Close()

	// Read body for keyword check (limit to 1MB)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// Check status code
	expectedCode := m.ExpectedStatusCode
	if expectedCode == 0 {
		expectedCode = 200
	}
	if resp.StatusCode != expectedCode {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("Expected status %d, got %d", expectedCode, resp.StatusCode),
		}
	}

	// Check keyword
	if m.KeywordCheck != "" {
		bodyStr := string(body)
		contains := strings.Contains(bodyStr, m.KeywordCheck)
		if m.KeywordPresent && !contains {
			return CheckResult{
				Status:       db.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("Keyword '%s' not found in response", m.KeywordCheck),
			}
		}
		if !m.KeywordPresent && contains {
			return CheckResult{
				Status:       db.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("Keyword '%s' found in response (should not be present)", m.KeywordCheck),
			}
		}
	}

	// Check SSL certificate expiry
	if m.SSLCheck && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
		threshold := m.SSLExpiryDays
		if threshold <= 0 {
			threshold = 30
		}
		if daysUntilExpiry <= 0 {
			return CheckResult{
				Status:       db.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("SSL certificate EXPIRED (expired %d days ago)", -daysUntilExpiry),
			}
		}
		if daysUntilExpiry <= threshold {
			// Still UP but with a warning message
			return CheckResult{
				Status:       db.StatusUp,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("SSL certificate expires in %d days", daysUntilExpiry),
			}
		}
	}

	return CheckResult{
		Status:       db.StatusUp,
		ResponseTime: elapsed,
		StatusCode:   resp.StatusCode,
		Message:      fmt.Sprintf("HTTP %d OK", resp.StatusCode),
	}
}
