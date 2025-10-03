package kafka

import (
	"context"
	"fmt"
	"net"

	"github.com/segmentio/kafka-go"
)

func EnsureTopicExists(brokerURL, topic string) error {
	conn, err := kafka.Dial("tcp", brokerURL)
	if err != nil {
		return fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerConn, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, fmt.Sprintf("%d", controller.Port)))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:         topic,
			NumPartitions: 1,
			ReplicationFactor: 1,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		return nil
	}

	return nil
}

func CreateRequiredTopics(ctx context.Context, brokerURL string) error {
	topics := []string{
		"workspace.requests",
		"workspace.status",
		"workspace.test.requests",
		"workspace.test.status",
	}

	for _, topic := range topics {
		if err := EnsureTopicExists(brokerURL, topic); err != nil {
			return fmt.Errorf("failed to create topic %s: %w", topic, err)
		}
	}

	return nil
}
