package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"traffic-go/internal/domain"
)

var severityMap = map[string]string{
	"极高": "high",
	"高":  "high",
	"中":  "medium",
	"低":  "low",
}

func Fingerprint(event map[string]any) string {
	raw := fmt.Sprintf("%v|%v|%v|%v",
		event["threat_source"],
		event["victim_target"],
		event["rule_desc"],
		event["occurrence_time"],
	)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func MakeIdempotencyKey(ev map[string]any) string {
	if id := asString(ev["event_id"]); id != "" {
		sum := sha256.Sum256([]byte(id))
		return hex.EncodeToString(sum[:])[:32]
	}
	raw := fmt.Sprintf("%v|%v|%v|%v|%v",
		ev["probe_id"], ev["analyser_id"], ev["threat_type"], ev["flow_id"], ev["time"],
	)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

func LyEventToDeepSOC(ly map[string]any) domain.Event {
	src := asString(ly["threat_source"])
	dst := asString(ly["victim_target"])
	context := map[string]any{
		"ly_id":             ly["id"],
		"event_type":        ly["event_type"],
		"detail_type":       ly["detail_type"],
		"detection_method":  ly["method"],
		"occurrence_time":   ly["occurrence_time"],
		"duration":          ly["duration"],
		"is_active":         ly["is_active"],
		"processing_status": ly["processing_status"],
		"system_ref":        "FlowShadow",
	}
	ctx, _ := json.Marshal(context)
	level := severityMap[asString(ly["event_level"])]
	if level == "" {
		level = "medium"
	}
	ruleDesc := asString(ly["rule_desc"])
	eventType := asString(ly["event_type"])
	title := fmt.Sprintf("%s (%s)", ruleDesc, eventType)
	message := fmt.Sprintf("SIEM告警：检测到 %s 对 %s 发起 %s 攻击，检测方式：%s",
		src, dst, ruleDesc, asString(ly["method"]))
	return domain.Event{
		EventID:     asString(ly["event_id"]),
		EventName:   title,
		Title:       title,
		Message:     message,
		Severity:    level,
		Source:      "FlowShadow",
		Category:    "Network Threat",
		Context:     string(ctx),
		EventStatus: "pending",
		Observables: []domain.IOC{
			{Type: "ip", Value: src, Role: "source"},
			{Type: "ip", Value: dst, Role: "destination"},
		},
	}
}
