package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"traffic-go/internal/bootstrap"
	"traffic-go/internal/config"
)

func main() {
	var (
		initOnly            = flag.Bool("init", false, "初始化：重建/创建 PostgreSQL 表 + 默认提示词 + 管理员用户 + RabbitMQ 拓扑")
		initWithDemo        = flag.Bool("init-with-demo", false, "完整初始化：PostgreSQL + RabbitMQ + 演示数据")
		loadDemo            = flag.Bool("load-demo", false, "仅加载 DeepSOC 演示数据，要求 PostgreSQL 表已存在")
		loadDemoLegacy      = flag.Bool("load_demo", false, "兼容原 DeepSOC 参数：等同于 -load-demo")
		reset               = flag.Bool("reset", false, "重置 PostgreSQL schema，危险操作，会删除现有表")
		initMQ              = flag.Bool("init-mq", false, "初始化 RabbitMQ exchange/queue/bindings")
		publishDemoMQ       = flag.Bool("publish-demo-mq", false, "发布一条演示 event.created 消息到 RabbitMQ")
		initLyCompat        = flag.Bool("init-lyserver-compat", false, "初始化 ly_server PostgreSQL 兼容 t_* schema 和基础演示数据")
		initLyServerDBAlias = flag.Bool("init-lyserver-db", false, "兼容旧参数：等同于 -init-lyserver-compat，不再创建 MySQL")
		showVersion         = flag.Bool("version", false, "显示版本信息")
	)
	flag.Parse()

	if *showVersion {
		log.Println("traffic-admin version 0.3.0")
		return
	}

	loadDotEnv(".env")
	cfg := config.Load()

	if *loadDemoLegacy {
		*loadDemo = true
	}
	if *initLyServerDBAlias {
		*initLyCompat = true
	}
	if *initOnly {
		*reset = true
		*initMQ = true
	}
	if *initWithDemo {
		*initLyCompat = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	shouldRunDeepSOCBootstrap := *initOnly || *initWithDemo || *loadDemo || *reset || *initMQ || *publishDemoMQ
	if shouldRunDeepSOCBootstrap {
		if err := bootstrap.Run(ctx, cfg, bootstrap.Options{
			Init:          *initOnly,
			InitWithDemo:  *initWithDemo,
			LoadDemo:      *loadDemo,
			Reset:         *reset,
			InitMQ:        *initMQ,
			PublishDemoMQ: *publishDemoMQ,
		}); err != nil {
			log.Fatalf("bootstrap failed: %v", err)
		}
	}

	if *initLyCompat {
		if err := bootstrap.InitLyServerCompat(ctx, cfg); err != nil {
			log.Fatalf("ly_server postgres-compatible bootstrap failed: %v", err)
		}
	}

	if !shouldRunDeepSOCBootstrap && !*initLyCompat {
		flag.Usage()
		return
	}
	log.Println("bootstrap completed")
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}
