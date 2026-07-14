package mailadapter

import (
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guard"
)

func redactText(text string) (string, bool) {
	report := guard.Detect("mail", text)
	if strings.TrimSpace(report.Redacted) == "" {
		return "", false
	}
	return report.Redacted, len(report.Findings) > 0
}

func trimText(text string, max int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func summarizeBody(body string, max int) (string, bool) {
	redacted, changed := redactText(body)
	return trimText(redacted, max), changed
}

func messageDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
