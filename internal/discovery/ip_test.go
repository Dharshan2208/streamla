package discovery

import (
	"net"
	"testing"
)

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	if ip == "" {
		t.Fatal("expected non-empty IP string")
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("expected valid IP address, got %q", ip)
	}

	if parsed.To4() == nil {
		t.Fatalf("expected IPv4 address, got %q", ip)
	}
}
