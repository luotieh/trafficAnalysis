package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"traffic-go/internal/client"
	"traffic-go/internal/config"
	"traffic-go/internal/httpapi"
	"traffic-go/internal/mq"
	"traffic-go/internal/service"
	"traffic-go/internal/store"
	"traffic-go/internal/worker"
)

func main() {
	cfg := config.Load()
	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}

	var st store.Store
	switch cfg.StoreBackend {
	case "postgres":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pg, err := store.NewPostgresStore(ctx, cfg.DatabaseURL, cfg.AutoMigrate)
		if err != nil {
			log.Fatalf("init postgres store failed: %v", err)
		}
		st = pg
	default:
		log.Printf("STORE_BACKEND=%q, using memory store", cfg.StoreBackend)
		st = store.NewMemoryStore()
	}

	var queue mq.Queue = mq.NoopQueue{}
	if cfg.MQBackend == "rabbitmq" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rabbit, err := mq.NewRabbitMQ(ctx, mq.RabbitConfig{
			URL:      cfg.RabbitMQURL,
			Exchange: cfg.RabbitMQExchange,
			Queue:    cfg.RabbitMQEventQueue,
		})
		if err != nil {
			log.Fatalf("init rabbitmq failed: %v", err)
		}
		defer rabbit.Close()
		queue = rabbit
		worker.StartRabbitEventWorker(context.Background(), cfg, st)
	}

	svc := service.Services{
		Store: st,
		DeepSOC: client.DeepSOCClient{
			BaseURL:  cfg.DeepSOCBaseURL,
			APIKey:   cfg.DeepSOCAPIKey,
			Username: cfg.DeepSOCUsername,
			Password: cfg.DeepSOCPassword,
			HTTP:     httpClient,
		},
		FlowShadow: client.FlowShadowClient{
			BaseURL: cfg.FlowShadowBaseURL,
			APIKey:  cfg.FlowShadowAPIKey,
			HTTP:    httpClient,
		},
		LLM: client.LLMClient{
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
			HTTP:    httpClient,
		},
		Queue: queue,
	}

	server := httpapi.New(cfg, svc)
	log.Printf("traffic-go listening on %s, store=%s, mq=%s", cfg.Addr, cfg.StoreBackend, cfg.MQBackend)
	if err := http.ListenAndServe(cfg.Addr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
