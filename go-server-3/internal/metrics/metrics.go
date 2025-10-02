package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps Prometheus collectors used by the Go v3 server.
type Registry struct {
	Connections gaugeVec
	Messages    counterVec
}

type gaugeVec struct {
	ActiveConnections prometheus.Gauge
}

type counterVec struct {
	MessagesPublished prometheus.Counter
	MessagesDelivered prometheus.Counter
	AcceptErrors      prometheus.Counter
	BroadcastDropped  prometheus.Counter
}

// NewRegistry creates Prometheus metrics collectors.
func NewRegistry() *Registry {
	return &Registry{
		Connections: gaugeVec{
			ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "odin_ws_connections_active",
				Help: "Number of active WebSocket connections handled by go-server-3",
			}),
		},
		Messages: counterVec{
			MessagesPublished: promauto.NewCounter(prometheus.CounterOpts{
				Name: "odin_ws_messages_published_total",
				Help: "Total number of messages published to clients",
			}),
			MessagesDelivered: promauto.NewCounter(prometheus.CounterOpts{
				Name: "odin_ws_messages_delivered_total",
				Help: "Total number of messages delivered successfully",
			}),
			AcceptErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "odin_ws_accept_errors_total",
				Help: "Total number of WebSocket accept/handshake errors",
			}),
			BroadcastDropped: promauto.NewCounter(prometheus.CounterOpts{
				Name: "odin_ws_messages_dropped_total",
				Help: "Total number of broadcast messages dropped due to back pressure",
			}),
		},
	}
}

// Handler returns an HTTP handler exposing Prometheus metrics.
func (r *Registry) Handler() http.Handler {
	return promhttp.Handler()
}
