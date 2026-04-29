package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"

	"traffic-go/internal/config"
	"traffic-go/internal/mq"
	"traffic-go/internal/store"
)

const demoEventID = "demo-event-001"

// Options mirrors the original DeepSOC startup flags:
//
//	-init            => reset/create tables + default prompts + admin user
//	-init-with-demo  => init + demo data
//	-load_demo       => demo data only, requiring an existing schema
//
// It also initializes RabbitMQ topology when MQ_BACKEND=rabbitmq.
type Options struct {
	Init          bool
	InitWithDemo  bool
	LoadDemo      bool
	Reset         bool
	InitMQ        bool
	PublishDemoMQ bool
}

func Run(ctx context.Context, cfg config.Config, opts Options) error {
	if opts.InitWithDemo {
		opts.Init = true
		opts.Reset = true
		opts.LoadDemo = true
		opts.InitMQ = true
		opts.PublishDemoMQ = true
	}
	if cfg.StoreBackend == "" {
		cfg.StoreBackend = "postgres"
	}
	if cfg.StoreBackend != "postgres" {
		return fmt.Errorf("bootstrap currently requires STORE_BACKEND=postgres, got %q", cfg.StoreBackend)
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is empty")
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	if opts.Reset {
		log.Println("bootstrap: resetting database schema")
		if err := ResetSchema(ctx, db); err != nil {
			return err
		}
	}
	if opts.Init || opts.Reset {
		log.Println("bootstrap: creating database schema")
		if _, err := db.ExecContext(ctx, store.PostgresSchema); err != nil {
			return err
		}
		log.Println("bootstrap: seeding default prompts and admin user")
		if err := SeedBase(ctx, db); err != nil {
			return err
		}
	}
	if opts.LoadDemo {
		log.Println("bootstrap: loading demo data")
		if err := SeedDemo(ctx, db); err != nil {
			return err
		}
	}
	if opts.InitMQ && strings.EqualFold(cfg.MQBackend, "rabbitmq") {
		log.Println("bootstrap: initializing rabbitmq exchange/queue/bindings")
		if err := InitRabbitMQ(ctx, cfg); err != nil {
			return err
		}
	}
	if opts.PublishDemoMQ && strings.EqualFold(cfg.MQBackend, "rabbitmq") {
		log.Println("bootstrap: publishing demo event message to rabbitmq")
		if err := PublishDemoEvent(ctx, cfg); err != nil {
			return err
		}
	}
	return nil
}

func ResetSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS pushed_events CASCADE;
DROP TABLE IF EXISTS sync_cursors CASCADE;
DROP TABLE IF EXISTS event_maps CASCADE;
DROP TABLE IF EXISTS summaries CASCADE;
DROP TABLE IF EXISTS executions CASCADE;
DROP TABLE IF EXISTS tasks CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS events CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS prompts CASCADE;
DROP TABLE IF EXISTS settings CASCADE;
`)
	return err
}

func SeedBase(ctx context.Context, db *sql.DB) error {
	prompts := map[string]string{
		"role_soc_captain":    "你是SOC团队总指挥，负责识别威胁、控制风险、安排团队任务并判断事件是否处置完成。",
		"role_soc_manager":    "你是SOC安全管理员，负责把指挥官的任务拆解为可执行动作，并协调分析员、处置员和工程师完成。",
		"role_soc_operator":   "你是一线安全工程师，负责执行查询、处置、通知等具体操作，并按事实返回执行结果。",
		"role_soc_executor":   "你是自动化执行器，负责调用剧本、工具或外部系统完成被授权的动作。",
		"role_soc_expert":     "你是SOC安全专家，负责从独立视角总结事件处置过程、识别遗漏并给出专业建议。",
		"background_security": "默认安全背景：本系统用于流量告警、事件分析、证据查询、响应编排和安全运营复盘。",
	}
	for role, content := range prompts {
		if _, err := db.ExecContext(ctx, `
INSERT INTO prompts (role, content, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (role) DO UPDATE SET content=EXCLUDED.content, updated_at=now()`, role, content); err != nil {
			return err
		}
	}

	_, err := db.ExecContext(ctx, `
INSERT INTO users (user_id, username, nickname, email, phone, password_hash, role, is_active, created_at, updated_at)
VALUES ('admin', 'admin', '管理员', 'admin@deepsoc.local', '18999990000', 'admin123', 'admin', true, now(), now())
ON CONFLICT (username) DO UPDATE SET
  nickname=EXCLUDED.nickname,
  email=EXCLUDED.email,
  phone=EXCLUDED.phone,
  password_hash=EXCLUDED.password_hash,
  role=EXCLUDED.role,
  is_active=true,
  updated_at=now()`)
	return err
}

func SeedDemo(ctx context.Context, db *sql.DB) error {
	observables, _ := json.Marshal([]map[string]string{
		{"type": "ip", "value": "66.240.205.34", "role": "source"},
		{"type": "ip", "value": "172.16.10.10", "role": "destination"},
	})
	contextData, _ := json.Marshal(map[string]any{
		"system_ref":       "demo",
		"ly_id":            "demo-ly-001",
		"event_type":       "Network Threat",
		"detail_type":      "SSH Brute Force",
		"detection_method": "rule",
		"occurrence_time":  time.Now().UTC().Format(time.RFC3339),
	})
	_, err := db.ExecContext(ctx, `
INSERT INTO events (event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,now(),now())
ON CONFLICT (event_id) DO UPDATE SET
  event_name=EXCLUDED.event_name,
  title=EXCLUDED.title,
  message=EXCLUDED.message,
  context=EXCLUDED.context,
  source=EXCLUDED.source,
  severity=EXCLUDED.severity,
  category=EXCLUDED.category,
  event_status=EXCLUDED.event_status,
  current_round=EXCLUDED.current_round,
  observables=EXCLUDED.observables,
  updated_at=now()`, demoEventID, "演示事件：公网 SSH 暴力破解", "演示事件：公网 SSH 暴力破解",
		"检测到 66.240.205.34 对 172.16.10.10 发起多次 SSH 登录尝试，疑似暴力破解。", string(contextData), "demo", "high", "Network Threat", "pending", 1, string(observables))
	if err != nil {
		return err
	}

	messages := []struct {
		ID      string
		From    string
		Type    string
		Content string
	}{
		{"demo-message-001", "system", "system_notification", `{"response_text":"系统创建了演示安全事件。"}`},
		{"demo-message-002", "_captain", "task_assignment", `{"response_text":"请先查询攻击源IP威胁情报和目标资产信息。"}`},
		{"demo-message-003", "queue-worker", "queue_notification", `{"response_text":"演示数据已初始化，等待异步任务处理。"}`},
	}
	for _, m := range messages {
		if _, err := db.ExecContext(ctx, `
INSERT INTO messages (message_id, event_id, user_id, user_nickname, message_from, message_type, message_content, round_id, created_at)
VALUES ($1,$2,'','',$3,$4,$5,1,now())
ON CONFLICT (message_id) DO UPDATE SET message_content=EXCLUDED.message_content`, m.ID, demoEventID, m.From, m.Type, m.Content); err != nil {
			return err
		}
	}

	tasks := []struct {
		ID, Name, Desc, Priority, Assigned string
	}{
		{"demo-task-001", "查询攻击源IP威胁情报", "查询 66.240.205.34 的威胁情报、地理位置和历史攻击记录。", "high", "_analyst"},
		{"demo-task-002", "查询目标资产信息", "查询 172.16.10.10 的资产负责人、业务归属和暴露面。", "medium", "_analyst"},
	}
	for _, t := range tasks {
		if _, err := db.ExecContext(ctx, `
INSERT INTO tasks (task_id, event_id, task_name, task_description, task_status, task_priority, assigned_to, round_id, created_at, updated_at)
VALUES ($1,$2,$3,$4,'pending',$5,$6,1,now(),now())
ON CONFLICT (task_id) DO UPDATE SET task_name=EXCLUDED.task_name, task_description=EXCLUDED.task_description, updated_at=now()`, t.ID, demoEventID, t.Name, t.Desc, t.Priority, t.Assigned); err != nil {
			return err
		}
	}

	_, err = db.ExecContext(ctx, `
INSERT INTO executions (execution_id, event_id, command_id, execution_status, execution_result, command_name, command_type, command_entity, command_params, created_at, updated_at)
VALUES ('demo-execution-001', $1, 'demo-command-001', 'pending', '', '威胁情报查询', 'playbook', '66.240.205.34', '{"ip":"66.240.205.34"}', now(), now())
ON CONFLICT (execution_id) DO UPDATE SET updated_at=now()`, demoEventID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO summaries (event_id, round_id, event_summary, created_at, updated_at)
VALUES ($1, 1, '演示事件已初始化：发现公网IP对内部资产进行SSH暴力破解尝试，建议先完成情报和资产核查。', now(), now())`, demoEventID)
	return err
}

func InitRabbitMQ(ctx context.Context, cfg config.Config) error {
	if strings.TrimSpace(cfg.RabbitMQURL) == "" {
		return fmt.Errorf("RABBITMQ_URL is empty")
	}
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	exchange := firstNonEmpty(cfg.RabbitMQExchange, "traffic.events")
	queue := firstNonEmpty(cfg.RabbitMQEventQueue, "traffic.events.default")
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return err
	}
	for _, key := range []string{"event.*", "sync.*", "evidence.*", "task.*", "llm.*", "report.*"} {
		if err := ch.QueueBind(queue, key, exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func PublishDemoEvent(ctx context.Context, cfg config.Config) error {
	q, err := mq.NewRabbitMQ(ctx, mq.RabbitConfig{
		URL:      cfg.RabbitMQURL,
		Exchange: cfg.RabbitMQExchange,
		Queue:    cfg.RabbitMQEventQueue,
	})
	if err != nil {
		return err
	}
	defer q.Close()
	return q.Publish(ctx, "event.created", mq.EventMessage{
		Type:      "event.created",
		EventID:   demoEventID,
		Source:    "bootstrap-demo",
		Payload:   map[string]any{"message": "demo event initialized"},
		CreatedAt: time.Now().UTC(),
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
