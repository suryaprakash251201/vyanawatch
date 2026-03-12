package alert

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/monitor"
)

// EmailNotifier sends alert emails via SMTP.
type EmailNotifier struct {
	cfg config.EmailConfig
}

// NewEmailNotifier creates an EmailNotifier, optionally with an initial config snapshot.
// The Send method always re-reads the live config from config.Get().
func NewEmailNotifier(cfg config.EmailConfig) *EmailNotifier {
	return &EmailNotifier{cfg: cfg}
}

func (e *EmailNotifier) Name() string { return "email" }

// RenderEmailHTML renders the HTML alert email for a monitor event.
func RenderEmailHTML(event monitor.Event) (string, error) {
	return renderEmailHTML(event)
}

func (e *EmailNotifier) Send(event monitor.Event) error {
	cfg := e.cfg
	if c := config.Get(); c != nil {
		cfg = c.Alerting.Email
	}

	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return fmt.Errorf("smtp host is empty")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("email from address is empty")
	}

	port := cfg.Port
	if port <= 0 {
		port = 587
	}

	subject := FormatSubject(event)
	htmlBody, err := renderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("render email template: %w", err)
	}
	plainBody := FormatPlainText(event)

	parts := strings.Split(cfg.To, ",")
	recipients := make([]string, 0, len(parts))
	for _, r := range parts {
		r = strings.TrimSpace(r)
		if r != "" {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no recipient configured")
	}

	msg := buildMIMEMessage(cfg.From, recipients, subject, plainBody, htmlBody)

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}

	// Implicit TLS on port 465
	if port == 465 {
		return sendImplicitTLS(addr, auth, cfg.From, recipients, msg)
	}

	// STARTTLS on port 587 (or plain for other ports)
	return sendWithSTARTTLS(addr, host, auth, cfg.From, recipients, msg)
}

func buildMIMEMessage(from string, to []string, subject, plainBody, htmlBody string) []byte {
	boundary := fmt.Sprintf("vw-%d", time.Now().UnixNano())
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(plainBody)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)
	buf.WriteString("\r\n")
	buf.WriteString("--" + boundary + "--\r\n")

	return buf.Bytes()
}

// smtpDialTimeout is the timeout for establishing SMTP connections.
const smtpDialTimeout = 30 * time.Second

func sendImplicitTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial %s: %w", addr, err)
	}
	defer conn.Close()

	return smtpSendOnConn(conn, host, auth, from, to, msg)
}

func sendWithSTARTTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Quit()

	// Upgrade to TLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, r := range to {
		if err := client.Rcpt(r); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", r, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func smtpSendOnConn(conn net.Conn, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Quit()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, r := range to {
		if err := client.Rcpt(r); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", r, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

// emailData holds template variables for the HTML email.
type emailData struct {
	IsDown       bool
	MonitorName  string
	MonitorType  string
	URL          string
	Hostname     string
	Reason       string
	ResponseTime int64
	Timestamp    string
	Downtime     string
	StatusColor  string
	StatusLabel  string
	StatusEmoji  string
}

func renderEmailHTML(event monitor.Event) (string, error) {
	m := event.Monitor
	d := emailData{
		IsDown:       event.Type == monitor.EventDown,
		MonitorName:  m.Name,
		MonitorType:  string(m.Type),
		URL:          m.URL,
		Hostname:     m.Hostname,
		ResponseTime: event.Result.ResponseTime,
		Timestamp:    event.Timestamp.Format("2006-01-02 15:04:05 UTC"),
		Reason:       event.Result.Message,
	}

	if d.IsDown {
		d.StatusColor = "#e74c3c"
		d.StatusLabel = "DOWN"
		d.StatusEmoji = "🔴"
	} else {
		d.StatusColor = "#2ecc71"
		d.StatusLabel = "RECOVERED"
		d.StatusEmoji = "🟢"
		if event.Incident != nil && event.Incident.Duration > 0 {
			d.Downtime = formatDuration(event.Incident.Duration)
		}
	}

	tmplSrc := recoveryEmailTemplate
	if d.IsDown {
		tmplSrc = downEmailTemplate
	}

	tmpl, err := template.New("email").Parse(tmplSrc)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const downEmailTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f4f4;font-family:Arial,Helvetica,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.08);">
  <!-- Header -->
  <tr>
    <td style="background:{{.StatusColor}};padding:24px 30px;">
			<h1 style="margin:0;color:#fff;font-size:22px;">{{.StatusEmoji}} Site Down Alert</h1>
			<p style="margin:8px 0 0;color:#ffeaea;font-size:13px;">VyanaWatch detected a failure and triggered this notification.</p>
    </td>
  </tr>
  <!-- Body -->
  <tr>
    <td style="padding:30px;">
      <h2 style="margin:0 0 16px;color:#333;">{{.MonitorName}}</h2>
      <table width="100%" cellpadding="8" cellspacing="0" style="border-collapse:collapse;">
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;width:140px;"><strong>Type</strong></td>
          <td style="color:#333;">{{.MonitorType}}</td>
        </tr>
        {{if .URL}}
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;"><strong>URL</strong></td>
          <td style="color:#333;">{{.URL}}</td>
        </tr>
        {{end}}
        {{if .Hostname}}
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;"><strong>Host</strong></td>
          <td style="color:#333;">{{.Hostname}}</td>
        </tr>
        {{end}}
        {{if .IsDown}}
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;"><strong>Reason</strong></td>
          <td style="color:#e74c3c;">{{.Reason}}</td>
        </tr>
        {{end}}
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;"><strong>Response Time</strong></td>
          <td style="color:#333;">{{.ResponseTime}} ms</td>
        </tr>
        {{if .Downtime}}
        <tr style="border-bottom:1px solid #eee;">
          <td style="color:#666;"><strong>Downtime</strong></td>
          <td style="color:#333;">{{.Downtime}}</td>
        </tr>
        {{end}}
        <tr>
          <td style="color:#666;"><strong>Time</strong></td>
          <td style="color:#333;">{{.Timestamp}}</td>
        </tr>
      </table>
    </td>
  </tr>
  <!-- Footer -->
  <tr>
    <td style="padding:16px 30px;background:#f9f9f9;text-align:center;">
      <p style="margin:0;color:#999;font-size:12px;">Sent by VyanaWatch &mdash; Lightweight Self-Hosted Uptime Monitor</p>
    </td>
  </tr>
</table>
</td></tr>
</table>
</body>
</html>`

const recoveryEmailTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f4f4;font-family:Arial,Helvetica,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.08);">
	<tr>
		<td style="background:{{.StatusColor}};padding:24px 30px;">
			<h1 style="margin:0;color:#fff;font-size:22px;">{{.StatusEmoji}} Site Recovered</h1>
			<p style="margin:8px 0 0;color:#e8fff0;font-size:13px;">The monitored endpoint is healthy again.</p>
		</td>
	</tr>
	<tr>
		<td style="padding:30px;">
			<h2 style="margin:0 0 16px;color:#333;">{{.MonitorName}}</h2>
			<table width="100%" cellpadding="8" cellspacing="0" style="border-collapse:collapse;">
				<tr style="border-bottom:1px solid #eee;">
					<td style="color:#666;width:140px;"><strong>Type</strong></td>
					<td style="color:#333;">{{.MonitorType}}</td>
				</tr>
				{{if .URL}}
				<tr style="border-bottom:1px solid #eee;">
					<td style="color:#666;"><strong>URL</strong></td>
					<td style="color:#333;">{{.URL}}</td>
				</tr>
				{{end}}
				{{if .Hostname}}
				<tr style="border-bottom:1px solid #eee;">
					<td style="color:#666;"><strong>Host</strong></td>
					<td style="color:#333;">{{.Hostname}}</td>
				</tr>
				{{end}}
				<tr style="border-bottom:1px solid #eee;">
					<td style="color:#666;"><strong>Response Time</strong></td>
					<td style="color:#333;">{{.ResponseTime}} ms</td>
				</tr>
				{{if .Downtime}}
				<tr style="border-bottom:1px solid #eee;">
					<td style="color:#666;"><strong>Downtime</strong></td>
					<td style="color:#333;">{{.Downtime}}</td>
				</tr>
				{{end}}
				<tr>
					<td style="color:#666;"><strong>Time</strong></td>
					<td style="color:#333;">{{.Timestamp}}</td>
				</tr>
			</table>
		</td>
	</tr>
	<tr>
		<td style="padding:16px 30px;background:#f9f9f9;text-align:center;">
			<p style="margin:0;color:#999;font-size:12px;">Sent by VyanaWatch &mdash; Lightweight Self-Hosted Uptime Monitor</p>
		</td>
	</tr>
</table>
</td></tr>
</table>
</body>
</html>`
