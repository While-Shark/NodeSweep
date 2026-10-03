package alerts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPlatformFormatsBoundMetadataAndDisableMentions(t *testing.T) {
	event := Event{Node: "node", Name: "@everyone <@123> <!channel>\n" + strings.Repeat("猫", 4000), Kind: "cleanup_failure", Task: "task", At: time.Now()}
	for _, format := range []string{"slack", "discord"} {
		raw, err := formatEvent(format, event)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 3000 || strings.Contains(string(raw), "@everyone") {
			t.Fatal("unbounded or mentions", len(raw))
		}
		var data map[string]any
		if err = json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if format == "slack" {
			if data["mrkdwn"] != false || data["unfurl_links"] != false || data["unfurl_media"] != false {
				t.Fatal(data)
			}
			blocks := data["blocks"].([]any)
			text := blocks[0].(map[string]any)["text"].(map[string]any)
			if text["type"] != "plain_text" {
				t.Fatal(data)
			}
		} else {
			mentions := data["allowed_mentions"].(map[string]any)
			if len(mentions["parse"].([]any)) != 0 || data["flags"] != float64(4) {
				t.Fatal(data)
			}
		}
	}
	if _, err := formatEvent("unknown", event); err == nil {
		t.Fatal("unknown format")
	}
	raw, _ := formatEvent("event", event)
	var decoded Event
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Name != event.Name {
		t.Fatal("legacy JSON compatibility", err)
	}
}
