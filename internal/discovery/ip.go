package discovery

import (
	"net"
	"strings"
)

// GetLocalIP returns the primary non-loopback IPv4 address on the LAN.
// It first attempts a route-based UDP dial. If offline or no default gateway,
// it iterates local network interfaces, prioritizing physical Wi-Fi/Ethernet adapters
// and filtering out virtual interfaces (docker, bridge, veth, etc.).
func GetLocalIP() string {
	// 1. Try route lookup via UDP dial (does not send packets)
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			ip := localAddr.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}

	// 2. Fallback: inspect physical network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	var candidates []string

	for _, iface := range ifaces {
		// Ignore interfaces that are down or loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		name := strings.ToLower(iface.Name)
		// Skip virtual/container/VPN interfaces
		if strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "virbr") ||
			strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "tun") ||
			strings.HasPrefix(name, "tap") ||
			strings.HasPrefix(name, "wg") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}

			// Prioritize common Wi-Fi (wl*) and Ethernet (eth*, en*) interfaces
			if strings.HasPrefix(name, "wl") || strings.HasPrefix(name, "en") || strings.HasPrefix(name, "eth") {
				return ip4.String()
			}

			candidates = append(candidates, ip4.String())
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}

	return "127.0.0.1"
}
