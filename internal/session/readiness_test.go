package session

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func TestWaitForPort(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	if err := reserved.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			defer listener.Close()
			<-done
		}
	}()
	defer close(done)

	if err := WaitForPort("127.0.0.1", port, 2*time.Second); err != nil {
		t.Fatalf("WaitForPort returned error: %v", err)
	}
}

func TestWaitForPortTimeout(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	if err := reserved.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	err = WaitForPort("127.0.0.1", port, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
