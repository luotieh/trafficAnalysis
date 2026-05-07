package service

import (
	"encoding/json"
	"time"

	"traffic-go/internal/domain"
)

func NormalizeMessageFrom(v string) string {
	return domain.NormalizeMessageFrom(v)
}

func SenderType(messageFrom string) string {
	return domain.SenderType(messageFrom)
}

func StandardContent(data any) string {
	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"data":      data,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func responseText(text string, extra map[string]any) map[string]any {
	out := map[string]any{"response_text": text}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
