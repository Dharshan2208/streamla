package media

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// generateAtomFixture creates a small synthetic MP4 box structure
func generateAtomFixture(t *testing.T, boxes []struct {
	typ  string
	size uint64
}) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "fixture-*.mp4")
	if err != nil {
		t.Fatalf("create temp fixture: %v", err)
	}

	for _, b := range boxes {
		if b.size > 0xFFFFFFFF || b.size == 1 {
			// Write 64-bit largesize header: size=1, typ, then 64-bit size
			var h [16]byte
			binary.BigEndian.PutUint32(h[0:4], 1)
			copy(h[4:8], []byte(b.typ))
			binary.BigEndian.PutUint64(h[8:16], b.size)
			if _, err := f.Write(h[:]); err != nil {
				t.Fatalf("write largesize box header: %v", err)
			}
			if b.size > 16 {
				payload := make([]byte, b.size-16)
				if _, err := f.Write(payload); err != nil {
					t.Fatalf("write box payload: %v", err)
				}
			}
		} else {
			// Write 32-bit box header
			var h [8]byte
			binary.BigEndian.PutUint32(h[0:4], uint32(b.size))
			copy(h[4:8], []byte(b.typ))
			if _, err := f.Write(h[:]); err != nil {
				t.Fatalf("write box header: %v", err)
			}
			if b.size > 8 {
				payload := make([]byte, b.size-8)
				if _, err := f.Write(payload); err != nil {
					t.Fatalf("write box payload: %v", err)
				}
			}
		}
	}

	return f
}

func TestAtomOrderDetection_MoovBeforeMdat(t *testing.T) {
	// ftyp (16), moov (24), mdat (32) -> Valid FastStart
	f := generateAtomFixture(t, []struct {
		typ  string
		size uint64
	}{
		{"ftyp", 16},
		{"moov", 24},
		{"mdat", 32},
	})
	defer os.Remove(f.Name())
	defer f.Close()

	if err := ValidateMP4(f); err != nil {
		t.Fatalf("expected valid FastStart MP4, got: %v", err)
	}
}

func TestAtomOrderDetection_MdatBeforeMoov(t *testing.T) {
	// ftyp (16), mdat (32), moov (24) -> Lacks FastStart
	f := generateAtomFixture(t, []struct {
		typ  string
		size uint64
	}{
		{"ftyp", 16},
		{"mdat", 32},
		{"moov", 24},
	})
	defer os.Remove(f.Name())
	defer f.Close()

	err := ValidateMP4(f)
	if !errors.Is(err, ErrLacksFastStart) {
		t.Fatalf("expected ErrLacksFastStart, got: %v", err)
	}
}

func TestAtomOrderDetection_64BitLargeSize(t *testing.T) {
	// Tests 64-bit box size parsing (size == 1):
	// ftyp (16), mdat (largesize: 40 bytes), moov (24) -> Lacks FastStart
	f := generateAtomFixture(t, []struct {
		typ  string
		size uint64
	}{
		{"ftyp", 16},
		{"mdat", 40}, // parsed as largesize because size=1 is signaled
	})
	defer os.Remove(f.Name())
	defer f.Close()

	// If moov is never seen after mdat, should still detect seenMdat
	err := ValidateMP4(f)
	if !errors.Is(err, ErrLacksFastStart) {
		t.Fatalf("expected ErrLacksFastStart when mdat seen, got: %v", err)
	}
}

func TestEnsureFastStart_NoRemuxFlag(t *testing.T) {
	f := generateAtomFixture(t, []struct {
		typ  string
		size uint64
	}{
		{"ftyp", 16},
		{"mdat", 32},
		{"moov", 24},
	})
	defer os.Remove(f.Name())
	defer f.Close()

	_, _, err := EnsureFastStart(f.Name(), true)
	if !errors.Is(err, ErrLacksFastStart) {
		t.Fatalf("expected ErrLacksFastStart with noRemux=true, got: %v", err)
	}
}

func TestEnsureFastStart_AlreadyValid(t *testing.T) {
	// Test video.mp4 in repo
	repoVideo := "../../video.mp4"
	if _, err := os.Stat(repoVideo); os.IsNotExist(err) {
		t.Skip("video.mp4 not found, skipping already valid test")
	}

	absPath, err := filepath.Abs(repoVideo)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	effectivePath, wasRemuxed, err := EnsureFastStart(absPath, false)
	if err != nil {
		t.Fatalf("unexpected error on valid file: %v", err)
	}
	if wasRemuxed {
		t.Fatal("expected wasRemuxed=false for already valid FastStart MP4")
	}
	if effectivePath != absPath {
		t.Fatalf("expected effectivePath == absPath, got %s vs %s", effectivePath, absPath)
	}
}

func TestEnsureFastStart_FFmpegNotFoundMessage(t *testing.T) {
	// Temporarily override PATH to empty to verify clear error message
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	f := generateAtomFixture(t, []struct {
		typ  string
		size uint64
	}{
		{"ftyp", 16},
		{"mdat", 32},
		{"moov", 24},
	})
	defer os.Remove(f.Name())
	defer f.Close()

	_, _, err := EnsureFastStart(f.Name(), false)
	if err == nil {
		t.Fatal("expected error when ffmpeg not on PATH, got nil")
	}
	if !errors.Is(err, exec.ErrNotFound) && !contains(err.Error(), "ffmpeg not found on PATH") {
		t.Fatalf("expected clear ffmpeg missing message, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (filepath.Base(s) == substr || (len(substr) > 0 && hasSubstr(s, substr))))
}

func hasSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
