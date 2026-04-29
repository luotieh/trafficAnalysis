package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"traffic-go/internal/config"
	"traffic-go/internal/domain"
	"traffic-go/internal/service"
)

type Server struct {
	cfg      config.Config
	services service.Services
	mux      *http.ServeMux
	tokens   map[string]string
}

func New(cfg config.Config, services service.Services) *Server {
	s := &Server{cfg: cfg, services: services, mux: http.NewServeMux(), tokens: map[string]string{}}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withCORS(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("GET /api/version", s.version)

	s.mux.HandleFunc("POST /internal/event/push", s.internalEventPush)
	s.mux.HandleFunc("POST /internal/sync:run", s.internalSyncRun)
	s.mux.HandleFunc("POST /internal/admin/dedup/reset", s.notImplemented("数据库版建议用 SQL 管理去重表；此入口保留兼容"))
	s.mux.HandleFunc("GET /internal/flows/{flow_id}", s.internalGetFlow)
	s.mux.HandleFunc("GET /internal/flows/{flow_id}/related", s.internalRelatedFlows)
	s.mux.HandleFunc("GET /internal/assets/{ip}", s.internalAsset)
	s.mux.HandleFunc("POST /internal/flows/{flow_id}/pcap:prepare", s.internalPreparePCAP)
	s.mux.HandleFunc("GET /internal/pcaps/{pcap_id}", s.internalGetPCAP)

	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.ok)
	s.mux.HandleFunc("GET /api/auth/me", s.me)
	s.mux.HandleFunc("GET /api/auth/check-auth", s.checkAuth)
	s.mux.HandleFunc("POST /api/auth/init-admin", s.initAdmin)
	s.mux.HandleFunc("POST /api/auth/create-user", s.createUser)
	s.mux.HandleFunc("POST /api/auth/change-password", s.changePassword)

	s.mux.HandleFunc("POST /api/event/create", s.createEvent)
	s.mux.HandleFunc("GET /api/event/list", s.listEvents)
	s.mux.HandleFunc("GET /api/event/{event_id}", s.getEvent)
	s.mux.HandleFunc("GET /api/event/{event_id}/messages", s.getMessages)
	s.mux.HandleFunc("GET /api/event/{event_id}/tasks", s.getTasks)
	s.mux.HandleFunc("GET /api/event/{event_id}/stats", s.getStats)
	s.mux.HandleFunc("GET /api/event/{event_id}/summaries", s.getSummaries)
	s.mux.HandleFunc("POST /api/event/send_message/{event_id}", s.sendEventMessage)
	s.mux.HandleFunc("GET /api/event/{event_id}/executions", s.getExecutions)
	s.mux.HandleFunc("POST /api/event/{event_id}/execution/{execution_id}/complete", s.completeExecution)
	s.mux.HandleFunc("GET /api/event/{event_id}/hierarchy", s.getHierarchy)

	s.mux.HandleFunc("GET /api/user/list", s.listUsers)
	s.mux.HandleFunc("POST /api/user", s.createUser)
	s.mux.HandleFunc("GET /api/user/{user_id}", s.getUser)
	s.mux.HandleFunc("PUT /api/user/{user_id}", s.updateUser)
	s.mux.HandleFunc("DELETE /api/user/{user_id}", s.deleteUser)
	s.mux.HandleFunc("PUT /api/user/{user_id}/password", s.updateUserPassword)

	s.mux.HandleFunc("GET /api/prompt/list", s.promptList)
	s.mux.HandleFunc("GET /api/prompt/{role}", s.promptGet)
	s.mux.HandleFunc("PUT /api/prompt/{role}", s.ok)
	s.mux.HandleFunc("GET /api/prompt/background/{name}", s.promptBackgroundGet)
	s.mux.HandleFunc("PUT /api/prompt/background/{name}", s.ok)

	s.mux.HandleFunc("GET /api/state/driving-mode", s.drivingModeGet)
	s.mux.HandleFunc("PUT /api/state/driving-mode", s.ok)

	s.mux.HandleFunc("POST /api/engineer-chat/send", s.engineerChatSend)
	s.mux.HandleFunc("GET /api/engineer-chat/history", s.engineerChatHistory)
	s.mux.HandleFunc("POST /api/engineer-chat/new-session", s.engineerNewSession)
	s.mux.HandleFunc("GET /api/engineer-chat/status", s.engineerStatus)

	s.mux.HandleFunc("POST /api/report/global", s.reportGlobal)
	s.mux.HandleFunc("/d/", s.flowShadowProxy)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok", "service": "traffic-go", "store_backend": s.cfg.StoreBackend, "mq_backend": s.cfg.MQBackend})
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": "0.3.0-db-mq", "name": "traffic-go"})
}

func (s *Server) internalEventPush(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	var ly map[string]any
	if !decodeJSON(w, r, &ly) {
		return
	}
	res, err := s.services.ProcessLyEvent(r.Context(), ly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalSyncRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	res, err := s.services.RunSyncOnce(r.Context(), s.cfg.SyncBatchSize, s.cfg.SyncLookbackSeconds, s.cfg.SyncMaxRetries)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalGetFlow(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	id := r.PathValue("flow_id")
	s.audit(r, "QUERY_FLOW", id, "")
	res, err := s.services.FlowShadow.GetFlow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalRelatedFlows(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	id := r.PathValue("flow_id")
	q := r.URL.Query()
	limit := 50
	s.audit(r, "QUERY_RELATED", id, q.Encode())
	res, err := s.services.FlowShadow.GetRelatedFlows(r.Context(), id, firstNonEmpty(q.Get("window"), "1h"), firstNonEmpty(q.Get("rel_type"), "src"), limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalAsset(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	ip := r.PathValue("ip")
	s.audit(r, "QUERY_ASSET", ip, "")
	res, err := s.services.FlowShadow.GetAsset(r.Context(), ip)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalPreparePCAP(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	id := r.PathValue("flow_id")
	s.audit(r, "PREPARE_PCAP", id, "")
	res, err := s.services.FlowShadow.PreparePCAP(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) internalGetPCAP(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalKey(w, r) {
		return
	}
	id := r.PathValue("pcap_id")
	s.audit(r, "GET_PCAP", id, "")
	res, err := s.services.FlowShadow.GetPCAP(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, ok := s.services.Store.GetUserByUsername(req.Username)
	if !ok || !u.IsActive || (u.Password != "" && req.Password != u.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token := randomToken()
	s.tokens[token] = u.UserID
	now := time.Now().UTC()
	_, _ = s.services.Store.UpdateUser(u.UserID, map[string]any{"last_login_at": now})
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "access_token": token, "token_type": "bearer", "data": u})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: u})
}

func (s *Server) checkAuth(w http.ResponseWriter, r *http.Request) {
	_, ok := s.currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": ok})
}

func (s *Server) initAdmin(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	u := domain.User{
		UserID:   firstNonEmpty(asString(body["user_id"]), "admin"),
		Username: firstNonEmpty(asString(body["username"]), "admin"),
		Password: firstNonEmpty(asString(body["password"]), "admin"),
		Email:    firstNonEmpty(asString(body["email"]), "admin@example.local"),
		Role:     "admin",
		Nickname: firstNonEmpty(asString(body["nickname"]), "管理员"),
	}
	created, err := s.services.Store.CreateUser(u)
	if err != nil {
		writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Message: "管理员已存在", Data: map[string]any{"username": u.Username}})
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: created})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	u := domain.User{
		UserID:   asString(body["user_id"]),
		Username: asString(body["username"]),
		Nickname: asString(body["nickname"]),
		Email:    asString(body["email"]),
		Phone:    asString(body["phone"]),
		Password: firstNonEmpty(asString(body["password"]), "ChangeMe123!"),
		Role:     firstNonEmpty(asString(body["role"]), "user"),
	}
	if u.Username == "" {
		writeError(w, http.StatusBadRequest, "username不能为空")
		return
	}
	created, err := s.services.Store.CreateUser(u)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: created})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	newPassword := asString(body["new_password"])
	if newPassword == "" {
		newPassword = asString(body["password"])
	}
	if newPassword == "" {
		writeError(w, http.StatusBadRequest, "password不能为空")
		return
	}
	updated, _ := s.services.Store.UpdateUser(u.UserID, map[string]any{"password": newPassword})
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: updated})
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	e, err := s.services.CreateEventFromRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Message: "事件创建成功", Data: e})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListEvents()})
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.services.Store.GetEvent(r.PathValue("event_id"))
	if !ok {
		writeError(w, http.StatusNotFound, "事件不存在")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: e})
}

func (s *Server) getMessages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListMessages(r.PathValue("event_id"))})
}

func (s *Server) getTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListTasks(r.PathValue("event_id"))})
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{
		"event_id":     eventID,
		"messages":     len(s.services.Store.ListMessages(eventID)),
		"tasks":        len(s.services.Store.ListTasks(eventID)),
		"executions":   len(s.services.Store.ListExecutions(eventID)),
		"summaries":    len(s.services.Store.ListSummaries(eventID)),
		"generated_at": time.Now().UTC(),
	}})
}

func (s *Server) getSummaries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListSummaries(r.PathValue("event_id"))})
}

func (s *Server) sendEventMessage(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	msg := domain.Message{
		EventID:        eventID,
		MessageFrom:    firstNonEmpty(asString(body["message_from"]), "engineer"),
		MessageType:    firstNonEmpty(asString(body["message_type"]), "text"),
		MessageContent: firstNonEmpty(asString(body["message"]), asString(body["message_content"])),
		RoundID:        1,
	}
	created, err := s.services.Store.AddMessage(msg)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: created})
}

func (s *Server) getExecutions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListExecutions(r.PathValue("event_id"))})
}

func (s *Server) completeExecution(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Message: "execution completion accepted", Data: map[string]string{"execution_id": r.PathValue("execution_id")}})
}

func (s *Server) getHierarchy(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{
		"event_id":   eventID,
		"messages":   s.services.Store.ListMessages(eventID),
		"tasks":      s.services.Store.ListTasks(eventID),
		"executions": s.services.Store.ListExecutions(eventID),
		"summaries":  s.services.Store.ListSummaries(eventID),
	}})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListUsers()})
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.services.Store.GetUser(r.PathValue("user_id"))
	if !ok {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: u})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	u, ok := s.services.Store.UpdateUser(r.PathValue("user_id"), body)
	if !ok {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: u})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.services.Store.DeleteUser(r.PathValue("user_id")) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Message: "用户已删除"})
}

func (s *Server) updateUserPassword(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	pass := firstNonEmpty(asString(body["password"]), asString(body["new_password"]))
	if pass == "" {
		writeError(w, http.StatusBadRequest, "password不能为空")
		return
	}
	u, ok := s.services.Store.UpdateUser(r.PathValue("user_id"), map[string]any{"password": pass})
	if !ok {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: u})
}

func (s *Server) promptList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]string{
		"captain":  "负责统筹安全事件处置",
		"manager":  "负责拆解任务与分派",
		"expert":   "负责分析证据与给出判断",
		"operator": "负责执行处置动作",
	}})
}

func (s *Server) promptGet(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]string{"role": role, "prompt": "请根据事件上下文进行 SOC 分析。"}})
}

func (s *Server) promptBackgroundGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]string{"name": r.PathValue("name"), "content": ""}})
}

func (s *Server) drivingModeGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{"enabled": false}})
}

func (s *Server) engineerChatSend(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	eventID := asString(body["event_id"])
	message := asString(body["message"])
	if eventID == "" || strings.TrimSpace(message) == "" {
		writeError(w, http.StatusBadRequest, "event_id和message不能为空")
		return
	}
	if _, ok := s.services.Store.GetEvent(eventID); !ok {
		writeError(w, http.StatusNotFound, "事件不存在")
		return
	}
	_, _ = s.services.Store.AddMessage(domain.Message{EventID: eventID, MessageFrom: "engineer", MessageType: "text", MessageContent: message, RoundID: 1})
	reply, err := s.services.LLM.Chat(r.Context(), message)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	m, _ := s.services.Store.AddMessage(domain.Message{EventID: eventID, MessageFrom: "ai", MessageType: "assistant_response", MessageContent: reply, RoundID: 1})
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{"reply": reply, "message": m}})
}

func (s *Server) engineerChatHistory(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: s.services.Store.ListMessages(eventID)})
}

func (s *Server) engineerNewSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{"session_id": randomToken()}})
}

func (s *Server) engineerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{"event_id": r.URL.Query().Get("event_id"), "status": "idle"}})
}

func (s *Server) reportGlobal(w http.ResponseWriter, r *http.Request) {
	events := s.services.Store.ListEvents()
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success", Data: map[string]any{"event_count": len(events), "generated_at": time.Now().UTC()}})
}

func (s *Server) flowShadowProxy(w http.ResponseWriter, r *http.Request) {
	s.services.FlowShadow.Proxy(w, r)
}

func (s *Server) ok(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.APIResponse{Status: "success"})
}

func (s *Server) notImplemented(message string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, domain.APIResponse{Status: "accepted", Message: message})
	}
}

func (s *Server) requireInternalKey(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.InternalAPIKey == "" || r.Header.Get("X-API-Key") == s.cfg.InternalAPIKey {
		return true
	}
	writeError(w, http.StatusUnauthorized, "UNAUTHORIZED")
	return false
}

func (s *Server) currentUser(r *http.Request) (domain.User, bool) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return domain.User{}, false
	}
	userID, ok := s.tokens[token]
	if !ok {
		return domain.User{}, false
	}
	return s.services.Store.GetUser(userID)
}

func (s *Server) audit(r *http.Request, action, target, meta string) {
	actor := firstNonEmpty(r.Header.Get("X-Actor"), "deepsoc")
	s.services.Store.AddAuditLog(domain.AuditLog{Actor: actor, Action: action, Target: target, Meta: meta})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, domain.APIResponse{Status: "error", Message: message})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-Actor")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return strings.Trim(string(b), `"`)
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
