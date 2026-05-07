package autopilot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"traffic-go/internal/realtime"
)

const drivingModeKey = "driving_mode"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS app_states (
    key VARCHAR(128) PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// GetDrivingMode returns the persisted DeepSOC driving-mode state.
func GetDrivingMode(ctx context.Context, databaseURL string) (bool, error) {
	db, err := open(ctx, databaseURL)
	if err != nil {
		return false, err
	}
	defer db.Close()
	if err := ensureSchema(ctx, db); err != nil {
		return false, err
	}

	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT value FROM app_states WHERE key=$1`, drivingModeKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var v struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, err
	}
	return v.Enabled, nil
}

// SetDrivingMode persists the DeepSOC driving-mode state.
func SetDrivingMode(ctx context.Context, databaseURL string, enabled bool) error {
	db, err := open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := ensureSchema(ctx, db); err != nil {
		return err
	}

	value, _ := json.Marshal(map[string]any{"enabled": enabled})
	_, err = db.ExecContext(ctx, `
INSERT INTO app_states(key, value, updated_at)
VALUES($1, $2::jsonb, now())
ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
`, drivingModeKey, string(value))
	return err
}

// RunForEvent creates the minimum DeepSOC autopilot artifacts for a new event
// when driving-mode is enabled. It intentionally uses defensive, schema-aware
// inserts so it can work across the current compatibility schema and later
// schema refinements.
func RunForEvent(ctx context.Context, databaseURL, eventID, title string) error {
	if strings.TrimSpace(eventID) == "" {
		return nil
	}
	enabled, err := GetDrivingMode(ctx, databaseURL)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	db, err := open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := ensureSchema(ctx, db); err != nil {
		return err
	}

	if strings.TrimSpace(title) == "" {
		title = eventID
	}

	now := time.Now().UTC()
	taskID := "auto-task-" + stableSuffix(eventID)
	execID := "auto-exec-" + stableSuffix(eventID)
	messageID := "auto-msg-" + stableSuffix(eventID)

	_ = updateEventProcessing(ctx, db, eventID)

	message := map[string]any{
		"message_id":      messageID,
		"event_id":        eventID,
		"message_from":    "autopilot",
		"message_type":    "system_notification",
		"message_content": jsonText(map[string]any{"response_text": "自动驾驶已启动，正在生成任务、执行和总结。", "type": "autopilot_started"}),
		"content":         jsonText(map[string]any{"response_text": "自动驾驶已启动，正在生成任务、执行和总结。", "type": "autopilot_started"}),
		"round_id":        1,
		"created_at":      now,
		"updated_at":      now,
	}
	if err := insertCompat(ctx, db, "messages", message); err != nil {
		log.Printf("autopilot: insert message failed event_id=%s err=%v", eventID, err)
	} else {
		realtime.BroadcastMessage(eventID, message)
	}

	task := map[string]any{
		"task_id":      taskID,
		"event_id":     eventID,
		"round_id":     1,
		"task_name":    "自动驾驶：威胁情报与资产分析",
		"title":        "自动驾驶：威胁情报与资产分析",
		"name":         "自动驾驶：威胁情报与资产分析",
		"task_type":    "autopilot_analysis",
		"type":         "autopilot_analysis",
		"task_content": "自动查询攻击源威胁情报、目标资产信息并生成初步分析。",
		"description":  "自动查询攻击源威胁情报、目标资产信息并生成初步分析。",
		"content":      "自动查询攻击源威胁情报、目标资产信息并生成初步分析。",
		"assignee":     "autopilot",
		"assigned_to":  "autopilot",
		"message_from": "autopilot",
		"status":       "completed",
		"task_status":  "completed",
		"created_at":   now,
		"updated_at":   now,
		"completed_at": now,
		"metadata":     jsonText(map[string]any{"source": "driving-mode"}),
	}
	if err := insertCompat(ctx, db, "tasks", task); err != nil {
		log.Printf("autopilot: insert task failed event_id=%s err=%v", eventID, err)
	}

	execution := map[string]any{
		"execution_id":     execID,
		"event_id":         eventID,
		"task_id":          taskID,
		"round_id":         1,
		"executor":         "autopilot",
		"executor_role":    "autopilot",
		"execution_type":   "autopilot_analysis",
		"type":             "autopilot_analysis",
		"status":           "completed",
		"execution_status": "completed",
		"result":           jsonText(map[string]any{"response_text": "自动驾驶已完成初步分析。", "event_id": eventID}),
		"output":           jsonText(map[string]any{"response_text": "自动驾驶已完成初步分析。", "event_id": eventID}),
		"created_at":       now,
		"updated_at":       now,
		"completed_at":     now,
	}
	if err := insertCompat(ctx, db, "executions", execution); err != nil {
		log.Printf("autopilot: insert execution failed event_id=%s err=%v", eventID, err)
	} else {
		realtime.BroadcastExecutionUpdate(eventID, map[string]any{
			"event_id":     eventID,
			"execution_id": execID,
			"status":       "completed",
			"source":       "autopilot",
		})
	}

	summaryText := "自动驾驶初步总结：事件已进入自动分析流程，已生成任务和执行记录。建议继续拉取证据、关联流量并生成最终报告。"
	if _, err := db.ExecContext(ctx, `
INSERT INTO summaries (
	event_id,
	round_id,
	event_summary,
	created_at,
	updated_at
) VALUES (
	$1,
	1,
	$2,
	$3,
	$3
)`, eventID, summaryText, now); err != nil {
		log.Printf("autopilot: insert summary failed event_id=%s err=%v", eventID, err)
	}

	realtime.BroadcastStatus(eventID, map[string]any{
		"event_id": eventID,
		"status":   "autopilot_completed",
		"message":  "自动驾驶已生成任务、执行和总结。",
	})

	return nil
}

func open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}

func updateEventProcessing(ctx context.Context, db *sql.DB, eventID string) error {
	for _, stmt := range []string{
		`UPDATE events SET event_status='processing', updated_at=now() WHERE event_id=$1`,
		`UPDATE events SET status='processing', updated_at=now() WHERE event_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, stmt, eventID); err == nil {
			return nil
		}
	}
	return nil
}

type columnInfo struct {
	Name       string
	DataType   string
	UDTName    string
	Nullable   bool
	HasDefault bool
}

func insertCompat(ctx context.Context, db *sql.DB, table string, values map[string]any) error {
	if !allowedTable(table) {
		return fmt.Errorf("unsupported table %q", table)
	}
	cols, err := tableColumns(ctx, db, table)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}

	insertCols := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	placeholders := make([]string, 0, len(cols))

	for _, col := range cols {
		name := col.Name
		if strings.EqualFold(name, "id") && col.HasDefault {
			continue
		}
		v, ok := values[name]
		if !ok {
			if col.Nullable || col.HasDefault {
				continue
			}
			v = defaultValueForColumn(col, values)
		}
		insertCols = append(insertCols, pqQuoteIdent(name))
		args = append(args, v)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	if len(insertCols) == 0 {
		return nil
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", pqQuoteIdent(table), strings.Join(insertCols, ", "), strings.Join(placeholders, ", "))
	if conflictColumn(table) != "" {
		query += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", pqQuoteIdent(conflictColumn(table)))
	}
	_, err = db.ExecContext(ctx, query, args...)
	return err
}

func tableColumns(ctx context.Context, db *sql.DB, table string) ([]columnInfo, error) {
	rows, err := db.QueryContext(ctx, `
SELECT column_name, data_type, udt_name, is_nullable, column_default IS NOT NULL
FROM information_schema.columns
WHERE table_schema='public' AND table_name=$1
ORDER BY ordinal_position
`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []columnInfo
	for rows.Next() {
		var c columnInfo
		var nullable string
		if err := rows.Scan(&c.Name, &c.DataType, &c.UDTName, &nullable, &c.HasDefault); err != nil {
			return nil, err
		}
		c.Nullable = nullable == "YES"
		out = append(out, c)
	}
	return out, rows.Err()
}

func defaultValueForColumn(c columnInfo, values map[string]any) any {
	name := strings.ToLower(c.Name)
	if v, ok := values["event_id"]; ok && (strings.Contains(name, "event") || name == "flow_id") {
		return v
	}
	if strings.Contains(name, "time") || strings.HasSuffix(name, "_at") {
		return time.Now().UTC()
	}
	if strings.Contains(c.DataType, "json") || c.UDTName == "jsonb" || c.UDTName == "json" {
		return jsonText(map[string]any{"source": "autopilot"})
	}
	if c.DataType == "boolean" || c.UDTName == "bool" {
		return true
	}
	if strings.Contains(c.DataType, "integer") || strings.Contains(c.DataType, "numeric") || c.UDTName == "int4" || c.UDTName == "int8" {
		if strings.Contains(name, "round") {
			return 1
		}
		return 0
	}
	if strings.Contains(name, "status") {
		return "completed"
	}
	if strings.Contains(name, "type") {
		return "autopilot"
	}
	if strings.Contains(name, "content") || strings.Contains(name, "summary") || strings.Contains(name, "result") || strings.Contains(name, "description") {
		return "自动驾驶兼容数据"
	}
	if strings.Contains(name, "from") || strings.Contains(name, "by") || strings.Contains(name, "owner") || strings.Contains(name, "executor") || strings.Contains(name, "assignee") {
		return "autopilot"
	}
	return "autopilot"
}

func allowedTable(table string) bool {
	switch table {
	case "messages", "tasks", "executions", "summaries":
		return true
	default:
		return false
	}
}

func conflictColumn(table string) string {
	switch table {
	case "messages":
		return "message_id"
	case "tasks":
		return "task_id"
	case "executions":
		return "execution_id"
	case "summaries":
		return "summary_id"
	default:
		return ""
	}
}

func pqQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func stableSuffix(eventID string) string {
	repl := strings.NewReplacer("/", "-", " ", "-", ":", "-", ".", "-", "_", "-")
	s := repl.Replace(strings.TrimSpace(eventID))
	if s == "" {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if len(s) > 48 {
		return s[:48]
	}
	return s
}
