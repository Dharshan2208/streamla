package room

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewRoom(t *testing.T) {
	room := NewRoom()
	if room.ID == "" {
		t.Fatal("expected non-empty room ID")
	}
	if room.HostToken == "" {
		t.Fatal("expected non-empty host token")
	}
	if room.PlayBack == nil {
		t.Fatal("expected non-nil PlayBack")
	}
	if room.PlayBack.Playing() {
		t.Fatal("expected initial state to be paused")
	}
	if pos := room.PlayBack.CurrentPosition(); pos != 0 {
		t.Fatalf("expected initial position 0, got %f", pos)
	}
	if count := room.ClientCount(); count != 0 {
		t.Fatalf("expected client count 0, got %d", count)
	}
}

func TestPlaybackState_UpdateAndPosition(t *testing.T) {
	ps := NewPlaybackState()

	if ps.Playing() {
		t.Error("expected playing to be false")
	}
	if pos := ps.CurrentPosition(); pos != 0 {
		t.Errorf("expected position 0, got %f", pos)
	}

	ps.Update(20.0, true)
	if !ps.Playing() {
		t.Error("expected playing to be true")
	}

	time.Sleep(50 * time.Millisecond)
	elapsed := ps.CurrentPosition()
	if elapsed <= 20.0 {
		t.Errorf("expected position to advance past 20.0, got %f", elapsed)
	}

	ps.Pause(50.0)
	if ps.Playing() {
		t.Error("expected playing to be false")
	}
	if pos := ps.CurrentPosition(); pos != 50.0 {
		t.Errorf("expected position 50.0, got %f", pos)
	}
}

func TestPlaybackState_SchedulePlay(t *testing.T) {
	ps := NewPlaybackState()

	// Schedule to resume 100ms in the future at position 15.0
	future := time.Now().Add(100 * time.Millisecond)
	ps.SchedulePlay(15.0, future)

	if !ps.Playing() {
		t.Error("expected playing to be true")
	}

	// While still in the future, position must remain exactly 15.0
	pos := ps.CurrentPosition()
	if pos != 15.0 {
		t.Errorf("expected position to remain 15.0 before scheduled time, got %f", pos)
	}

	// Sleep until past scheduled resume time
	time.Sleep(150 * time.Millisecond)
	posAfter := ps.CurrentPosition()
	if posAfter <= 15.0 {
		t.Errorf("expected position to advance past 15.0 after resume, got %f", posAfter)
	}
}

func TestPlaybackState_ConcurrentAccess(t *testing.T) {
	ps := NewPlaybackState()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Writers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					if id%2 == 0 {
						ps.SchedulePlay(float64(id*10), time.Now().Add(50*time.Millisecond))
					} else {
						ps.Pause(float64(id * 10))
					}
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = ps.CurrentPosition()
					_ = ps.Playing()
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

func TestRoom_StartHeartbeat(t *testing.T) {
	r := NewRoom()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go r.StartHeartbeat(ctx, 20*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
}
