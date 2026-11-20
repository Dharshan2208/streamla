package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EnsureFastStart checks if an MP4 file has FastStart (moov before mdat).
// If it lacks FastStart and noRemux is false, it remuxes the file with ffmpeg
// using -c copy -movflags +faststart into a cache directory and returns the cached path.
func EnsureFastStart(absPath string, noRemux bool) (effectivePath string, wasRemuxed bool, err error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", false, fmt.Errorf("open file: %w", err)
	}

	valErr := ValidateMP4(file)
	_ = file.Close()

	if valErr == nil {
		// Valid FastStart MP4
		return absPath, false, nil
	}

	if !errors.Is(valErr, ErrLacksFastStart) {
		// Fatal/unrecoverable error (e.g. corrupted file, missing ftyp, not an MP4)
		return "", false, valErr
	}

	// File lacks FastStart
	if noRemux {
		return "", false, ErrLacksFastStart
	}

	// 1. Verify ffmpeg exists on PATH
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", false, fmt.Errorf("ffmpeg not found on PATH.\nThis file lacks FastStart (mdat before moov) and cannot seek properly in browsers.\nTo fix this manually, run:\n  ffmpeg -i %q -c copy -movflags +faststart <output.mp4>", absPath)
	}

	// 2. Determine cache path keyed on path + size + mtime
	stat, err := os.Stat(absPath)
	if err != nil {
		return "", false, fmt.Errorf("stat file: %w", err)
	}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s:%d:%d", absPath, stat.Size(), stat.ModTime().UnixNano())
	keyHash := hex.EncodeToString(h.Sum(nil))[:16]

	cacheBaseDir, err := os.UserCacheDir()
	if err != nil {
		cacheBaseDir = os.TempDir()
	}
	cacheDir := filepath.Join(cacheBaseDir, "streamla")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", false, fmt.Errorf("create cache directory: %w", err)
	}

	baseName := filepath.Base(absPath)
	ext := filepath.Ext(baseName)
	cleanName := strings.TrimSuffix(baseName, ext)
	cacheFileName := fmt.Sprintf("%s_%s.mp4", cleanName, keyHash)
	cachePath := filepath.Join(cacheDir, cacheFileName)

	// 3. Check if cached copy already exists and is valid
	if cStat, err := os.Stat(cachePath); err == nil && cStat.Size() > 0 {
		cFile, err := os.Open(cachePath)
		if err == nil {
			if cErr := ValidateMP4(cFile); cErr == nil {
				_ = cFile.Close()
				log.Printf("Using cached FastStart copy: %s", cachePath)
				return cachePath, false, nil
			}
			_ = cFile.Close()
		}
	}

	// 4. Remux into temp file then rename
	tempPath := fmt.Sprintf("%s.tmp.%d.mp4", cachePath, time.Now().UnixNano())
	defer func() {
		if _, err := os.Stat(tempPath); err == nil {
			_ = os.Remove(tempPath)
		}
	}()

	log.Printf("MP4 lacks FastStart ('mdat' before 'moov'). Remuxing with +faststart into cache (%s)... this takes a few seconds", cachePath)

	args := []string{
		"-v", "error",
		"-i", absPath,
		"-c", "copy",
		"-f", "mp4",
		"-movflags", "+faststart",
		"-y",
		tempPath,
	}

	cmd := exec.CommandContext(context.Background(), ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("ffmpeg remux failed: %w\nstderr: %s", err, stderr.String())
	}

	// Verify temp file was produced
	tempStat, err := os.Stat(tempPath)
	if err != nil || tempStat.Size() == 0 {
		return "", false, fmt.Errorf("remux produced empty or missing file: %v", err)
	}

	// Atomically rename to final cache location
	if err := os.Rename(tempPath, cachePath); err != nil {
		return "", false, fmt.Errorf("rename cached remux file: %w", err)
	}

	log.Printf("FastStart remux complete. Cached at: %s", cachePath)
	return cachePath, true, nil
}
