package main

import (
	"fmt"
	"net"
	"os"

	"github.com/mdp/qrterminal"
)

// get ip
func GetLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()

		localAddr := conn.LocalAddr().(*net.UDPAddr)

		return localAddr.IP.String()
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "localhost"
}

func PrintBanner(port string, roomID string) {
	ip := GetLocalIP()
	url := fmt.Sprintf("http://%s:%s", ip, port)

	fmt.Printf("Streamla is running on your LAN!\n")
	fmt.Printf("Shareable Link: %s\n", url)
	fmt.Println("Scan with your phone to join:")

	config := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	}
	qrterminal.GenerateWithConfig(url, config)
}
