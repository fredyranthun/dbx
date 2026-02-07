package session

import (
	"net"
	"testing"
)

func TestValidatePortAvailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if err := ValidatePortAvailable("127.0.0.1", port); err == nil {
		t.Fatalf("expected port %d to be unavailable", port)
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := FindFreePort("127.0.0.1", 5500, 5600)
	if err != nil {
		t.Fatalf("FindFreePort returned error: %v", err)
	}

	if port < 5500 || port > 5600 {
		t.Fatalf("FindFreePort returned port %d outside range", port)
	}
}

func TestFindFreePortReturnsErrorWhenRangeIsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if _, err := FindFreePort("127.0.0.1", port, port); err == nil {
		t.Fatalf("expected error when only port %d in range is occupied", port)
	}
}
