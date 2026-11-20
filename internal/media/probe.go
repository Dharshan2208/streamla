package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StreamInfo describes an individual audio, video, or subtitle stream.
type StreamInfo struct {
	Index    int    `json:"index"`
	Type     string `json:"type"` // "video", "audio", "subtitle"
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Channels int    `json:"channels,omitempty"`
}

// MediaProbeResult stores the complete probed media attributes.
type MediaProbeResult struct {
	FormatName       string       `json:"formatName"`
	Duration         float64      `json:"duration"`
	Video            *StreamInfo  `json:"video,omitempty"`
	Audio            []StreamInfo `json:"audio"`
	Subtitles        []StreamInfo `json:"subtitles"`
	ExternalSubtitle string       `json:"externalSubtitle,omitempty"` // path to adjacent .srt if found
	NeedsRemux       bool         `json:"needsRemux"`
	IsHEVC           bool         `json:"isHEVC"`
}

// HEVCTranscodeCommand returns the exact ffmpeg command string needed to transcode an HEVC file to H.264 MP4.
func HEVCTranscodeCommand(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	cleanName := strings.TrimSuffix(base, ext)
	outputPath := filepath.Join(dir, cleanName+"_h264.mp4")
	return fmt.Sprintf("ffmpeg -i %q -c:v libx264 -preset fast -crf 22 -c:a copy %q", inputPath, outputPath)
}

// CheckCodecCompatibility returns an error if the media contains a video codec incompatible with standard web browsers (such as HEVC/H.265).
func CheckCodecCompatibility(probe *MediaProbeResult, filePath string) error {
	if probe != nil && probe.IsHEVC {
		codec := "hevc"
		if probe.Video != nil && probe.Video.Codec != "" {
			codec = probe.Video.Codec
		}
		return fmt.Errorf("unsupported video codec %q (H.265).\nWeb browsers (Firefox, Chrome on Linux, and default Windows Edge) cannot decode HEVC/H.265 natively in MP4.\nTo transcode this file to browser-compatible H.264, run:\n  %s",
			codec, HEVCTranscodeCommand(filePath))
	}
	return nil
}

type ffprobeOutput struct {
	Streams []struct {
		Index     int    `json:"index"`
		CodecName string `json:"codec_name"`
		CodecType string `json:"codec_type"`
		Channels  int    `json:"channels"`
		Tags      struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

// ProbeMedia runs ffprobe to inspect codecs, audio tracks, and subtitles.
func ProbeMedia(filePath string) (*MediaProbeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed for %q: %w", filePath, err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse ffprobe json: %w", err)
	}

	result := &MediaProbeResult{
		FormatName: parsed.Format.FormatName,
		Audio:      []StreamInfo{},
		Subtitles:  []StreamInfo{},
	}

	if dur, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
		result.Duration = dur
	}

	isMP4 := strings.Contains(parsed.Format.FormatName, "mp4") || strings.Contains(parsed.Format.FormatName, "mov")

	for _, s := range parsed.Streams {
		info := StreamInfo{
			Index:    s.Index,
			Type:     s.CodecType,
			Codec:    s.CodecName,
			Language: s.Tags.Language,
			Title:    s.Tags.Title,
			Channels: s.Channels,
		}
		if info.Language == "" {
			info.Language = "und"
		}

		switch s.CodecType {
		case "video":
			if result.Video == nil {
				result.Video = &info
			}
			codecLower := strings.ToLower(s.CodecName)
			if codecLower == "hevc" || codecLower == "h265" || codecLower == "hev1" || codecLower == "hvc1" {
				result.IsHEVC = true
			}
		case "audio":
			result.Audio = append(result.Audio, info)
		case "subtitle":
			result.Subtitles = append(result.Subtitles, info)
		}
	}

	// Check if container or audio requires remuxing
	// Native browser requires MP4 container, H.264 video, and AAC audio
	if !isMP4 {
		result.NeedsRemux = true
	} else if len(result.Audio) > 0 && result.Audio[0].Codec != "aac" && result.Audio[0].Codec != "mp3" {
		result.NeedsRemux = true
	}

	// Check for adjacent external subtitle (.srt or .vtt)
	dir := filepath.Dir(filePath)
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	candidates := []string{
		filepath.Join(dir, base+".srt"),
		filepath.Join(dir, base+".vtt"),
		filepath.Join(dir, base+".en.srt"),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			result.ExternalSubtitle = c
			result.Subtitles = append(result.Subtitles, StreamInfo{
				Index:    -1, // -1 denotes external
				Type:     "subtitle",
				Codec:    "srt",
				Language: "external",
				Title:    filepath.Base(c),
			})
			break
		}
	}

	return result, nil
}
