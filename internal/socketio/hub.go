package socketio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"traffic-go/internal/realtime"
	"traffic-go/internal/service"

	"github.com/gorilla/websocket"
)

const recordSeparator = "\x1e"

type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	rooms    map[string]map[string]*Session
	upgrader websocket.Upgrader
}

type Session struct {
	SID     string
	Rooms   map[string]bool
	Queue   chan string
	WS      *websocket.Conn
	Created time.Time
}

func NewHub() *Hub {
	h := &Hub{
		sessions: map[string]*Session{},
		rooms:    map[string]map[string]*Session{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	realtime.Register(h)
	return h
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-Actor")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	transport := r.URL.Query().Get("transport")
	switch transport {
	case "websocket":
		h.serveWebSocket(w, r)
	default:
		h.servePolling(w, r)
	}
}

func (h *Hub) servePolling(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")

	sid := r.URL.Query().Get("sid")
	if r.Method == http.MethodGet && sid == "" {
		s := h.newSession()
		open := map[string]any{
			"sid":          s.SID,
			"upgrades":     []string{"websocket"},
			"pingInterval": 25000,
			"pingTimeout":  20000,
			"maxPayload":   1000000,
		}
		b, _ := json.Marshal(open)
		_, _ = w.Write([]byte("0" + string(b)))
		return
	}

	s := h.getSession(sid)
	if s == nil {
		http.Error(w, "unknown sid", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		for _, packet := range strings.Split(string(body), recordSeparator) {
			h.handleEnginePacket(s, strings.TrimSpace(packet), nil)
		}
		_, _ = w.Write([]byte("ok"))
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		packets := h.drainPackets(ctx, s)
		if len(packets) == 0 {
			// Engine.IO ping. The client should answer with pong packet "3".
			packets = append(packets, "2")
		}
		_, _ = w.Write([]byte(strings.Join(packets, recordSeparator)))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Hub) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sid := r.URL.Query().Get("sid")
	s := h.getSession(sid)
	if s == nil {
		s = h.newSession()
	}
	s.WS = conn

	open := map[string]any{
		"sid":          s.SID,
		"upgrades":     []string{},
		"pingInterval": 25000,
		"pingTimeout":  20000,
		"maxPayload":   1000000,
	}
	b, _ := json.Marshal(open)
	_ = conn.WriteMessage(websocket.TextMessage, []byte("0"+string(b)))

	stopPing := make(chan struct{})
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = conn.WriteMessage(websocket.TextMessage, []byte("2"))
			case <-stopPing:
				return
			}
		}
	}()

	defer func() {
		close(stopPing)
		h.removeSession(s.SID)
		_ = conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		h.handleEnginePacket(s, string(data), conn)
	}
}

func (h *Hub) handleEnginePacket(s *Session, packet string, conn *websocket.Conn) {
	if packet == "" {
		return
	}
	send := func(p string) {
		if conn != nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(p))
			return
		}
		h.enqueue(s, p)
	}

	switch {
	case packet == "2":
		// client ping, answer pong
		send("3")
	case packet == "3":
		// client pong
		return
	case packet == "40":
		// Socket.IO namespace connect ack
		ack, _ := json.Marshal(map[string]any{"sid": s.SID})
		send("40" + string(ack))
	case strings.HasPrefix(packet, "42"):
		h.handleSocketEvent(s, packet[2:])
	}
}

func (h *Hub) handleSocketEvent(s *Session, payload string) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil || len(arr) == 0 {
		h.emitToSession(s, "error", map[string]any{"message": "invalid socket event"})
		return
	}
	var event string
	_ = json.Unmarshal(arr[0], &event)
	data := map[string]any{}
	if len(arr) > 1 {
		_ = json.Unmarshal(arr[1], &data)
	}

	switch event {
	case "join":
		eventID := asString(data["event_id"])
		if eventID == "" {
			h.emitToSession(s, "error", map[string]any{"message": "缺少event_id参数"})
			return
		}
		h.joinRoom(s, eventID)
		h.emitToSession(s, "status", map[string]any{"status": "joined", "event_id": eventID})
		h.emitToSession(s, "test_message", map[string]any{"message": "这是一条测试消息，用于验证WebSocket连接", "timestamp": time.Now().Format(time.RFC3339Nano)})
		h.Broadcast(eventID, "test_message", map[string]any{"message": fmt.Sprintf("客户端 %s 已加入房间 %s", s.SID, eventID), "timestamp": time.Now().Format(time.RFC3339Nano)})
	case "leave":
		eventID := asString(data["event_id"])
		if eventID != "" {
			h.leaveRoom(s, eventID)
			h.emitToSession(s, "status", map[string]any{"status": "left", "event_id": eventID})
		}
	case "message":
		eventID := asString(data["event_id"])
		content := firstNonEmpty(asString(data["message"]), asString(data["message_content"]), asString(data["content"]))
		if eventID == "" || content == "" {
			h.emitToSession(s, "error", map[string]any{"message": "缺少必要参数"})
			return
		}
		sender := service.NormalizeMessageFrom(firstNonEmpty(asString(data["sender"]), asString(data["message_from"]), "user"))
		msg := map[string]any{
			"message_id":      "ws-" + randomID(),
			"event_id":        eventID,
			"message_from":    sender,
			"message_type":    "user_message",
			"sender_type":     service.SenderType(sender),
			"message_content": content,
			"created_at":      time.Now().Format(time.RFC3339Nano),
		}
		if tempID := asString(data["temp_id"]); tempID != "" {
			msg["temp_id"] = tempID
		}
		h.Broadcast(eventID, "new_message", msg)
	case "test_connection":
		eventID := asString(data["event_id"])
		h.emitToSession(s, "test_connection_response", map[string]any{
			"message":           "连接测试成功",
			"timestamp":         time.Now().Format(time.RFC3339Nano),
			"request_timestamp": data["timestamp"],
		})
		if eventID != "" {
			h.Broadcast(eventID, "new_message", map[string]any{
				"message_id":   "ws-test-" + randomID(),
				"event_id":     eventID,
				"message_from": "system",
				"message_type": "system_notification",
				"message_content": map[string]any{
					"type":      "system_notification",
					"timestamp": time.Now().Format(time.RFC3339Nano),
					"data": map[string]any{
						"response_text": fmt.Sprintf("这是一条通过WebSocket发送的测试系统通知 (SID: %s)", s.SID),
					},
				},
				"created_at": time.Now().Format(time.RFC3339Nano),
			})
		}
	}
}

func (h *Hub) Emit(event string, data any) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.sessions {
		h.enqueue(s, socketEventPacket(event, data))
		if s.WS != nil {
			_ = s.WS.WriteMessage(websocket.TextMessage, []byte(socketEventPacket(event, data)))
		}
	}
}

func (h *Hub) Broadcast(room, event string, data any) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.rooms[room] {
		packet := socketEventPacket(event, data)
		h.enqueue(s, packet)
		if s.WS != nil {
			_ = s.WS.WriteMessage(websocket.TextMessage, []byte(packet))
		}
	}
}

func (h *Hub) emitToSession(s *Session, event string, data any) {
	packet := socketEventPacket(event, data)
	h.enqueue(s, packet)
	if s.WS != nil {
		_ = s.WS.WriteMessage(websocket.TextMessage, []byte(packet))
	}
}

func socketEventPacket(event string, data any) string {
	b, _ := json.Marshal([]any{event, data})
	return "42" + string(b)
}

func (h *Hub) newSession() *Session {
	s := &Session{SID: randomID(), Rooms: map[string]bool{}, Queue: make(chan string, 256), Created: time.Now()}
	h.mu.Lock()
	h.sessions[s.SID] = s
	h.mu.Unlock()
	return s
}

func (h *Hub) getSession(sid string) *Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[sid]
}

func (h *Hub) removeSession(sid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[sid]
	if s == nil {
		return
	}
	for room := range s.Rooms {
		delete(h.rooms[room], sid)
	}
	delete(h.sessions, sid)
}

func (h *Hub) joinRoom(s *Session, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s.Rooms[room] = true
	if h.rooms[room] == nil {
		h.rooms[room] = map[string]*Session{}
	}
	h.rooms[room][s.SID] = s
}

func (h *Hub) leaveRoom(s *Session, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(s.Rooms, room)
	delete(h.rooms[room], s.SID)
}

func (h *Hub) enqueue(s *Session, packet string) {
	select {
	case s.Queue <- packet:
	default:
	}
}

func (h *Hub) drainPackets(ctx context.Context, s *Session) []string {
	packets := []string{}
	select {
	case p := <-s.Queue:
		packets = append(packets, p)
	case <-ctx.Done():
		return packets
	}
	for len(packets) < 16 {
		select {
		case p := <-s.Queue:
			packets = append(packets, p)
		default:
			return packets
		}
	}
	return packets
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
