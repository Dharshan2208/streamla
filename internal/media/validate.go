package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrLacksFastStart indicates the MP4 is valid but 'mdat' appears before 'moov'.
var ErrLacksFastStart = errors.New("MP4 lacks FastStart: 'mdat' appears before 'moov' (seeking will fail in browser)")

// ValidateMP4 verifies that the file is an MP4 and has FastStart (moov before mdat).
func ValidateMP4(file *os.File) error {
	var header [8]byte

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek file: %w", err)
	}

	// 1. Check if first box is 'ftyp'
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return fmt.Errorf("file too small")
	}
	if string(header[4:8]) != "ftyp" {
		return fmt.Errorf("invalid format: file must be an MP4 (missing ftyp)")
	}

	// 2. Scan top-level boxes to check moov vs mdat order
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek start: %w", err)
	}

	var seenMdat bool
	for {
		if _, err := io.ReadFull(file, header[:]); err != nil {
			break // Reached EOF
		}

		size := binary.BigEndian.Uint32(header[0:4])
		boxType := string(header[4:8])

		if boxType == "moov" {
			if seenMdat {
				return ErrLacksFastStart
			}
			return nil // Valid MP4 with FastStart!
		}
		if boxType == "mdat" {
			seenMdat = true
		}

		if size == 0 {
			// Box extends to EOF
			break
		}
		if size == 1 {
			// 64-bit largesize
			var largeHeader [8]byte
			if _, err := io.ReadFull(file, largeHeader[:]); err != nil {
				break
			}
			largeSize := binary.BigEndian.Uint64(largeHeader[:])
			if largeSize < 16 {
				break
			}
			if _, err := file.Seek(int64(largeSize-16), io.SeekCurrent); err != nil {
				break
			}
			continue
		}
		if size < 8 {
			break
		}
		// Skip to the next box
		if _, err := file.Seek(int64(size-8), io.SeekCurrent); err != nil {
			break
		}
	}

	if seenMdat {
		return ErrLacksFastStart
	}

	return fmt.Errorf("invalid MP4: missing 'moov' metadata atom")
}
