package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"traffic-go/internal/autopilot"
)

type drivingModeResponseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *drivingModeResponseRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *drivingModeResponseRecorder) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (w *drivingModeResponseRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *drivingModeResponseRecorder) flush() {
	w.ResponseWriter.WriteHeader(w.statusCode())
	_, _ = w.ResponseWriter.Write(w.body.Bytes())
}

func (s *Server) drivingModeGetCompat(w http.ResponseWriter, r *http.Request) {
	enabled, err := autopilot.GetDrivingMode(r.Context(), s.cfg.DatabaseURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"enabled": enabled,
		},
	})
}

func (s *Server) drivingModePut(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if len(bytes.TrimSpace(body)) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	if err := autopilot.SetDrivingMode(r.Context(), s.cfg.DatabaseURL, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"enabled": req.Enabled,
		},
	})
}

func (s *Server) withDrivingModeAutomation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &drivingModeResponseRecorder{ResponseWriter: w}
		next(rec, r)

		if code := rec.statusCode(); code >= 200 && code < 300 {
			eventID, title := drivingEventFromResponse(rec.body.Bytes())
			if eventID == "" {
				eventID, title = drivingEventFromRequest(r)
			}
			if eventID != "" {
				if err := autopilot.RunForEvent(r.Context(), s.cfg.DatabaseURL, eventID, title); err != nil {
					log.Printf("driving-mode: automation failed event_id=%s err=%v", eventID, err)
				}
			}
		}

		rec.flush()
	}
}

func drivingEventFromResponse(raw []byte) (string, string) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", ""
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", ""
	}
	if data, ok := env["data"].(map[string]any); ok {
		return drivingEventFromMap(data)
	}
	return drivingEventFromMap(env)
}

func drivingEventFromRequest(r *http.Request) (string, string) {
	if r == nil || r.Body == nil {
		return "", ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return "", ""
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}
	return drivingEventFromMap(req)
}

func drivingEventFromMap(m map[string]any) (string, string) {
	if m == nil {
		return "", ""
	}
	eventID := firstString(m, "event_id", "eventId", "eventID", "id")
	title := firstString(m, "title", "event_name", "eventName", "message")
	return eventID, title
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
