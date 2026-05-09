package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"traffic-go/internal/autopilot"
)

const drivingModeStateKey = "driving_mode"

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
	enabled, err := s.getDrivingMode(r)
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
	if err := s.setDrivingMode(r, req.Enabled); err != nil {
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
		requestBody, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(requestBody))

		rec := &drivingModeResponseRecorder{ResponseWriter: w}
		next(rec, r)

		if code := rec.statusCode(); code >= 200 && code < 300 {
			eventID, _ := drivingEventFromResponse(rec.body.Bytes())
			if eventID == "" {
				eventID, _ = drivingEventFromBytes(requestBody)
			}
			if eventID != "" {
				forceAutomation := shouldAutoDriveCreatedEvent(r, requestBody)
				enabled, err := s.getDrivingMode(r)
				if err != nil {
					log.Printf("driving-mode: read state failed event_id=%s err=%v", eventID, err)
				} else if enabled || forceAutomation {
					if forceAutomation && !enabled {
						if err := s.setDrivingMode(r, true); err != nil {
							log.Printf("driving-mode: enable default automation failed event_id=%s err=%v", eventID, err)
						}
					}
					err = s.services.RunAgentWorkflow(r.Context(), eventID)
				}
				if err != nil {
					log.Printf("driving-mode: automation failed event_id=%s err=%v", eventID, err)
				}
			} else if shouldAutoDriveCreatedEvent(r, requestBody) {
				log.Printf("driving-mode: automation skipped because event id was not found path=%s", r.URL.Path)
			}
		}

		rec.flush()
	}
}

func (s *Server) getDrivingMode(r *http.Request) (bool, error) {
	if strings.TrimSpace(s.cfg.DatabaseURL) != "" {
		enabled, err := autopilot.GetDrivingMode(r.Context(), s.cfg.DatabaseURL)
		if err == nil {
			s.setMemoryState(drivingModeStateKey, enabled)
			return enabled, nil
		}
		if s.cfg.StoreBackend == "postgres" {
			return false, err
		}
	}
	return s.getMemoryState(drivingModeStateKey), nil
}

func (s *Server) setDrivingMode(r *http.Request, enabled bool) error {
	s.setMemoryState(drivingModeStateKey, enabled)
	if strings.TrimSpace(s.cfg.DatabaseURL) == "" {
		return nil
	}
	if err := autopilot.SetDrivingMode(r.Context(), s.cfg.DatabaseURL, enabled); err != nil && s.cfg.StoreBackend == "postgres" {
		return err
	}
	return nil
}

func (s *Server) getMemoryState(key string) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.states[key]
}

func (s *Server) setMemoryState(key string, value bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.states[key] = value
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

func drivingEventFromBytes(body []byte) (string, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", ""
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}
	return drivingEventFromMap(req)
}

func shouldAutoDriveCreatedEvent(r *http.Request, body []byte) bool {
	if r == nil {
		return false
	}
	var req map[string]any
	if len(bytes.TrimSpace(body)) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	if v, ok := boolRequestValue(req, "auto_driving", "autoDriving", "driving_mode", "drivingMode", "autopilot", "ai_analysis", "aiAnalysis"); ok {
		return v
	}
	if r.URL == nil {
		return false
	}
	switch r.URL.Path {
	case "/api/event/create", "/internal/event/push":
		return true
	default:
		return false
	}
}

func boolRequestValue(m map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case bool:
			return x, true
		case string:
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "1", "true", "yes", "y", "on":
				return true, true
			case "0", "false", "no", "n", "off":
				return false, true
			}
		}
	}
	return false, false
}

func drivingEventFromMap(m map[string]any) (string, string) {
	if m == nil {
		return "", ""
	}
	eventID := firstString(m, "event_id", "eventId", "eventID", "deepsoc_event_id", "deepSOCEventID", "deepSocEventID", "id")
	if eventID == "" {
		if upstream, ok := m["upstream"].(map[string]any); ok {
			eventID, _ = drivingEventFromMap(upstream)
		}
	}
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
