package session

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

const readinessPollInterval = 100 * time.Millisecond

// WaitForPort waits until a TCP connection can be established to bind:port.
func WaitForPort(bind string, port int, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("invalid timeout %s", timeout)
	}

	address := net.JoinHostPort(bind, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)

	for {
		conn, err := net.DialTimeout("tcp", address, readinessPollInterval)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s after %s", address, timeout)
		}

		time.Sleep(readinessPollInterval)
	}
}
