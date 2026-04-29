package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"traffic-go/internal/client"
	"traffic-go/internal/domain"
	"traffic-go/internal/mq"
	"traffic-go/internal/store"
)

type Services struct {
	Store      store.Store
	DeepSOC    client.DeepSOCClient
	FlowShadow client.FlowShadowClient
	LLM        client.LLMClient
	Queue      mq.Queue
}

func (s Services) ProcessLyEvent(ctx context.Context, ly map[string]any) (map[string]any, error) {
	fp := Fingerprint(ly)
	lyID := asString(ly["id"])
	if !s.Store.ReserveFingerprint(fp) {
		row, _ := s.Store.GetEventMap(fp)
		return map[string]any{
			"skipped":          true,
			"reason":           "duplicate",
			"fingerprint":      fp,
			"ly_id":            lyID,
			"deepsoc_event_id": row.DeepSOCEventID,
		}, nil
	}

	payload := LyEventToDeepSOC(ly)
	if s.DeepSOC.Enabled() {
		resp, err := s.DeepSOC.CreateEvent(ctx, payload, fp)
		if err != nil {
			return nil, err
		}
		deepID := extractEventID(resp)
		s.Store.BindEventMap(fp, lyID, deepID)
		_ = s.publish(ctx, "event.ingested", deepID, "flowshadow", map[string]any{
			"fingerprint": fp,
			"ly_event_id": lyID,
			"upstream":    resp,
		})
		return map[string]any{"success": true, "fingerprint": fp, "ly_event_id": lyID, "deepsoc_event_id": deepID, "upstream": resp}, nil
	}

	if payload.EventID == "" {
		payload.EventID = "ly-" + fp[:16]
	}
	event, err := s.Store.CreateEvent(payload)
	if err != nil {
		return nil, err
	}
	s.Store.BindEventMap(fp, lyID, event.EventID)
	systemMsg := map[string]any{"response_text": "系统创建了安全事件: " + firstNonEmpty(event.EventName, "未命名事件")}
	b, _ := json.Marshal(systemMsg)
	_, _ = s.Store.AddMessage(domain.Message{
		EventID:        event.EventID,
		MessageFrom:    "system",
		MessageType:    "system_notification",
		MessageContent: string(b),
		RoundID:        1,
	})
	_ = s.publish(ctx, "event.ingested", event.EventID, "flowshadow", map[string]any{
		"fingerprint": fp,
		"ly_event_id": lyID,
		"event":       event,
	})
	return map[string]any{"success": true, "fingerprint": fp, "ly_event_id": lyID, "deepsoc_event_id": event.EventID}, nil
}

func (s Services) RunSyncOnce(ctx context.Context, batchSize, lookbackSeconds, maxRetries int) (map[string]any, error) {
	cursor := s.Store.GetCursor("flowshadow_events")
	since := cursor.LastTS
	if since == "" {
		since = time.Now().UTC().Add(-time.Duration(lookbackSeconds) * time.Second).Format(time.RFC3339)
	}
	items, err := s.FlowShadow.ListEvents(ctx, since, batchSize)
	if err != nil {
		return nil, err
	}

	pushed, failed := 0, 0
	newestTS := since
	for _, ev := range items {
		evTS := asString(ev["time"])
		if evTS != "" && evTS > newestTS {
			newestTS = evTS
		}
		recordID := asString(ev["event_id"])
		idemKey := MakeIdempotencyKey(ev)
		if recordID == "" {
			recordID = "noid:" + idemKey
		}
		if s.Store.AlreadyPushed(recordID) {
			continue
		}
		pe := domain.PushedEvent{LyEventID: recordID, IdempotencyKey: idemKey, Status: "FAILED"}
		var lastErr error
		for i := 0; i < maxRetries; i++ {
			pe.Attempts = i + 1
			res, err := s.ProcessLyEvent(ctx, ev)
			if err == nil {
				pe.Status = "SUCCESS"
				pe.DeepSOCEventID = asString(res["deepsoc_event_id"])
				lastErr = nil
				break
			}
			lastErr = err
		}
		if lastErr != nil {
			pe.LastError = lastErr.Error()
			failed++
		} else {
			pushed++
		}
		s.Store.SavePushedEvent(pe)
	}
	s.Store.SaveCursor(domain.SyncCursor{Name: "flowshadow_events", LastTS: newestTS})
	_ = s.publish(ctx, "sync.completed", "", "flowshadow", map[string]any{"since": since, "newest_ts": newestTS, "fetched": len(items), "pushed": pushed, "failed": failed})
	return map[string]any{"since": since, "newest_ts": newestTS, "fetched": len(items), "pushed": pushed, "failed": failed}, nil
}

func (s Services) CreateEventFromRequest(body map[string]any) (domain.Event, error) {
	msg := asString(body["message"])
	if msg == "" {
		return domain.Event{}, errors.New("事件消息不能为空")
	}
	eventID := asString(body["event_id"])
	if eventID == "" {
		eventID = newID("evt")
	}
	e := domain.Event{
		EventID:     eventID,
		EventName:   firstNonEmpty(asString(body["event_name"]), asString(body["title"])),
		Title:       asString(body["title"]),
		Message:     msg,
		Context:     asString(body["context"]),
		Source:      firstNonEmpty(asString(body["source"]), "manual"),
		Severity:    firstNonEmpty(asString(body["severity"]), "medium"),
		Category:    asString(body["category"]),
		EventStatus: "pending",
	}
	if obs, ok := body["observables"].([]any); ok {
		for _, item := range obs {
			if m, ok := item.(map[string]any); ok {
				e.Observables = append(e.Observables, domain.IOC{Type: asString(m["type"]), Value: asString(m["value"]), Role: asString(m["role"])})
			}
		}
	}
	created, err := s.Store.CreateEvent(e)
	if err != nil {
		return domain.Event{}, err
	}
	systemMsg := map[string]any{"response_text": "系统创建了安全事件: " + firstNonEmpty(created.EventName, "未命名事件")}
	b, _ := json.Marshal(systemMsg)
	_, _ = s.Store.AddMessage(domain.Message{
		EventID:        created.EventID,
		MessageFrom:    "system",
		MessageType:    "system_notification",
		MessageContent: string(b),
		RoundID:        1,
	})
	_ = s.publish(context.Background(), "event.created", created.EventID, firstNonEmpty(created.Source, "manual"), created)
	return created, nil
}

func (s Services) publish(ctx context.Context, routingKey, eventID, source string, payload interface{}) error {
	if s.Queue == nil || !s.Queue.Enabled() {
		return nil
	}
	return s.Queue.Publish(ctx, routingKey, mq.EventMessage{
		Type:      routingKey,
		EventID:   eventID,
		Source:    source,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
}

func extractEventID(resp map[string]any) string {
	if v := asString(resp["event_id"]); v != "" {
		return v
	}
	if v := asString(resp["id"]); v != "" {
		return v
	}
	if data, ok := resp["data"].(map[string]any); ok {
		if v := asString(data["event_id"]); v != "" {
			return v
		}
		if v := asString(data["id"]); v != "" {
			return v
		}
	}
	return ""
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
