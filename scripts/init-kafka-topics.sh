#!/bin/bash

echo "Waiting for Redpanda to be ready..."
sleep 10

echo "Creating Kafka topics..."

# Create main topics that services actually use
docker exec redpanda rpk topic create workspace.cmd --brokers localhost:9092 || true
docker exec redpanda rpk topic create workspace.requests --brokers localhost:9092 || true
docker exec redpanda rpk topic create workspace.status --brokers localhost:9092 || true

echo "Kafka topics created successfully!"
