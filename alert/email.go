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

	"github.com/vyanawatch/vyanawatch/config"
	"github.com/vyanawatch/vyanawatch/monitor"
)

// EmailNotifier sends alert emails via SMTP.
type EmailNotifier struct {
	cfg config.EmailConfig
}

func (e *EmailNotifier) Name() string { return "email" }

func (e *EmailNotifier) Send(event monitor.Event) error {
	subject := FormatSubject(event)
	htmlBody, err := renderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("render email template: %w", err)
	}

	recipients := strings.Split(e.cfg.To, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	msg := buildMIMEMessage(e.cfg.From, recipients, subject, htmlBody)

	addr := net.JoinHostPort(e.cfg.Host, strconv.Itoa(e.cfg.Port))

	var auth smtp.Auth
	if e.cfg.Username != "" {
		auth = smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.Host)
	}

	// Support STARTTLS on port 587, implicit TLS on 465, plain on 25
	if e.cfg.Port == 465 {
		return sendImplicitTLS(addr, auth, e.cfg.From, recipients, msg)
	}
	return smtp.SendMail(addr, auth, e.cfg.From, recipients, msg)
}

func buildMIMEMessage(from string, to []string, subject, htmlBody string) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)
	return buf.Bytes()
}

func sendImplicitTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

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
		return err
	}
	for _, r := range to {
		if err := client.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
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

	tmpl, err := template.New("email").Parse(emailTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const emailTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f4f4;font-family:Arial,Helvetica,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.08);">
  <!-- Header -->
  <tr>
    <td style="background:{{.StatusColor}};padding:24px 30px;">
      <h1 style="margin:0;color:#fff;font-size:22px;">{{.StatusEmoji}} Monitor {{.StatusLabel}}</h1>
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
