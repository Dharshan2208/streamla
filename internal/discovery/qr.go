package discovery

import (
	"fmt"
	"os"

	"github.com/mdp/qrterminal/v3"
)

// PrintBanner outputs the LAN access URL and ASCII QR code to the terminal.
func PrintBanner(port, roomID string) {
	ip := GetLocalIP()
	url := fmt.Sprintf("http://%s:%s", ip, port)

	fmt.Println("==================================================")
	fmt.Println("       STREAMLA — LAN Watch Party Server          ")
	fmt.Println("==================================================")
	fmt.Printf("Room ID:        %s\n", roomID)
	fmt.Printf("LAN URL:        %s\n", url)
	fmt.Println("Scan to join from phone or laptop:")
	fmt.Println()

	config := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	}
	qrterminal.GenerateWithConfig(url, config)
	fmt.Println("==================================================")
}
