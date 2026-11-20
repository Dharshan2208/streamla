package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
)

// StreamRemux remuxes video and selected audio into a fragmented MP4 stream.
// It uses stream-copy (-c:v copy) for zero host CPU utilization on video.
func StreamRemux(ctx context.Context, filePath string, audioTrackIndex int, isAudioAAC bool, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Transfer-Encoding", "chunked")

	audioCodec := "aac"
	if isAudioAAC {
		audioCodec = "copy"
	}

	args := []string{
		"-v", "error",
		"-i", filePath,
		"-map", "0:v:0",
	}

	if audioTrackIndex >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(audioTrackIndex))
	} else {
		args = append(args, "-map", "0:a:0?")
	}

	args = append(args,
		"-c:v", "copy",
		"-c:a", audioCodec,
	)

	if audioCodec == "aac" {
		args = append(args, "-b:a", "192k")
	}

	args = append(args,
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg remux: %w", err)
	}

	// Stream stdout to HTTP response
	buf := make([]byte, 32*1024)
	flusher, canFlush := w.(http.Flusher)

	_, copyErr := io.CopyBuffer(w, stdout, buf)
	if canFlush {
		flusher.Flush()
	}

	// Clean up process
	_ = cmd.Wait()

	return copyErr
}
