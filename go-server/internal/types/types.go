package types

import (
	"time"

	"github.com/gorilla/websocket"
)

// Message types
type MessageType string

const (
	MessageTypePriceUpdate           MessageType = "priceUpdate"
	MessageTypeTradeExecuted         MessageType = "tradeExecuted"
	MessageTypeVolumeUpdate          MessageType = "volumeUpdate"
	MessageTypeBatchUpdate           MessageType = "batchUpdate"
	MessageTypeMarketStats           MessageType = "marketStats"
	MessageTypeHeartbeat             MessageType = "heartbeat"
	MessageTypeConnectionEstablished MessageType = "connectionEstablished"
	MessageTypePing                  MessageType = "ping"
	MessageTypePong                  MessageType = "pong"
)

// Base message structure
type BaseMessage struct {
	Type      MessageType `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Nonce     string      `json:"nonce"`
}

// Price update message
type PriceUpdateMessage struct {
	BaseMessage
	TokenID     string  `json:"tokenId"`
	Price       float64 `json:"price"`
	Change24h   float64 `json:"change24h"`
	Volume24h   float64 `json:"volume24h"`
	MarketCap   float64 `json:"marketCap"`
	Source      string  `json:"source"`
	Exchange    string  `json:"exchange"`
	LastUpdated int64   `json:"lastUpdated"`
}

// Trade executed message
type TradeExecutedMessage struct {
	BaseMessage
	TokenID  string  `json:"tokenId"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Side     string  `json:"side"`
	TradeID  string  `json:"tradeId"`
	Exchange string  `json:"exchange"`
	BuyerID  string  `json:"buyerId,omitempty"`
	SellerID string  `json:"sellerId,omitempty"`
	Fees     float64 `json:"fees"`
}

// Volume update message
type VolumeUpdateMessage struct {
	BaseMessage
	TokenID      string  `json:"tokenId"`
	Volume24h    float64 `json:"volume24h"`
	VolumeChange float64 `json:"volumeChange"`
	TradeCount   int     `json:"tradeCount"`
}

// Batch update message
type BatchUpdateMessage struct {
	BaseMessage
	Updates []PriceUpdateMessage `json:"updates"`
	Count   int                  `json:"count"`
}

// Market stats message
type MarketStatsMessage struct {
	BaseMessage
	TotalMarketCap float64        `json:"totalMarketCap"`
	TotalVolume24h float64        `json:"totalVolume24h"`
	ActiveTokens   int            `json:"activeTokens"`
	TopGainers     []TokenSummary `json:"topGainers"`
	TopLosers      []TokenSummary `json:"topLosers"`
	MostTraded     []TokenSummary `json:"mostTraded"`
}

// Token summary for market stats
type TokenSummary struct {
	TokenID   string  `json:"tokenId"`
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change24h"`
	Volume24h float64 `json:"volume24h"`
}

// Heartbeat message
type HeartbeatMessage struct {
	BaseMessage
	ServerTime int64 `json:"serverTime"`
	Uptime     int64 `json:"uptime"`
}

// Connection established message
type ConnectionEstablishedMessage struct {
	BaseMessage
	ClientID     string   `json:"clientId"`
	ServerTime   int64    `json:"serverTime"`
	Capabilities []string `json:"capabilities"`
}

// Ping/Pong messages
type PingMessage struct {
	BaseMessage
	ClientID string `json:"clientId,omitempty"`
}

type PongMessage struct {
	BaseMessage
	OriginalTimestamp int64  `json:"originalTimestamp"`
	ClientID          string `json:"clientId,omitempty"`
}

// Client connection info
type ClientInfo struct {
	ID              string
	Conn            *websocket.Conn
	ConnectedAt     time.Time
	SeenNonces      map[string]bool
	HeartbeatTicker *time.Ticker
	MessageCount    int64
	LastMessageTime time.Time
	AuthToken       string
	UserID          string
	Subscriptions   map[string]bool
	SendChan        chan []byte
	CloseChan       chan struct{}
}

// Server metrics
type ServerMetrics struct {
	MessagesPublished int64
	MessagesDelivered int64
	ConnectionCount   int64
	DuplicatesDropped int64
	AverageLatency    float64
	PeakLatency       float64
	StartTime         time.Time
	ErrorCount        int64
	LastErrorTime     time.Time
	LastError         string
}

// Configuration
type Config struct {
	Server struct {
		Host           string `json:"host"`
		Port           int    `json:"port"`
		ReadTimeout    int    `json:"readTimeout"`
		WriteTimeout   int    `json:"writeTimeout"`
		MaxMessageSize int64  `json:"maxMessageSize"`
	} `json:"server"`

	WebSocket struct {
		CheckOrigin       bool `json:"checkOrigin"`
		EnableCompression bool `json:"enableCompression"`
		ReadBufferSize    int  `json:"readBufferSize"`
		WriteBufferSize   int  `json:"writeBufferSize"`
		HandshakeTimeout  int  `json:"handshakeTimeout"`
	} `json:"websocket"`

	NATS struct {
		URL             string `json:"url"`
		MaxReconnects   int    `json:"maxReconnects"`
		ReconnectWait   int    `json:"reconnectWait"`
		ReconnectJitter int    `json:"reconnectJitter"`
		MaxPingsOut     int    `json:"maxPingsOut"`
		PingInterval    int    `json:"pingInterval"`
	} `json:"nats"`

	Auth struct {
		JWTSecret       string `json:"jwtSecret"`
		TokenExpiration int    `json:"tokenExpiration"`
		RequireAuth     bool   `json:"requireAuth"`
	} `json:"auth"`

	Metrics struct {
		EnablePrometheus bool   `json:"enablePrometheus"`
		MetricsPath      string `json:"metricsPath"`
		UpdateInterval   int    `json:"updateInterval"`
	} `json:"metrics"`
}
