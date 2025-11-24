package rabbitmq

import (
	"context"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "workspace"
	ExchangeType = "topic"
	QueueRequests = "workspace.requests"
	QueueStatus   = "workspace.status"
)

type Producer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewProducer creates a new RabbitMQ producer
func NewProducer(rabbitMQURL string) (*Producer, error) {
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare the exchange
	err = ch.ExchangeDeclare(
		ExchangeName, // name
		ExchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare the requests queue
	_, err = ch.QueueDeclare(
		QueueRequests, // name
		true,          // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare requests queue: %w", err)
	}

	// Bind the queue to the exchange
	err = ch.QueueBind(
		QueueRequests, // queue name
		"*.requested", // routing key pattern (create.requested, release.requested, etc.)
		ExchangeName,  // exchange
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	// Declare the status queue
	_, err = ch.QueueDeclare(
		QueueStatus, // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare status queue: %w", err)
	}

	// Bind status queue to exchange
	err = ch.QueueBind(
		QueueStatus,  // queue name
		"*.status",   // routing key pattern
		ExchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind status queue: %w", err)
	}

	log.Println("✅ RabbitMQ producer connected and queues/exchange declared")

	return &Producer{
		conn:    conn,
		channel: ch,
	}, nil
}

// PublishWorkspaceRequest publishes a workspace request message
func (p *Producer) PublishWorkspaceRequest(ctx context.Context, routingKey string, message []byte) error {
	err := p.channel.PublishWithContext(
		ctx,
		ExchangeName, // exchange
		routingKey,   // routing key (e.g., "create.requested")
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         message,
			DeliveryMode: amqp.Persistent, // persistent messages
		},
	)
	if err != nil {
		log.Printf("❌ Failed to publish message to RabbitMQ: %v", err)
		return fmt.Errorf("failed to publish message: %w", err)
	}
	return nil
}

// Close closes the RabbitMQ connection
func (p *Producer) Close() error {
	if p.channel != nil {
		if err := p.channel.Close(); err != nil {
			log.Printf("Error closing RabbitMQ channel: %v", err)
		}
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
