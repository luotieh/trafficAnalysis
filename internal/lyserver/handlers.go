package lyserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Service struct {
	db *sql.DB
}

func New(databaseURL string) *Service {
	if strings.TrimSpace(databaseURL) == "" {
		return &Service{}
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return &Service{}
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	return &Service{db: db}
}

func (s *Service) Enabled() bool { return s != nil && s.db != nil }

func (s *Service) Auth(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	params := readParams(r)
	username := firstNonEmpty(params["username"], params["user"], params["name"], "admin")
	password := firstNonEmpty(params["password"], params["passwd"], params["pwd"], "")

	var row struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Password string `json:"-"`
		Role     string `json:"role"`
		Nickname string `json:"nickname"`
	}
	err := s.db.QueryRowContext(r.Context(), `
SELECT id, username, password, role, nickname
FROM t_user WHERE username=$1 AND enabled=true`, username).
		Scan(&row.ID, &row.Username, &row.Password, &row.Role, &row.Nickname)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if row.Password != "" && password != "" && row.Password != password {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token := fmt.Sprintf("ly-%d", time.Now().UTC().UnixNano())
	_, _ = s.db.ExecContext(r.Context(), `
INSERT INTO t_user_session (username, token, remote_addr, created_at, expires_at)
VALUES ($1,$2,$3,now(),now()+interval '12 hours')
ON CONFLICT (token) DO NOTHING`, row.Username, token, r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"result": "ok",
		"token":  token,
		"data": map[string]any{
			"token": token,
			"user":  map[string]any{"id": row.ID, "username": row.Username, "role": row.Role, "nickname": row.Nickname},
		},
	})
}

func (s *Service) Status(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	agents, err := s.queryRows(r.Context(), `SELECT id, name, ip, port, status, version, created_at, updated_at FROM t_agent ORDER BY id`, []string{"id", "name", "ip", "port", "status", "version", "created_at", "updated_at"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	devices, err := s.queryRows(r.Context(), `SELECT id, name, devid, ip, status, created_at, updated_at FROM t_device ORDER BY id`, []string{"id", "name", "devid", "ip", "status", "created_at", "updated_at"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"result":  "ok",
		"agents":  agents,
		"devices": devices,
		"data": map[string]any{
			"service": "traffic-go-lyserver-compat",
			"status":  "running",
			"agents":  agents,
			"devices": devices,
		},
	})
}

func (s *Service) GetConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	key := firstNonEmpty(r.URL.Query().Get("key"), r.URL.Query().Get("name"))
	if key != "" {
		var value []byte
		var desc string
		var updated time.Time
		err := s.db.QueryRowContext(r.Context(), `SELECT value, description, updated_at FROM t_config WHERE key=$1`, key).Scan(&value, &desc, &updated)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{}})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{"key": key, "value": jsonValue(value), "description": desc, "updated_at": updated}})
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT key, value, description, updated_at FROM t_config ORDER BY key`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var k, desc string
		var v []byte
		var updated time.Time
		if err := rows.Scan(&k, &v, &desc, &updated); err == nil {
			items = append(items, map[string]any{"key": k, "value": jsonValue(v), "description": desc, "updated_at": updated})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": items})
}

func (s *Service) SetConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	params := readParams(r)
	key := firstNonEmpty(params["key"], params["name"], "default")
	desc := firstNonEmpty(params["description"], params["desc"])
	value := params["value"]
	if value == "" {
		b, _ := json.Marshal(params)
		value = string(b)
	}
	if !json.Valid([]byte(value)) {
		b, _ := json.Marshal(value)
		value = string(b)
	}
	_, err := s.db.ExecContext(r.Context(), `
INSERT INTO t_config (key, value, description, updated_at)
VALUES ($1,$2::jsonb,$3,now())
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, description=EXCLUDED.description, updated_at=now()`, key, value, desc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{"key": key, "value": jsonValue([]byte(value)), "description": desc}})
}

func (s *Service) GetMO(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	moip := r.URL.Query().Get("moip")
	if moip == "" {
		moip = r.URL.Query().Get("ip")
	}
	query := `SELECT id, moip, moport, protocol, pip, pport, modesc, tag, mogroupid, filter, devid, direction, meta, created_at, updated_at FROM t_mo`
	args := []any{}
	if moip != "" {
		query += ` WHERE moip=$1`
		args = append(args, moip)
	}
	query += ` ORDER BY id LIMIT 500`
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, groupID int64
		var ip, port, protocol, pip, pport, desc, tag, filter, devid, direction string
		var meta []byte
		var created, updated time.Time
		if err := rows.Scan(&id, &ip, &port, &protocol, &pip, &pport, &desc, &tag, &groupID, &filter, &devid, &direction, &meta, &created, &updated); err == nil {
			items = append(items, map[string]any{"id": id, "moip": ip, "moport": port, "protocol": protocol, "pip": pip, "pport": pport, "modesc": desc, "tag": tag, "mogroupid": groupID, "filter": filter, "devid": devid, "direction": direction, "meta": jsonValue(meta), "created_at": created, "updated_at": updated})
		}
	}
	data := any(items)
	if moip != "" && len(items) == 1 {
		data = items[0]
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": data, "items": items})
}

func (s *Service) SetMO(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	params := readParams(r)
	moip := firstNonEmpty(params["moip"], params["ip"])
	if moip == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "success",
			"result": "ok",
			"data": map[string]any{
				"noop":    true,
				"message": "moip为空，未修改监控对象",
			},
		})
		return
	}
	moport := firstNonEmpty(params["moport"], params["port"], "0")
	protocol := params["protocol"]
	desc := firstNonEmpty(params["modesc"], params["description"], params["desc"])
	tag := params["tag"]
	filter := firstNonEmpty(params["filter"], "host "+moip)
	devid := firstNonEmpty(params["devid"], "3")
	direction := firstNonEmpty(params["direction"], "ALL")
	meta, _ := json.Marshal(params)

	var id int64
	err := s.db.QueryRowContext(r.Context(), `
INSERT INTO t_mo (moip, moport, protocol, modesc, tag, mogroupid, filter, devid, direction, meta, updated_at)
VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8,$9::jsonb,now())
ON CONFLICT (moip, moport, protocol) DO UPDATE SET
  modesc=EXCLUDED.modesc,
  tag=EXCLUDED.tag,
  filter=EXCLUDED.filter,
  devid=EXCLUDED.devid,
  direction=EXCLUDED.direction,
  meta=EXCLUDED.meta,
  updated_at=now()
RETURNING id`, moip, moport, protocol, desc, tag, filter, devid, direction, string(meta)).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{"id": id, "moip": moip, "moport": moport, "protocol": protocol, "modesc": desc}})
}

func (s *Service) GetBWList(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	listType := normalizeListType(firstNonEmpty(r.URL.Query().Get("type"), r.URL.Query().Get("list_type"), r.URL.Query().Get("op")))
	out := map[string]any{"status": "success", "result": "ok"}
	if listType == "black" || listType == "" {
		items, err := s.listBW(r.Context(), "t_blacklist")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out["blacklist"] = items
		if listType == "black" {
			out["data"] = items
		}
	}
	if listType == "white" || listType == "" {
		items, err := s.listBW(r.Context(), "t_whitelist")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out["whitelist"] = items
		if listType == "white" {
			out["data"] = items
		}
	}
	if _, ok := out["data"]; !ok {
		out["data"] = map[string]any{"blacklist": out["blacklist"], "whitelist": out["whitelist"]}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) SetBWList(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	params := readParams(r)
	listType := normalizeListType(firstNonEmpty(params["type"], params["list_type"], "black"))
	table := "t_blacklist"
	if listType == "white" {
		table = "t_whitelist"
	}
	value := firstNonEmpty(params["value"], params["ip"], params["domain"], params["target"])
	if value == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "success",
			"result": "ok",
			"data": map[string]any{
				"noop":    true,
				"message": "value为空，未修改黑白名单",
			},
		})
		return
	}
	op := strings.ToLower(firstNonEmpty(params["op"], params["action"], "add"))
	if op == "del" || op == "delete" || op == "remove" {
		_, err := s.db.ExecContext(r.Context(), fmt.Sprintf(`DELETE FROM %s WHERE value=$1`, table), value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "deleted": value})
		return
	}
	desc := firstNonEmpty(params["description"], params["desc"])
	valueType := firstNonEmpty(params["value_type"], "ip")
	_, err := s.db.ExecContext(r.Context(), fmt.Sprintf(`
INSERT INTO %s (value, value_type, description, enabled, updated_at)
VALUES ($1,$2,$3,true,now())
ON CONFLICT (value) DO UPDATE SET value_type=EXCLUDED.value_type, description=EXCLUDED.description, enabled=true, updated_at=now()`, table), value, valueType, desc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "result": "ok", "data": map[string]any{"type": listType, "value": value, "value_type": valueType, "description": desc}})
}

func (s *Service) requireDB(w http.ResponseWriter) bool {
	if s == nil || s.db == nil {
		writeError(w, http.StatusBadGateway, "ly_server PostgreSQL compatibility database is not configured")
		return false
	}
	if err := s.db.Ping(); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return false
	}
	return true
}

func (s *Service) listBW(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, value, value_type, description, enabled, created_at, updated_at FROM %s ORDER BY id`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var value, valueType, desc string
		var enabled bool
		var created, updated time.Time
		if err := rows.Scan(&id, &value, &valueType, &desc, &enabled, &created, &updated); err == nil {
			items = append(items, map[string]any{"id": id, "value": value, "value_type": valueType, "description": desc, "enabled": enabled, "created_at": created, "updated_at": updated})
		}
	}
	return items, nil
}

func (s *Service) queryRows(ctx context.Context, query string, names []string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		m := map[string]any{}
		for i, name := range names {
			m[name] = normalizeDBValue(vals[i])
		}
		items = append(items, m)
	}
	return items, nil
}

func readParams(r *http.Request) map[string]string {
	params := map[string]string{}
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			params[k] = vals[0]
		}
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		defer r.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			for k, v := range body {
				params[k] = fmt.Sprint(v)
			}
		}
		return params
	}
	_ = r.ParseForm()
	for k, vals := range r.PostForm {
		if len(vals) > 0 {
			params[k] = vals[0]
		}
	}
	return params
}

func normalizeListType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "white", "whitelist", "allow", "allowlist":
		return "white"
	case "black", "blacklist", "deny", "denylist", "block":
		return "black"
	default:
		return ""
	}
}

func jsonValue(b []byte) any {
	if len(b) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return string(b)
	}
	return out
}

func normalizeDBValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case int64, int, int32, bool, string, nil:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"status": "error", "result": "error", "message": message, "code": strconv.Itoa(status)})
}
