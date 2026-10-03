package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

func ValidateFormat(format string) error {
	switch format {
	case "", "event", "slack", "discord":
		return nil
	}
	return errors.New("webhookFormat must be event, slack or discord")
}
func plainField(value string) string {
	var out strings.Builder
	count := 0
	for _, r := range value {
		if count >= 160 {
			out.WriteString("…")
			break
		}
		count++
		if unicode.IsControl(r) {
			r = ' '
		}
		// Prevent links, markup and mention syntax in administrator/node metadata.
		if strings.ContainsRune("<>@&`*_~[]", r) {
			out.WriteRune(' ')
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
func formatEvent(format string, event Event) ([]byte, error) {
	if err := ValidateFormat(format); err != nil {
		return nil, err
	}
	if format == "" || format == "event" {
		return json.Marshal(event)
	}
	status := "alert"
	if event.Resolved {
		status = "recovered"
	}
	text := fmt.Sprintf("NodeSweep %s: %s\nNode: %s (%s)\nTime: %s", status, plainField(event.Kind), plainField(event.Name), plainField(event.Node), event.At.UTC().Format(time.RFC3339))
	if event.Path != "" {
		text += "\nMount: " + plainField(event.Path)
	}
	if event.Kind == "disk" || event.Kind == "inode" {
		text += fmt.Sprintf("\nUsage: %.1f%%", event.Percent)
	}
	if event.Task != "" {
		text += "\nTask: " + plainField(event.Task)
	}
	if format == "slack" {
		return json.Marshal(map[string]any{"text": text, "mrkdwn": false, "unfurl_links": false, "unfurl_media": false, "blocks": []any{map[string]any{"type": "section", "text": map[string]any{"type": "plain_text", "text": text, "emoji": false}}}})
	}
	return json.Marshal(map[string]any{"content": text, "allowed_mentions": map[string]any{"parse": []string{}, "replied_user": false}, "flags": 4})
}
