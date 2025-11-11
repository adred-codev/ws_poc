package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Alerter interface for sending notifications to external services
// Implementations: Slack, Email, PagerDuty, etc.
type Alerter interface {
	Alert(level AuditLevel, message string, metadata map[string]any)
}

// MultiAlerter sends alerts to multiple alerters
// Example: Send to both Slack and Email
type MultiAlerter struct {
	alerters []Alerter
}

func NewMultiAlerter(alerters ...Alerter) *MultiAlerter {
	return &MultiAlerter{alerters: alerters}
}

func (m *MultiAlerter) Alert(level AuditLevel, message string, metadata map[string]any) {
	for _, alerter := range m.alerters {
		// Run in goroutine to avoid blocking
		go alerter.Alert(level, message, metadata)
	}
}

// SlackAlerter sends alerts to Slack via webhook
type SlackAlerter struct {
	webhookURL string
	channel    string
	username   string
}

func NewSlackAlerter(webhookURL, channel, username string) *SlackAlerter {
	return &SlackAlerter{
		webhookURL: webhookURL,
		channel:    channel,
		username:   username,
	}
}

func (s *SlackAlerter) Alert(level AuditLevel, message string, metadata map[string]any) {
	if s.webhookURL == "" {
		return // Not configured
	}

	color := s.getColor(level)
	emoji := s.getEmoji(level)

	// Build fields from metadata
	fields := []map[string]any{}
	for k, v := range metadata {
		fields = append(fields, map[string]any{
			"title": k,
			"value": fmt.Sprintf("%v", v),
			"short": true,
		})
	}

	payload := map[string]any{
		"username": s.username,
		"channel":  s.channel,
		"text":     fmt.Sprintf("%s *%s Alert*", emoji, level),
		"attachments": []map[string]any{
			{
				"color":     color,
				"title":     message,
				"fields":    fields,
				"timestamp": time.Now().Unix(),
				"footer":    "WebSocket Server",
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return
	}

	// Send to Slack (with timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	_, _ = client.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	// Ignore errors - don't want alerting to break the server
}

func (s *SlackAlerter) getColor(level AuditLevel) string {
	switch level {
	case CRITICAL:
		return "danger"
	case ERROR:
		return "danger"
	case WARNING:
		return "warning"
	default:
		return "good"
	}
}

func (s *SlackAlerter) getEmoji(level AuditLevel) string {
	switch level {
	case CRITICAL:
		return ":rotating_light:"
	case ERROR:
		return ":x:"
	case WARNING:
		return ":warning:"
	case INFO:
		return ":information_source:"
	default:
		return ":white_check_mark:"
	}
}

// ConsoleAlerter prints alerts to console (for development/testing)
type ConsoleAlerter struct{}

func NewConsoleAlerter() *ConsoleAlerter {
	return &ConsoleAlerter{}
}

func (c *ConsoleAlerter) Alert(level AuditLevel, message string, metadata map[string]any) {
	fmt.Printf("\n🔔 ALERT [%s]: %s\n", level, message)
	if len(metadata) > 0 {
		fmt.Println("  Metadata:")
		for k, v := range metadata {
			fmt.Printf("    %s: %v\n", k, v)
		}
	}
	fmt.Println()
}
