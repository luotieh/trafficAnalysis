package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"traffic-go/internal/client"
	"traffic-go/internal/config"
	"traffic-go/internal/httpapi"
	"traffic-go/internal/service"
	"traffic-go/internal/store"
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
	}

	server := httpapi.New(cfg, svc)
	log.Printf("traffic-go listening on %s, store=%s", cfg.Addr, cfg.StoreBackend)
	if err := http.ListenAndServe(cfg.Addr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
