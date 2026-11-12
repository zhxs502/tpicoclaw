package common

import (
	"encoding/json"
	"testing"
)

func TestSerializeMessagesFileIDAndImageURL(t *testing.T) {
	messages := []Message{
		{
			Role:    "user",
			Content: "look at this",
			Media:   []string{"file_id:file-abc123", "data:image/jpeg;base64,/9j/2Q=="},
		},
		{
			Role:    "user",
			Content: "plain text only",
		},
	}
	out := SerializeMessages(messages)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}

	var decoded []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(decoded))
	}

	var parts []map[string]any
	if err := json.Unmarshal(decoded[0].Content, &parts); err != nil {
		t.Fatalf("content not array: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %s", len(parts), decoded[0].Content)
	}
	if parts[0]["type"] != "text" || parts[0]["text"] != "look at this" {
		t.Fatalf("text part mismatch: %v", parts[0])
	}
	if parts[1]["type"] != "file" || parts[1]["file_id"] != "file-abc123" {
		t.Fatalf("file part mismatch: %v", parts[1])
	}
	if parts[2]["type"] != "image_url" {
		t.Fatalf("image_url part missing: %v", parts[2])
	}
	img, _ := parts[2]["image_url"].(map[string]any)
	if img == nil || img["url"] != "data:image/jpeg;base64,/9j/2Q==" {
		t.Fatalf("image_url payload mismatch: %v", parts[2])
	}

	// Text-only message must stay a plain string.
	var plain string
	if err := json.Unmarshal(decoded[1].Content, &plain); err != nil || plain != "plain text only" {
		t.Fatalf("plain message should remain string, got %s (err %v)", decoded[1].Content, err)
	}
}
