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

	"traffic-go/internal/config"
)

// InitLyServerCompat initializes PostgreSQL tables that mirror the original
// ly_server t_* data model closely enough for Go-native /d/* replacement.
//
// This replaces the temporary MySQL compatibility layer. It keeps the original
// table names (t_user, t_mo, t_event_data, ...) but stores them in PostgreSQL,
// so the final deployment remains PostgreSQL + RabbitMQ + Go only.
func InitLyServerCompat(ctx context.Context, cfg config.Config) error {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is empty")
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	log.Println("bootstrap: creating ly_server PostgreSQL-compatible schema")
	if _, err := db.ExecContext(ctx, LyServerPostgresSchema); err != nil {
		return fmt.Errorf("create ly_server postgres-compatible schema: %w", err)
	}
	if err := seedLyServerCompatDemo(ctx, db); err != nil {
		return err
	}
	log.Println("bootstrap: ly_server PostgreSQL-compatible schema initialized")
	return nil
}

const LyServerPostgresSchema = `
CREATE TABLE IF NOT EXISTS t_agent (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    version VARCHAR(64) NOT NULL DEFAULT '',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_device (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL DEFAULT '',
    devid VARCHAR(128) UNIQUE NOT NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'online',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_user (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL DEFAULT '',
    role VARCHAR(64) NOT NULL DEFAULT 'admin',
    nickname VARCHAR(128) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_user_session (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL DEFAULT '',
    token VARCHAR(255) UNIQUE NOT NULL,
    remote_addr VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS t_config (
    key VARCHAR(128) PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_mogroup (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_mo (
    id BIGSERIAL PRIMARY KEY,
    moip VARCHAR(64) NOT NULL,
    moport VARCHAR(32) NOT NULL DEFAULT '0',
    protocol VARCHAR(32) NOT NULL DEFAULT '',
    pip VARCHAR(64) NOT NULL DEFAULT '',
    pport VARCHAR(32) NOT NULL DEFAULT '',
    modesc TEXT NOT NULL DEFAULT '',
    tag VARCHAR(128) NOT NULL DEFAULT '',
    mogroupid BIGINT NOT NULL DEFAULT 1,
    filter TEXT NOT NULL DEFAULT '',
    devid VARCHAR(128) NOT NULL DEFAULT '',
    direction VARCHAR(32) NOT NULL DEFAULT 'ALL',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(moip, moport, protocol)
);

CREATE TABLE IF NOT EXISTS t_blacklist (
    id BIGSERIAL PRIMARY KEY,
    value VARCHAR(256) UNIQUE NOT NULL,
    value_type VARCHAR(32) NOT NULL DEFAULT 'ip',
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_whitelist (
    id BIGSERIAL PRIMARY KEY,
    value VARCHAR(256) UNIQUE NOT NULL,
    value_type VARCHAR(32) NOT NULL DEFAULT 'ip',
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_internal_ip_list (
    id BIGSERIAL PRIMARY KEY,
    cidr VARCHAR(64) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_internal_srv_list (
    id BIGSERIAL PRIMARY KEY,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    protocol VARCHAR(32) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(ip, port, protocol)
);

CREATE TABLE IF NOT EXISTS t_event_type (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS t_event_level (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(64) UNIQUE NOT NULL,
    severity VARCHAR(32) NOT NULL DEFAULT 'medium'
);

CREATE TABLE IF NOT EXISTS t_event_status (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(64) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS t_event_action (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS t_event_data (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(128) UNIQUE NOT NULL,
    event_type VARCHAR(128) NOT NULL DEFAULT '',
    detail_type VARCHAR(128) NOT NULL DEFAULT '',
    event_level VARCHAR(64) NOT NULL DEFAULT '中',
    rule_desc TEXT NOT NULL DEFAULT '',
    threat_source VARCHAR(128) NOT NULL DEFAULT '',
    victim_target VARCHAR(128) NOT NULL DEFAULT '',
    method VARCHAR(128) NOT NULL DEFAULT '',
    occurrence_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration INTEGER NOT NULL DEFAULT 0,
    processing_status VARCHAR(64) NOT NULL DEFAULT 'pending',
    is_active BOOLEAN NOT NULL DEFAULT true,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_event_data_aggre (
    id BIGSERIAL PRIMARY KEY,
    aggre_key VARCHAR(256) UNIQUE NOT NULL,
    event_type VARCHAR(128) NOT NULL DEFAULT '',
    event_count INTEGER NOT NULL DEFAULT 0,
    threat_source VARCHAR(128) NOT NULL DEFAULT '',
    victim_target VARCHAR(128) NOT NULL DEFAULT '',
    first_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    severity VARCHAR(32) NOT NULL DEFAULT 'medium',
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_event_ignore (
    id BIGSERIAL PRIMARY KEY,
    rule_key VARCHAR(256) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_asset_ip (
    id BIGSERIAL PRIMARY KEY,
    ip VARCHAR(64) UNIQUE NOT NULL,
    asset_name VARCHAR(256) NOT NULL DEFAULT '',
    owner VARCHAR(128) NOT NULL DEFAULT '',
    business VARCHAR(128) NOT NULL DEFAULT '',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_asset_srv (
    id BIGSERIAL PRIMARY KEY,
    ip VARCHAR(64) NOT NULL,
    port INTEGER NOT NULL DEFAULT 0,
    protocol VARCHAR(32) NOT NULL DEFAULT '',
    service_name VARCHAR(128) NOT NULL DEFAULT '',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(ip, port, protocol)
);

CREATE TABLE IF NOT EXISTS t_asset_host (
    id BIGSERIAL PRIMARY KEY,
    hostname VARCHAR(256) UNIQUE NOT NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    os VARCHAR(128) NOT NULL DEFAULT '',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS t_asset_url (
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL,
    host VARCHAR(256) NOT NULL DEFAULT '',
    title VARCHAR(256) NOT NULL DEFAULT '',
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_t_mo_moip ON t_mo(moip);
CREATE INDEX IF NOT EXISTS idx_t_event_data_time ON t_event_data(occurrence_time DESC);
CREATE INDEX IF NOT EXISTS idx_t_event_data_src_dst ON t_event_data(threat_source, victim_target);
CREATE INDEX IF NOT EXISTS idx_t_event_data_aggre_time ON t_event_data_aggre(last_time DESC);
CREATE INDEX IF NOT EXISTS idx_t_asset_ip_ip ON t_asset_ip(ip);
`

func seedLyServerCompatDemo(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
INSERT INTO t_agent (id, name, ip, port, status, version)
VALUES (1, 'traffic-go-compat-agent', '127.0.0.1', 9010, 'running', 'go-compat')
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, updated_at=now();

INSERT INTO t_device (devid, name, ip, status)
VALUES ('3', 'demo-probe', '127.0.0.1', 'online')
ON CONFLICT (devid) DO UPDATE SET status=EXCLUDED.status, updated_at=now();

INSERT INTO t_user (username, password, role, nickname, enabled)
VALUES ('admin', 'admin', 'admin', '管理员', true)
ON CONFLICT (username) DO UPDATE SET role=EXCLUDED.role, enabled=true, updated_at=now();

INSERT INTO t_mogroup (id, name, description)
VALUES (1, 'default', '默认资产组')
ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description, updated_at=now();

INSERT INTO t_internal_ip_list (cidr, description)
VALUES ('172.16.0.0/12', '演示内网地址段')
ON CONFLICT (cidr) DO NOTHING;

INSERT INTO t_event_type (name, description)
VALUES ('Network Threat', '网络威胁')
ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO t_event_level (name, severity)
VALUES ('高', 'high'), ('中', 'medium'), ('低', 'low')
ON CONFLICT (name) DO UPDATE SET severity=EXCLUDED.severity;

INSERT INTO t_event_status (name, description)
VALUES ('pending', '待处理'), ('processing', '处理中'), ('closed', '已关闭')
ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO t_event_action (name, description)
VALUES ('investigate', '调查'), ('block', '封禁'), ('ignore', '忽略')
ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description;
`); err != nil {
		return err
	}

	moMeta, _ := json.Marshal(map[string]any{"source": "traffic-admin", "compat": true})
	if _, err := db.ExecContext(ctx, `
INSERT INTO t_mo (moip, moport, protocol, modesc, tag, mogroupid, filter, devid, direction, meta)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
ON CONFLICT (moip, moport, protocol) DO UPDATE SET
  modesc=EXCLUDED.modesc,
  tag=EXCLUDED.tag,
  filter=EXCLUDED.filter,
  updated_at=now()
`, "172.16.20.20", "0", "", "兼容测试资产", "compat", 1, "host 172.16.20.20", "3", "ALL", string(moMeta)); err != nil {
		return err
	}

	eventRaw, _ := json.Marshal(map[string]any{
		"ly_id":            "compat-ly-001",
		"detection_method": "rule",
		"system_ref":       "ly_server_compat_postgres",
	})
	if _, err := db.ExecContext(ctx, `
INSERT INTO t_event_data (event_id, event_type, detail_type, event_level, rule_desc, threat_source, victim_target, method, processing_status, raw)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
ON CONFLICT (event_id) DO UPDATE SET raw=EXCLUDED.raw, updated_at=now()
`, "compat-event-ly-001", "Network Threat", "SSH Brute Force", "高", "SSH 暴力破解", "66.240.205.34", "172.16.20.20", "rule", "pending", string(eventRaw)); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO t_event_data_aggre (aggre_key, event_type, event_count, threat_source, victim_target, severity, raw)
VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)
ON CONFLICT (aggre_key) DO UPDATE SET event_count=EXCLUDED.event_count, updated_at=now()
`, "66.240.205.34|172.16.20.20|SSH Brute Force", "Network Threat", 1, "66.240.205.34", "172.16.20.20", "high", string(eventRaw)); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO t_asset_ip (ip, asset_name, owner, business, meta)
VALUES ($1,$2,$3,$4,$5::jsonb)
ON CONFLICT (ip) DO UPDATE SET asset_name=EXCLUDED.asset_name, updated_at=now()
`, "172.16.20.20", "兼容测试资产", "security-team", "demo", string(moMeta)); err != nil {
		return err
	}

	return nil
}
