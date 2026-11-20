package discovery

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mdp/qrterminal/v3"
	"rsc.io/qr"
)

// PrintBanner outputs the LAN access URL and ASCII QR code to the terminal.
func PrintBanner(port, roomID, hostToken string) {
	ip := GetLocalIP()
	lanURL := fmt.Sprintf("http://%s:%s", ip, port)
	hostURL := fmt.Sprintf("http://localhost:%s/?token=%s", port, hostToken)

	fmt.Println("==================================================")
	fmt.Println("       STREAMLA — LAN Watch Party Server          ")
	fmt.Println("==================================================")
	fmt.Printf("Room ID:        %s\n", roomID)
	fmt.Printf("Host URL:       %s\n", hostURL)
	fmt.Printf("LAN URL:        %s\n", lanURL)
	fmt.Println("Scan QR to join from phone or second laptop:")
	fmt.Println()

	config := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	}
	qrterminal.GenerateWithConfig(lanURL, config)
	fmt.Println("==================================================")
}

// QRHandler returns an HTTP handler serving a PNG QR code for the LAN access URL.
func QRHandler(port string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := GetLocalIP()
		url := fmt.Sprintf("http://%s:%s", ip, port)

		code, err := qr.Encode(url, qr.M)
		if err != nil {
			http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write(code.PNG())
	}
}
