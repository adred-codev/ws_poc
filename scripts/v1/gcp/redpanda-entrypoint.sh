#!/bin/bash
# Redpanda entrypoint with dynamic advertised address
# Uses BACKEND_INTERNAL_IP environment variable to set advertised addresses

set -e

# Default to 'redpanda' hostname if BACKEND_INTERNAL_IP not set
ADVERTISE_HOST="${BACKEND_INTERNAL_IP:-redpanda}"

echo "Starting Redpanda with advertised address: $ADVERTISE_HOST:9092"

# Note: Arguments must be in array format to match docker-compose YAML behavior
exec redpanda start \
  "--kafka-addr" "0.0.0.0:9092" \
  "--advertise-kafka-addr" "$ADVERTISE_HOST:9092" \
  "--pandaproxy-addr" "0.0.0.0:8082" \
  "--advertise-pandaproxy-addr" "$ADVERTISE_HOST:8082" \
  "--schema-registry-addr" "0.0.0.0:8081" \
  "--rpc-addr" "redpanda:33145" \
  "--advertise-rpc-addr" "redpanda:33145" \
  "--mode" "dev-container" \
  "--smp" "1" \
  "--memory" "450M" \
  "--reserve-memory" "50M" \
  "--overprovisioned"
