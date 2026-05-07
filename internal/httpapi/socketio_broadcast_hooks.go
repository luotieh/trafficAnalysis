package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"traffic-go/internal/realtime"
)

type socketBroadcastRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *socketBroadcastRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *socketBroadcastRecorder) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (w *socketBroadcastRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *socketBroadcastRecorder) flush() {
	w.ResponseWriter.WriteHeader(w.statusCode())
	_, _ = w.ResponseWriter.Write(w.body.Bytes())
}

func (s *Server) withNewMessageBroadcast(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID := r.PathValue("event_id")
		if eventID == "" {
			eventID = eventIDFromRequestBody(r)
		}

		rec := &socketBroadcastRecorder{ResponseWriter: w}
		next(rec, r)

		if code := rec.statusCode(); code >= 200 && code < 300 {
			payload, resolvedEventID := messagePayloadFromResponse(rec.body.Bytes())
			if eventID == "" {
				eventID = resolvedEventID
			}
			if eventID != "" && payload != nil {
				realtime.BroadcastMessage(eventID, payload)
			}
		}

		rec.flush()
	}
}

func (s *Server) withExecutionUpdateBroadcast(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID := r.PathValue("event_id")
		rec := &socketBroadcastRecorder{ResponseWriter: w}
		next(rec, r)

		if code := rec.statusCode(); code >= 200 && code < 300 && eventID != "" {
			payload, _ := messagePayloadFromResponse(rec.body.Bytes())
			if payload == nil {
				payload = map[string]any{"event_id": eventID, "status": "updated"}
			}
			realtime.BroadcastExecutionUpdate(eventID, payload)
		}

		rec.flush()
	}
}

func eventIDFromRequestBody(r *http.Request) string {
	if r == nil || r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"event_id", "eventId", "eventID"} {
		if v, ok := obj[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func messagePayloadFromResponse(raw []byte) (any, string) {
	if len(raw) == 0 {
		return nil, ""
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, ""
	}
	payload := any(envelope)
	if data, ok := envelope["data"]; ok && data != nil {
		payload = data
	}
	return payload, eventIDFromAny(payload)
}

func eventIDFromAny(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"event_id", "eventId", "eventID"} {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	if inner, ok := m["message"].(map[string]any); ok {
		return eventIDFromAny(inner)
	}
	return ""
}
