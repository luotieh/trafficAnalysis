package worker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"traffic-go/internal/config"
	"traffic-go/internal/domain"
	"traffic-go/internal/mq"
	"traffic-go/internal/store"
)

func StartRabbitEventWorker(ctx context.Context, cfg config.Config, st store.Store) {
	if strings.ToLower(cfg.MQBackend) != "rabbitmq" || !cfg.RabbitMQConsumerEnabled {
		return
	}
	go func() {
		for {
			if err := runRabbitEventWorker(ctx, cfg, st); err != nil {
				log.Printf("rabbitmq worker stopped: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

func runRabbitEventWorker(ctx context.Context, cfg config.Config, st store.Store) error {
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
	queueName := firstNonEmpty(cfg.RabbitMQEventQueue, "traffic.events.default")
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	for _, key := range []string{"event.*", "sync.*", "evidence.*"} {
		if err := ch.QueueBind(q.Name, key, exchange, false, nil); err != nil {
			return err
		}
	}
	if err := ch.Qos(5, 0, false); err != nil {
		return err
	}

	deliveries, err := ch.Consume(q.Name, "traffic-go-worker", false, false, false, false, nil)
	if err != nil {
		return err
	}
	log.Printf("rabbitmq worker consuming queue=%s exchange=%s", q.Name, exchange)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return nil
			}
			if err := handleEventMessage(st, d.RoutingKey, d.Body); err != nil {
				log.Printf("rabbitmq handle failed routing_key=%s err=%v", d.RoutingKey, err)
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func handleEventMessage(st store.Store, routingKey string, body []byte) error {
	var msg mq.EventMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}
	if msg.EventID == "" {
		return nil
	}
	content := map[string]any{
		"response_text": "异步队列已接收事件，后续可在此扩展证据拉取、AI分析、任务编排。",
		"routing_key":   routingKey,
		"source":        msg.Source,
		"created_at":    msg.CreatedAt,
	}
	b, _ := json.Marshal(content)
	_, err := st.AddMessage(domain.Message{
		EventID:        msg.EventID,
		MessageFrom:    "queue-worker",
		MessageType:    "queue_notification",
		MessageContent: string(b),
		RoundID:        1,
	})
	return err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
