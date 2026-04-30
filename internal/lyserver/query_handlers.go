package lyserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Service) Event(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	q := r.URL.Query()
	reqType := strings.ToLower(firstNonEmpty(q.Get("req_type"), q.Get("type"), "list"))
	limit := parseLimit(q.Get("limit"), 100)
	if limit > 500 {
		limit = 500
	}

	switch reqType {
	case "aggre", "aggregate", "aggregation":
		items, err := s.queryRows(r.Context(), `
SELECT id, aggre_key, event_type, event_count, threat_source, victim_target, first_time, last_time, severity, raw
FROM t_event_data_aggre
ORDER BY last_time DESC, id DESC
LIMIT $1`, []string{"id", "aggre_key", "event_type", "event_count", "threat_source", "victim_target", "first_time", "last_time", "severity", "raw"}, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "req_type": reqType, "data": items, "total": len(items)})
		return
	case "detail":
		id := firstNonEmpty(q.Get("event_id"), q.Get("id"))
		if id == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{}})
			return
		}
		items, err := s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
WHERE event_id=$1 OR id::text=$1
ORDER BY occurrence_time DESC
LIMIT 1`, eventColumns(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		data := map[string]any{}
		if len(items) > 0 {
			data = items[0]
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "req_type": reqType, "data": data})
		return
	default:
		items, err := s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
ORDER BY occurrence_time DESC, id DESC
LIMIT $1`, eventColumns(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "req_type": reqType, "data": items, "events": items, "total": len(items)})
		return
	}
}

func (s *Service) Feature(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id := firstNonEmpty(r.URL.Query().Get("id"), r.URL.Query().Get("flow_id"), r.URL.Query().Get("event_id"))
	item := s.findEventLike(r, id)
	feature := map[string]any{
		"id":       id,
		"flow_id":  id,
		"event":    item,
		"features": map[string]any{"source": "postgres_compat", "derived": true},
	}
	if src, ok := item["threat_source"]; ok {
		feature["src_ip"] = src
	}
	if dst, ok := item["victim_target"]; ok {
		feature["dst_ip"] = dst
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": feature})
}

func (s *Service) TopN(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	q := r.URL.Query()
	kind := strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("field"), q.Get("by"), "threat_source"))
	limit := parseLimit(q.Get("limit"), 10)
	if limit > 100 {
		limit = 100
	}

	field := "threat_source"
	switch kind {
	case "dst", "dest", "destination", "victim", "victim_target":
		field = "victim_target"
	case "event", "event_type", "type":
		field = "event_type"
	case "level", "event_level", "severity":
		field = "event_level"
	}
	items, err := s.queryRows(r.Context(), `
SELECT `+field+` AS name, COUNT(*)::bigint AS count
FROM t_event_data
WHERE `+field+` <> ''
GROUP BY `+field+`
ORDER BY count DESC, name ASC
LIMIT $1`, []string{"name", "count"}, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "type": kind, "field": field, "data": items, "total": len(items)})
}

func (s *Service) Evidence(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id := firstNonEmpty(r.URL.Query().Get("id"), r.URL.Query().Get("flow_id"), r.URL.Query().Get("event_id"))
	item := s.findEventLike(r, id)
	evidence := []map[string]any{}
	if len(item) > 0 {
		evidence = append(evidence, map[string]any{
			"id":          firstNonEmpty(asString(item["event_id"]), id),
			"event_id":    item["event_id"],
			"type":        "event_record",
			"source":      "t_event_data",
			"description": item["rule_desc"],
			"data":        item,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": evidence, "total": len(evidence)})
}

func (s *Service) InternalAsset(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	ip := pathTail(r.URL.Path, "/internal/assets/")
	items, err := s.queryRows(r.Context(), `
SELECT ip, asset_name, owner, business, meta, created_at, updated_at
FROM t_asset_ip
WHERE ip=$1
LIMIT 1`, []string{"ip", "asset_name", "owner", "business", "meta", "created_at", "updated_at"}, ip)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	moItems, _ := s.queryRows(r.Context(), `
SELECT id, moip, moport, protocol, modesc, tag, mogroupid, filter, devid, direction, meta, created_at, updated_at
FROM t_mo WHERE moip=$1 ORDER BY id LIMIT 20`, []string{"id", "moip", "moport", "protocol", "modesc", "tag", "mogroupid", "filter", "devid", "direction", "meta", "created_at", "updated_at"}, ip)
	services, _ := s.queryRows(r.Context(), `SELECT ip, port, protocol, service_name, meta, created_at, updated_at FROM t_asset_srv WHERE ip=$1 ORDER BY port LIMIT 100`, []string{"ip", "port", "protocol", "service_name", "meta", "created_at", "updated_at"}, ip)
	data := map[string]any{"ip": ip, "asset": map[string]any{}, "managed_objects": moItems, "services": services}
	if len(items) > 0 {
		data["asset"] = items[0]
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": data})
}

func (s *Service) InternalFlow(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/related") {
		s.InternalRelatedFlows(w, r)
		return
	}
	flowID := pathTail(r.URL.Path, "/internal/flows/")
	item := s.findEventLike(r, flowID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": item, "flow_id": flowID})
}

func (s *Service) InternalRelatedFlows(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	flowID := strings.TrimSuffix(pathTail(r.URL.Path, "/internal/flows/"), "/related")
	base := s.findEventLike(r, flowID)
	limit := parseLimit(r.URL.Query().Get("limit"), 10)
	if limit > 100 {
		limit = 100
	}
	var items []map[string]any
	var err error
	if src := asString(base["threat_source"]); src != "" {
		items, err = s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
WHERE threat_source=$1 OR victim_target=$1
ORDER BY occurrence_time DESC, id DESC
LIMIT $2`, eventColumns(), src, limit)
	} else {
		items, err = s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
ORDER BY occurrence_time DESC, id DESC
LIMIT $1`, eventColumns(), limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "flow_id": flowID, "data": items, "total": len(items)})
}

func (s *Service) InternalPCAP(w http.ResponseWriter, r *http.Request) {
	pcapID := pathTail(r.URL.Path, "/internal/pcaps/")
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"result": "ok",
		"data": map[string]any{
			"pcap_id":      pcapID,
			"status":       "not_available",
			"message":      "Go 兼容层已接管 PCAP 查询入口，真实 PCAP 文件生成将在后续证据链路实现。",
			"download_url": "",
		},
	})
}

func (s *Service) findEventLike(r *http.Request, id string) map[string]any {
	if id == "" {
		items, err := s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
ORDER BY occurrence_time DESC, id DESC
LIMIT 1`, eventColumns())
		if err == nil && len(items) > 0 {
			return items[0]
		}
		return map[string]any{}
	}
	items, err := s.queryRows(r.Context(), `
SELECT id, event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, occurrence_time, duration, processing_status, is_active, raw, created_at, updated_at
FROM t_event_data
WHERE event_id=$1 OR id::text=$1 OR raw->>'flow_id'=$1 OR raw->>'id'=$1
ORDER BY occurrence_time DESC, id DESC
LIMIT 1`, eventColumns(), id)
	if err == nil && len(items) > 0 {
		return items[0]
	}
	return map[string]any{}
}

func eventColumns() []string {
	return []string{"id", "event_id", "event_type", "detail_type", "event_level", "rule_desc", "threat_source", "victim_target", "method", "occurrence_time", "duration", "processing_status", "is_active", "raw", "created_at", "updated_at"}
}

func pathTail(path, prefix string) string {
	out := strings.TrimPrefix(path, prefix)
	out = strings.Trim(out, "/")
	if v, err := url.PathUnescape(out); err == nil {
		return v
	}
	return out
}

func parseLimit(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
