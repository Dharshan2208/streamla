package room

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTwoDeviceSyncFlow(t *testing.T) {
	rm := NewRoom()
	server := httptest.NewServer(WSHandler(rm))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 1. Connect Host
	hostDialer := websocket.Dialer{}
	hostConn, _, err := hostDialer.Dial(wsURL+"?token="+rm.HostToken, nil)
	if err != nil {
		t.Fatalf("host connect failed: %v", err)
	}
	defer hostConn.Close()

	// Read initial SYNC message sent to host
	var hostSync Message
	if err := hostConn.ReadJSON(&hostSync); err != nil {
		t.Fatalf("host read initial sync failed: %v", err)
	}
	if hostSync.Type != "SYNC" {
		t.Fatalf("expected SYNC, got %s", hostSync.Type)
	}

	// 2. Connect Viewer
	viewerDialer := websocket.Dialer{}
	viewerConn, _, err := viewerDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("viewer connect failed: %v", err)
	}
	defer viewerConn.Close()

	// Read initial SYNC message sent to viewer
	var viewerSync Message
	if err := viewerConn.ReadJSON(&viewerSync); err != nil {
		t.Fatalf("viewer read initial sync failed: %v", err)
	}
	if viewerSync.Type != "SYNC" {
		t.Fatalf("expected SYNC, got %s", viewerSync.Type)
	}
	if viewerSync.Count != 2 {
		t.Fatalf("expected viewer count 2, got %d", viewerSync.Count)
	}

	// 3. NTP PING / PONG test
	t1 := float64(time.Now().UnixNano()) / 1e6
	if err := viewerConn.WriteJSON(Message{Type: "PING", T1: t1}); err != nil {
		t.Fatalf("viewer write PING failed: %v", err)
	}

	var pong Message
	// Drain any intermediate messages (like JOIN) until PONG
	for {
		if err := viewerConn.ReadJSON(&pong); err != nil {
			t.Fatalf("read PONG failed: %v", err)
		}
		if pong.Type == "PONG" {
			break
		}
	}
	if pong.T1 != t1 || pong.T2 == 0 || pong.T3 == 0 {
		t.Fatalf("invalid PONG timestamps: %+v", pong)
	}

	// 4. Host sends PLAY at position 10.0
	if err := hostConn.WriteJSON(Message{Type: "PLAY", Position: 10.0, Token: rm.HostToken}); err != nil {
		t.Fatalf("host write PLAY failed: %v", err)
	}

	// Both host and viewer must receive PREPARE_PLAY with scheduled resumeAt
	var hostPlay Message
	for {
		if err := hostConn.ReadJSON(&hostPlay); err != nil {
			t.Fatalf("host read PREPARE_PLAY failed: %v", err)
		}
		if hostPlay.Type == "PREPARE_PLAY" {
			break
		}
	}
	if hostPlay.Position != 10.0 || !hostPlay.IsPlaying || hostPlay.ResumeAt <= 0 {
		t.Fatalf("invalid host PREPARE_PLAY message: %+v", hostPlay)
	}

	var viewerPlay Message
	for {
		if err := viewerConn.ReadJSON(&viewerPlay); err != nil {
			t.Fatalf("viewer read PREPARE_PLAY failed: %v", err)
		}
		if viewerPlay.Type == "PREPARE_PLAY" {
			break
		}
	}
	if viewerPlay.Position != 10.0 || !viewerPlay.IsPlaying || viewerPlay.ResumeAt != hostPlay.ResumeAt {
		t.Fatalf("mismatched viewer PREPARE_PLAY message: %+v", viewerPlay)
	}

	// 5. Viewer attempts unauthorized playback control (Anti-Desync check)
	if err := viewerConn.WriteJSON(Message{Type: "PAUSE", Position: 0}); err != nil {
		t.Fatalf("viewer write PAUSE failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if !rm.PlayBack.Playing() {
		t.Fatal("viewer was able to desync room by pausing unauthorized!")
	}

	// 6. Host sends PAUSE at position 15.0
	if err := hostConn.WriteJSON(Message{Type: "PAUSE", Position: 15.0, Token: rm.HostToken}); err != nil {
		t.Fatalf("host write PAUSE failed: %v", err)
	}

	var viewerPause Message
	for {
		if err := viewerConn.ReadJSON(&viewerPause); err != nil {
			t.Fatalf("viewer read PAUSE failed: %v", err)
		}
		if viewerPause.Type == "PAUSE" {
			break
		}
	}
	if viewerPause.Position != 15.0 || viewerPause.IsPlaying {
		t.Fatalf("invalid viewer PAUSE message: %+v", viewerPause)
	}

	// 7. Late Joiner connects and receives paused position 15.0
	lateDialer := websocket.Dialer{}
	lateConn, _, err := lateDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("late viewer connect failed: %v", err)
	}
	defer lateConn.Close()

	var lateSync Message
	if err := lateConn.ReadJSON(&lateSync); err != nil {
		t.Fatalf("late viewer read SYNC failed: %v", err)
	}
	if lateSync.Type != "SYNC" || lateSync.Position != 15.0 || lateSync.IsPlaying {
		t.Fatalf("invalid late joiner state: %+v", lateSync)
	}
}

func TestHeartbeatDelivery(t *testing.T) {
	rm := NewRoom()
	server := httptest.NewServer(WSHandler(rm))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go rm.StartHeartbeat(ctx, 30*time.Millisecond)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Wait for HEARTBEAT
	foundHeartbeat := false
	for i := 0; i < 5; i++ {
		var msg Message
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		if msg.Type == "HEARTBEAT" {
			foundHeartbeat = true
			if msg.ServerTime == 0 {
				t.Fatalf("heartbeat missing ServerTime: %+v", msg)
			}
			break
		}
	}

	if !foundHeartbeat {
		t.Fatal("expected HEARTBEAT message not received")
	}
}
