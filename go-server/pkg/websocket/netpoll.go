// +build linux

package websocket

import (
	"net"
	"os"
	"syscall"
)

// SetTCPOptions optimizes TCP socket for high connection loads
func SetTCPOptions(conn *net.TCPConn) error {
	// Get the file descriptor
	file, err := conn.File()
	if err != nil {
		return err
	}
	defer file.Close()

	fd := int(file.Fd())

	// TCP_NODELAY - disable Nagle's algorithm
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)

	// TCP_QUICKACK - send ACKs immediately
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 12, 1) // TCP_QUICKACK = 12

	// SO_KEEPALIVE - enable keepalive
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)

	// TCP keepalive parameters
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, 30)  // Start after 30s
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 10) // Interval 10s
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 3)    // 3 probes

	// SO_RCVBUF - receive buffer size (256KB)
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 262144)

	// SO_SNDBUF - send buffer size (256KB)
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 262144)

	// TCP_USER_TIMEOUT - total time for unacknowledged data
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 18, 30000) // 30 seconds

	return nil
}

// CreateOptimizedListener creates a listener with maximum performance settings
func CreateOptimizedListener(addr string) (net.Listener, error) {
	// Parse address
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Create socket manually for more control
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	// Set socket options BEFORE binding
	// SO_REUSEADDR - allow reuse of port
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)

	// SO_REUSEPORT - allow multiple listeners on same port (Linux 3.9+)
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, 15, 1) // SO_REUSEPORT = 15

	// TCP_FASTOPEN - allow fast open (Linux 3.6+)
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 23, 5) // TCP_FASTOPEN = 23, queue = 5

	// TCP_DEFER_ACCEPT - defer accept until data arrives
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 9, 1) // TCP_DEFER_ACCEPT = 9

	// Bind to address
	addr4 := &syscall.SockaddrInet4{Port: tcpAddr.Port}
	copy(addr4.Addr[:], tcpAddr.IP.To4())
	if err := syscall.Bind(fd, addr4); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	// Listen with huge backlog
	if err := syscall.Listen(fd, 32768); err != nil { // Max backlog
		syscall.Close(fd)
		return nil, err
	}

	// Convert to net.Listener
	file := os.NewFile(uintptr(fd), "")
	listener, err := net.FileListener(file)
	file.Close()

	return listener, err
}

// EnableEpoll uses epoll for efficient connection handling
type EpollServer struct {
	epfd      int
	events    []syscall.EpollEvent
	listeners map[int]net.Listener
}

// NewEpollServer creates an epoll-based server
func NewEpollServer() (*EpollServer, error) {
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}

	return &EpollServer{
		epfd:      epfd,
		events:    make([]syscall.EpollEvent, 10000), // Handle 10k events at once
		listeners: make(map[int]net.Listener),
	}, nil
}

// AddListener adds a listener to epoll
func (e *EpollServer) AddListener(l net.Listener) error {
	// Get file descriptor from listener
	switch lt := l.(type) {
	case *net.TCPListener:
		file, err := lt.File()
		if err != nil {
			return err
		}
		defer file.Close()

		fd := int(file.Fd())
		event := syscall.EpollEvent{
			Events: syscall.EPOLLIN | syscall.EPOLLET, // Edge-triggered
			Fd:     int32(fd),
		}

		if err := syscall.EpollCtl(e.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
			return err
		}

		e.listeners[fd] = l
	}
	return nil
}

// Wait waits for events
func (e *EpollServer) Wait() ([]int, error) {
	n, err := syscall.EpollWait(e.epfd, e.events, -1)
	if err != nil {
		return nil, err
	}

	ready := make([]int, 0, n)
	for i := 0; i < n; i++ {
		ready = append(ready, int(e.events[i].Fd))
	}

	return ready, nil
}