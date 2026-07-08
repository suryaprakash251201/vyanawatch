package engine

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/internal/model"
)

type HTTPChecker struct {
	proxyURL string
}

func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{}
}

func (c *HTTPChecker) WithProxy(proxyURL string) *HTTPChecker {
	c.proxyURL = proxyURL
	return c
}

func (c *HTTPChecker) Check(ctx context.Context, m *model.Monitor) CheckResult {
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	if c.proxyURL != "" {
		if proxyURI, err := url.Parse(c.proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURI)
		}
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

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
			Status:  model.StatusDown,
			Message: fmt.Sprintf("Failed to create request: %s", err),
		}
	}

	if m.Headers != "" {
		headers := make(map[string]string)
		if err := json.Unmarshal([]byte(m.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "VyanaWatch/1.0")
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:       model.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("Request failed: %s", err),
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	expectedCode := m.ExpectedStatusCode
	if expectedCode == 0 {
		expectedCode = 200
	}
	if resp.StatusCode != expectedCode {
		return CheckResult{
			Status:       model.StatusDown,
			ResponseTime: elapsed,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("Expected status %d, got %d", expectedCode, resp.StatusCode),
		}
	}

	if m.KeywordCheck != "" {
		bodyStr := string(body)
		contains := strings.Contains(bodyStr, m.KeywordCheck)
		if m.KeywordPresent && !contains {
			return CheckResult{
				Status:       model.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("Keyword '%s' not found in response", m.KeywordCheck),
			}
		}
		if !m.KeywordPresent && contains {
			return CheckResult{
				Status:       model.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("Keyword '%s' found in response (should not be present)", m.KeywordCheck),
			}
		}
	}

	if m.SSLCheck && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		expiry := cert.NotAfter
		daysUntilExpiry := int(time.Until(expiry).Hours() / 24)
		threshold := m.SSLExpiryDays
		if threshold <= 0 {
			threshold = 30
		}
		if daysUntilExpiry <= 0 {
			return CheckResult{
				Status:       model.StatusDown,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("SSL certificate EXPIRED (expired %d days ago)", -daysUntilExpiry),
				CertExpiry:   &expiry,
			}
		}
		if daysUntilExpiry <= threshold {
			return CheckResult{
				Status:       model.StatusUp,
				ResponseTime: elapsed,
				StatusCode:   resp.StatusCode,
				Message:      fmt.Sprintf("SSL certificate expires in %d days", daysUntilExpiry),
				CertExpiry:   &expiry,
			}
		}
		return CheckResult{
			Status:       model.StatusUp,
			ResponseTime: elapsed,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("HTTP %d OK", resp.StatusCode),
			CertExpiry:   &expiry,
		}
	}

	return CheckResult{
		Status:       model.StatusUp,
		ResponseTime: elapsed,
		StatusCode:   resp.StatusCode,
		Message:      fmt.Sprintf("HTTP %d OK", resp.StatusCode),
	}
}
