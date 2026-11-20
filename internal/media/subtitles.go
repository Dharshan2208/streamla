package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
)

// ExtractSubtitleVTT extracts an embedded subtitle stream and converts it on the fly to WebVTT.
func ExtractSubtitleVTT(ctx context.Context, filePath string, streamIndex int, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-i", filePath,
		"-map", "0:"+strconv.Itoa(streamIndex),
		"-f", "webvtt",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("subtitle pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg subtitle extraction: %w", err)
	}

	_, copyErr := io.Copy(w, stdout)
	_ = cmd.Wait()

	return copyErr
}

// ConvertExternalSRT converts an external .srt file to WebVTT format and streams it to the client.
func ConvertExternalSRT(ctx context.Context, srtPath string, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-i", srtPath,
		"-f", "webvtt",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("srt pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg srt conversion: %w", err)
	}

	_, copyErr := io.Copy(w, stdout)
	_ = cmd.Wait()

	return copyErr
}
