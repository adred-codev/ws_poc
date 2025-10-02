package main

import (
	"net"
	"sync"
)

// Client represents a WebSocket client connection
type Client struct {
	id     int64
	conn   net.Conn
	server *Server
	send   chan []byte
	mu     sync.RWMutex
}

// ConnectionPool manages a pool of reusable client objects
type ConnectionPool struct {
	pool     sync.Pool
	maxSize  int
	current  int64
}

func NewConnectionPool(maxSize int) *ConnectionPool {
	return &ConnectionPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return &Client{
					send: make(chan []byte, 256),
				}
			},
		},
	}
}

func (p *ConnectionPool) Get() *Client {
	v := p.pool.Get()
	if client, ok := v.(*Client); ok {
		// Reset client state
		select {
		case <-client.send:
			// Drain any pending messages
		default:
		}
		return client
	}
	return nil
}

func (p *ConnectionPool) Put(c *Client) {
	if c == nil {
		return
	}
	
	// Reset connection
	c.conn = nil
	c.server = nil
	c.id = 0
	
	p.pool.Put(c)
}
