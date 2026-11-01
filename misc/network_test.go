package misc

import (
	"fmt"
	"net"
	"testing"
)

func TestGetFreePort(t *testing.T) {
	port, err := GetFreePort()
	if err != nil {
		t.Fatalf("GetFreePort returned error: %s", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("GetFreePort = %d, want a valid TCP port", port)
	}

	// The port was released, so it should be possible to listen on it
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatalf("unable to listen on returned port %d: %s", port, err)
	}
	_ = l.Close()
}
