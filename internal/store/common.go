package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func stringPatch(p map[string]any, key string) (string, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	default:
		return fmt.Sprint(x), true
	}
}

func boolPatch(p map[string]any, key string) (bool, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func timePatch(p map[string]any, key string) (*time.Time, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return nil, false
	}
	switch x := v.(type) {
	case time.Time:
		return &x, true
	case string:
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return &t, true
		}
	}
	return nil, false
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return prefix + "-" + hex.EncodeToString(b)
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func toJSON(v any) []byte {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}
