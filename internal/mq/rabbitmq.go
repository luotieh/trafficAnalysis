package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
	queue    string
}

type RabbitConfig struct {
	URL      string
	Exchange string
	Queue    string
}

func NewRabbitMQ(ctx context.Context, cfg RabbitConfig) (*RabbitMQ, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("rabbitmq url is empty")
	}
	if cfg.Exchange == "" {
		cfg.Exchange = "traffic.events"
	}
	if cfg.Queue == "" {
		cfg.Queue = "traffic.events.default"
	}

	connCh := make(chan struct {
		conn *amqp.Connection
		err  error
	}, 1)
	go func() {
		conn, err := amqp.Dial(cfg.URL)
		connCh <- struct {
			conn *amqp.Connection
			err  error
		}{conn: conn, err: err}
	}()

	var conn *amqp.Connection
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-connCh:
		if res.err != nil {
			return nil, res.err
		}
		conn = res.conn
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ch.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	if _, err := ch.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	for _, key := range []string{"event.*", "sync.*", "evidence.*"} {
		if err := ch.QueueBind(cfg.Queue, key, cfg.Exchange, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, err
		}
	}

	return &RabbitMQ{conn: conn, ch: ch, exchange: cfg.Exchange, queue: cfg.Queue}, nil
}

func (r *RabbitMQ) Enabled() bool { return r != nil && r.conn != nil && r.ch != nil }

func (r *RabbitMQ) Publish(ctx context.Context, routingKey string, msg EventMessage) error {
	if !r.Enabled() {
		return nil
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.ch.PublishWithContext(ctx, r.exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	var err error
	if r.ch != nil {
		err = r.ch.Close()
	}
	if r.conn != nil {
		if e := r.conn.Close(); err == nil {
			err = e
		}
	}
	return err
}
