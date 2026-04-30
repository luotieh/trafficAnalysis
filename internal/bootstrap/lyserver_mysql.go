package bootstrap

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"traffic-go/internal/config"
)

//go:embed sql/ly_server_init.sql
var lyServerSQLFS embed.FS

// InitLyServerDB initializes the original ly_server MySQL schema.
// It intentionally uses the original ly_server dump as the source of truth
// so that Go can later replace /d/* APIs while reading the same t_* tables.
func InitLyServerDB(ctx context.Context, cfg config.Config) error {
	if cfg.LYDBBackend != "" && cfg.LYDBBackend != "mysql" {
		return fmt.Errorf("unsupported LY_DB_BACKEND=%q", cfg.LYDBBackend)
	}
	if strings.TrimSpace(cfg.LYDatabaseDSN) == "" {
		return fmt.Errorf("LY_DATABASE_DSN is empty")
	}

	db, err := sql.Open("mysql", ensureMultiStatements(cfg.LYDatabaseDSN))
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

	sqlBytes, err := lyServerSQLFS.ReadFile("sql/ly_server_init.sql")
	if err != nil {
		return err
	}

	log.Println("bootstrap: importing original ly_server MySQL schema")
	if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("import ly_server mysql schema: %w", err)
	}

	if err := seedLyServerDemo(ctx, db); err != nil {
		return err
	}
	log.Println("bootstrap: ly_server mysql schema initialized")
	return nil
}

func ensureMultiStatements(dsn string) string {
	if strings.Contains(dsn, "multiStatements=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "multiStatements=true"
}

func seedLyServerDemo(ctx context.Context, db *sql.DB) error {
	// Keep this seed intentionally small. The original dump already contains
	// built-in dictionaries, admin user, event types, levels, actions and status rows.
	// These rows add a stable smoke-test asset for /d/mo and related compatibility work.
	_, err := db.ExecContext(ctx, `
INSERT INTO t_mo (moip, moport, protocol, pip, pport, modesc, tag, mogroupid, filter, devid, direction)
SELECT '172.16.20.20', '0', '', '', '', '兼容测试资产', 'compat', 1, 'host 172.16.20.20', 3, 'ALL'
WHERE NOT EXISTS (SELECT 1 FROM t_mo WHERE moip='172.16.20.20' AND modesc='兼容测试资产');
`)
	if err != nil {
		// Some historical dumps use slightly different t_mo indexes. Do not fail the
		// whole initialization because this is demo data, not schema-critical data.
		log.Printf("bootstrap: seed t_mo demo skipped: %v", err)
	}
	return nil
}
