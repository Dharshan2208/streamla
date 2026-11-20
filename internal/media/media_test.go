package media

import (
	"encoding/binary"
	"os"
	"testing"
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

	if err := ValidateMP4(f); err == nil {
		t.Fatal("expected error for mdat before moov, got nil")
	}
}

func TestMovieState_SetGetCleanup(t *testing.T) {
	ms := NewMovieState()

	_, _, ok := ms.Get()
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

	path, name, ok := ms.Get()
	if !ok || path != tmpName || name != "dummy.mp4" {
		t.Fatalf("unexpected Get result: path=%s name=%s ok=%v", path, name, ok)
	}

	// Verify cleanup deletes the file if isTemp=true
	ms.Cleanup()

	_, _, ok = ms.Get()
	if ok {
		t.Fatal("expected empty movie state after cleanup")
	}

	if _, err := os.Stat(tmpName); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be deleted, got err: %v", err)
	}
}

func TestMovieState_NonTempFileNotDeleted(t *testing.T) {
	ms := NewMovieState()

	tmp, err := os.CreateTemp("", "permanent-*.mp4")
	if err != nil {
		t.Fatalf("create temp failed: %v", err)
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	// isTemp is false (e.g. host passed CLI file path)
	ms.Set(tmpName, "permanent.mp4", false)

	ms.Cleanup()

	// File should still exist on disk!
	if _, err := os.Stat(tmpName); err != nil {
		t.Fatalf("expected non-temp file to remain on disk, got err: %v", err)
	}
}
