package realtime

import "sync/atomic"

// Publisher is intentionally small so business code does not depend on a
// concrete Socket.IO implementation. The socketio.Hub implements this method.
type Publisher interface {
	Broadcast(room, event string, data any)
}

var current atomic.Value // stores Publisher

func Register(p Publisher) {
	if p != nil {
		current.Store(p)
	}
}

func Get() Publisher {
	v := current.Load()
	if v == nil {
		return nil
	}
	p, _ := v.(Publisher)
	return p
}

func Broadcast(room, event string, data any) {
	if room == "" || event == "" {
		return
	}
	if p := Get(); p != nil {
		p.Broadcast(room, event, data)
	}
}

// BroadcastMessage emits a DeepSOC-compatible Socket.IO `new_message` event.
// The payload is intentionally the saved message object itself rather than a
// nested wrapper, matching the original chat frontend's expectations.
func BroadcastMessage(eventID string, message any) {
	Broadcast(eventID, "new_message", message)
}

func BroadcastExecutionUpdate(eventID string, payload any) {
	Broadcast(eventID, "execution_update", payload)
}

func BroadcastStatus(eventID string, payload any) {
	Broadcast(eventID, "status", payload)
}
