package monitor

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/db"
)

// DNSChecker performs DNS resolution checks.
type DNSChecker struct{}

// Check resolves the hostname using the configured DNS record type.
func (c *DNSChecker) Check(ctx context.Context, m *db.Monitor) CheckResult {
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	resolver := &net.Resolver{
		PreferGo: true,
	}

	dnsType := strings.ToUpper(m.DNSType)
	if dnsType == "" {
		dnsType = "A"
	}

	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var records []string
	var err error

	switch dnsType {
	case "A", "AAAA":
		var ips []net.IP
		ips, err = resolver.LookupIP(resolveCtx, networkForType(dnsType), m.Hostname)
		for _, ip := range ips {
			records = append(records, ip.String())
		}
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(resolveCtx, m.Hostname)
		if cname != "" {
			records = append(records, cname)
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(resolveCtx, m.Hostname)
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
		}
	case "TXT":
		records, err = resolver.LookupTXT(resolveCtx, m.Hostname)
	case "NS":
		var nss []*net.NS
		nss, err = resolver.LookupNS(resolveCtx, m.Hostname)
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	default:
		// Fallback to A record lookup
		var ips []net.IP
		ips, err = resolver.LookupIP(resolveCtx, "ip4", m.Hostname)
		for _, ip := range ips {
			records = append(records, ip.String())
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("DNS %s lookup for %s failed: %s", dnsType, m.Hostname, err),
		}
	}

	if len(records) == 0 {
		return CheckResult{
			Status:       db.StatusDown,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("DNS %s lookup for %s returned no records", dnsType, m.Hostname),
		}
	}

	// Truncate display if many records
	display := strings.Join(records, ", ")
	if len(display) > 200 {
		display = display[:200] + "..."
	}

	return CheckResult{
		Status:       db.StatusUp,
		ResponseTime: elapsed,
		Message:      fmt.Sprintf("DNS %s: %s → %s", dnsType, m.Hostname, display),
	}
}

// networkForType maps DNS record type to Go net.Resolver network parameter.
func networkForType(dnsType string) string {
	switch dnsType {
	case "AAAA":
		return "ip6"
	default:
		return "ip4"
	}
}
