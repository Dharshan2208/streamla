package media

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

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
				return fmt.Errorf("MP4 lacks FastStart: 'mdat' appears before 'moov' (seeking will fail in browser)")
			}
			return nil // Valid MP4 with FastStart!
		}
		if boxType == "mdat" {
			seenMdat = true
		}

		if size < 8 {
			break
		}
		// Skip to the next box
		if _, err := file.Seek(int64(size-8), io.SeekCurrent); err != nil {
			break
		}
	}

	return fmt.Errorf("invalid MP4: missing 'moov' metadata atom")
}
