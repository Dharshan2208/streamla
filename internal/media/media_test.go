package media

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func createTestMP4(t *testing.T, moovBeforeMdat bool, validFtyp bool) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.mp4")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	ftypName := "ftyp"
	if !validFtyp {
		ftypName = "xxxx"
	}
	var ftypHeader [8]byte
	binary.BigEndian.PutUint32(ftypHeader[0:4], 8)
	copy(ftypHeader[4:8], []byte(ftypName))
	if _, err := f.Write(ftypHeader[:]); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	writeBox := func(boxType string, size uint32) {
		var h [8]byte
		binary.BigEndian.PutUint32(h[0:4], size)
		copy(h[4:8], []byte(boxType))
		if _, err := f.Write(h[:]); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if size > 8 {
			pad := make([]byte, size-8)
			if _, err := f.Write(pad); err != nil {
				t.Fatalf("write failed: %v", err)
			}
		}
	}

	if moovBeforeMdat {
		writeBox("moov", 16)
		writeBox("mdat", 16)
	} else {
		writeBox("mdat", 16)
		writeBox("moov", 16)
	}

	return f
}

func TestValidateMP4_Valid(t *testing.T) {
	f := createTestMP4(t, true, true)
	defer os.Remove(f.Name())
	defer f.Close()

	if err := ValidateMP4(f); err != nil {
		t.Fatalf("expected valid MP4, got error: %v", err)
	}
}

func TestValidateMP4_InvalidFtyp(t *testing.T) {
	f := createTestMP4(t, true, false)
	defer os.Remove(f.Name())
	defer f.Close()

	if err := ValidateMP4(f); err == nil {
		t.Fatal("expected error for invalid ftyp, got nil")
	}
}

func TestValidateMP4_MdatBeforeMoov(t *testing.T) {
	f := createTestMP4(t, false, true)
	defer os.Remove(f.Name())
	defer f.Close()

	err := ValidateMP4(f)
	if !errors.Is(err, ErrLacksFastStart) {
		t.Fatalf("expected ErrLacksFastStart, got: %v", err)
	}
}

func TestMovieState_SetGetCleanup(t *testing.T) {
	ms := NewMovieState()

	_, _, _, ok := ms.Get()
	if ok {
		t.Fatal("expected empty movie state initially")
	}

	tmp, err := os.CreateTemp("", "dummy-*.mp4")
	if err != nil {
		t.Fatalf("create temp failed: %v", err)
	}
	tmpName := tmp.Name()
	tmp.Close()

	ms.Set(tmpName, "dummy.mp4", true)

	path, name, _, ok := ms.Get()
	if !ok || path != tmpName || name != "dummy.mp4" {
		t.Fatalf("unexpected Get result: path=%s name=%s ok=%v", path, name, ok)
	}

	ms.Cleanup()

	_, _, _, ok = ms.Get()
	if ok {
		t.Fatal("expected empty movie state after cleanup")
	}

	if _, err := os.Stat(tmpName); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be deleted, got err: %v", err)
	}
}

func TestConvertExternalSRT(t *testing.T) {
	tmpSRT, err := os.CreateTemp("", "test-*.srt")
	if err != nil {
		t.Fatalf("create temp srt failed: %v", err)
	}
	defer os.Remove(tmpSRT.Name())

	content := "1\n00:00:01,000 --> 00:00:03,000\nHello Watch Party\n"
	if _, err := tmpSRT.WriteString(content); err != nil {
		t.Fatalf("write srt failed: %v", err)
	}
	tmpSRT.Close()

	rec := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := ConvertExternalSRT(ctx, tmpSRT.Name(), rec); err != nil {
		t.Fatalf("convert external srt failed: %v", err)
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, "WEBVTT") {
		t.Fatalf("expected WebVTT output, got: %s", body)
	}
	if !strings.Contains(body, "Hello Watch Party") {
		t.Fatalf("expected subtitle text in output, got: %s", body)
	}
}

func TestProbeMedia_RealVideo(t *testing.T) {
	// Probe video.mp4 at repo root
	videoPath := "../../video.mp4"
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		t.Skip("video.mp4 not found, skipping real media probe test")
	}

	result, err := ProbeMedia(videoPath)
	if err != nil {
		t.Fatalf("probe real video failed: %v", err)
	}

	if result.Video == nil || result.Video.Codec != "h264" {
		t.Fatalf("expected h264 video, got: %+v", result.Video)
	}
	if len(result.Audio) == 0 || result.Audio[0].Codec != "aac" {
		t.Fatalf("expected aac audio, got: %+v", result.Audio)
	}
	if result.Duration <= 0 {
		t.Fatalf("expected positive duration, got: %f", result.Duration)
	}
}
