package mq

import (
	"context"
	"time"
)

type EventMessage struct {
	Type      string      `json:"type"`
	EventID   string      `json:"event_id,omitempty"`
	Source    string      `json:"source,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

type Queue interface {
	Enabled() bool
	Publish(ctx context.Context, routingKey string, msg EventMessage) error
	Close() error
}

type NoopQueue struct{}

func (NoopQueue) Enabled() bool                                       { return false }
func (NoopQueue) Publish(context.Context, string, EventMessage) error { return nil }
func (NoopQueue) Close() error                                        { return nil }
