# Commit Summary for kafka-refactor branch (since November 12, 2025 PHT)

This report summarizes the development activity on the `kafka-refactor` branch on or after November 12, 2025, Philippine Time, grouped by the type of change.

## ✨ Features

*   Increase shard count from 3 to 7 to match CPU cores
*   Scale up to e2-highcpu-8 (8 vCPU) with 7 CPU limit
*   Add per-shard and system metrics to LoadBalancer health endpoint
*   Add startup logging for bind/advertise addresses debugging

## 🐛 Fixes

*   Change LoadBalancer port from 3005 to 3001 to avoid conflict with Shard 3
*   Preserve WS_MODE from .env.production in GCP deployment
*   Use explicit IPv4 loopback (127.0.0.1) instead of localhost for shard advertise addresses
*   Separate bind and advertise addresses for shard-to-LoadBalancer communication
*   Resolve IPv4/IPv6 mismatch in multi-core shard binding
*   Update multi-core config to 18K total connections (6K per shard)
*   Configure multi-core for 18K total connections (6K per shard)

## 🚀 Performance

*   Optimize broadcast path for 20-30% capacity increase

## 📦 Chore

*   Temporarily disable local deployment tasks and refactor mode handling

## 🔄 Revert

*   Revert "temp: Reduce METRICS_INTERVAL to 1s for faster CPU admission control"

## miscellaneous

*   temp: Reduce METRICS_INTERVAL to 1s for faster CPU admission control
