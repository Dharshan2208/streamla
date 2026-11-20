package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net/http"
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

func TestCheckCodecCompatibility_HEVC(t *testing.T) {
	probe := &MediaProbeResult{
		IsHEVC: true,
		Video: &StreamInfo{
			Index: 0,
			Type:  "video",
			Codec: "hevc",
		},
	}
	err := CheckCodecCompatibility(probe, "/path/to/movie.mp4")
	if err == nil {
		t.Fatal("expected error for HEVC video, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "unsupported video codec") {
		t.Errorf("expected 'unsupported video codec' in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "ffmpeg -i \"/path/to/movie.mp4\" -c:v libx264 -preset fast -crf 22 -c:a copy \"/path/to/movie_h264.mp4\"") {
		t.Errorf("expected exact ffmpeg transcode command in error, got: %s", errMsg)
	}
}

func TestCheckCodecCompatibility_H264(t *testing.T) {
	probe := &MediaProbeResult{
		IsHEVC: false,
		Video: &StreamInfo{
			Index: 0,
			Type:  "video",
			Codec: "h264",
		},
	}
	err := CheckCodecCompatibility(probe, "/path/to/movie.mp4")
	if err != nil {
		t.Fatalf("expected nil error for H.264 video, got: %v", err)
	}
}

func TestStreamHandler_ByteRangeRequests(t *testing.T) {
	// Create dummy test file with 100 bytes
	tmp, err := os.CreateTemp("", "range-test-*.mp4")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmp.Name())

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	if _, err := tmp.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
	_ = tmp.Close()

	state := NewMovieState()
	state.mu.Lock()
	state.filePath = tmp.Name()
	state.originalName = "test.mp4"
	state.probe = &MediaProbeResult{IsHEVC: false}
	state.mu.Unlock()

	handler := StreamHandler(state)

	// Test 1: Full request without Range
	{
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got: %d", rec.Code)
		}
		if rec.Header().Get("Accept-Ranges") != "bytes" {
			t.Errorf("expected Accept-Ranges: bytes, got: %s", rec.Header().Get("Accept-Ranges"))
		}
		if rec.Body.Len() != 100 {
			t.Errorf("expected 100 bytes, got: %d", rec.Body.Len())
		}
	}

	// Test 2: Range request for first 10 bytes (0-9)
	{
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		req.Header.Set("Range", "bytes=0-9")
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got: %d", rec.Code)
		}
		if rec.Header().Get("Content-Range") != "bytes 0-9/100" {
			t.Errorf("expected Content-Range 'bytes 0-9/100', got: %s", rec.Header().Get("Content-Range"))
		}
		if rec.Body.Len() != 10 {
			t.Errorf("expected 10 bytes, got: %d", rec.Body.Len())
		}
		if !bytes.Equal(rec.Body.Bytes(), data[0:10]) {
			t.Errorf("byte mismatch for range 0-9")
		}
	}

	// Test 3: Range request for mid-file (50-69)
	{
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		req.Header.Set("Range", "bytes=50-69")
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got: %d", rec.Code)
		}
		if rec.Header().Get("Content-Range") != "bytes 50-69/100" {
			t.Errorf("expected Content-Range 'bytes 50-69/100', got: %s", rec.Header().Get("Content-Range"))
		}
		if rec.Body.Len() != 20 {
			t.Errorf("expected 20 bytes, got: %d", rec.Body.Len())
		}
		if !bytes.Equal(rec.Body.Bytes(), data[50:70]) {
			t.Errorf("byte mismatch for range 50-69")
		}
	}
}

func TestStreamHandler_HEVC_Rejected(t *testing.T) {
	state := NewMovieState()
	state.mu.Lock()
	state.filePath = "/dummy/hevc.mp4"
	state.originalName = "hevc.mp4"
	state.probe = &MediaProbeResult{
		IsHEVC: true,
		Video: &StreamInfo{
			Codec: "hevc",
		},
	}
	state.mu.Unlock()

	handler := StreamHandler(state)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 Unsupported Media Type for HEVC stream, got: %d", rec.Code)
	}
}
