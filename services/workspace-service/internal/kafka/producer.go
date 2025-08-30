package kafka

import (
	"context"
	"log"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokerURL, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokerURL),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &Producer{writer: writer}
}

func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	err := p.writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
	if err != nil {
		log.Printf("failed to write kafka message: %v", err)
		return err
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
