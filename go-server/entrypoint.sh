#!/bin/sh

# Set system limits for high connection loads
echo "Setting system limits for high-performance WebSocket server..."

# Aggressive file descriptor limits with available memory
ulimit -n 200000

# Increase max user processes
ulimit -u 32768

# Set socket buffer sizes (if running as root)
if [ "$(id -u)" = "0" ]; then
    echo "Setting kernel parameters..."

    # Increase socket backlog
    echo 32768 > /proc/sys/net/core/somaxconn

    # Increase TCP backlog
    echo 32768 > /proc/sys/net/ipv4/tcp_max_syn_backlog

    # Enable TCP fast open
    echo 3 > /proc/sys/net/ipv4/tcp_fastopen

    # Reduce TIME_WAIT sockets
    echo 1 > /proc/sys/net/ipv4/tcp_tw_reuse
    echo 1 > /proc/sys/net/ipv4/tcp_tw_recycle

    # Increase local port range
    echo "1024 65535" > /proc/sys/net/ipv4/ip_local_port_range

    # Increase max connections tracking
    echo 524288 > /proc/sys/net/netfilter/nf_conntrack_max 2>/dev/null || true
else
    echo "Running as non-root, skipping kernel parameter changes"
fi

# Show current limits
echo "Current limits:"
ulimit -a

# Start the application
exec ./odin-ws-server "$@"