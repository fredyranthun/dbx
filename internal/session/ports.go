package session

import (
	"fmt"
	"net"
)

// FindFreePort returns an available TCP port bound to bind within [min, max].
func FindFreePort(bind string, min int, max int) (int, error) {
	if min <= 0 || max <= 0 {
		return 0, fmt.Errorf("invalid port range %d-%d", min, max)
	}
	if min > max {
		return 0, fmt.Errorf("invalid port range %d-%d", min, max)
	}

	for port := min; port <= max; port++ {
		if err := ValidatePortAvailable(bind, port); err == nil {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no free port available on %s in range %d-%d", bind, min, max)
}

// ValidatePortAvailable verifies whether a TCP port can be bound on bind.
func ValidatePortAvailable(bind string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(bind, fmt.Sprintf("%d", port)))
	if err != nil {
		return fmt.Errorf("port %d not available on %s: %w", port, bind, err)
	}
	defer listener.Close()

	return nil
}
