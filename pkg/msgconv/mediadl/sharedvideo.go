package mediadl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
)

const (
	SharedVideoMaxSize       int64 = 64 * 1024 * 1024
	SharedVideoMaxSourceSize int64 = 256 * 1024 * 1024
)

var (
	ErrSharedVideoCompressionUnavailable = errors.New("video compression unavailable")
	ErrSharedVideoCompressionBusy        = errors.New("video compression busy")
	ErrSharedVideoCompressionFailed      = errors.New("video compression failed")
	sharedVideoGate                      = make(chan struct{}, 1)
)

// SelectSharedVideo chooses the smallest source that can fill a 720x1280 box
// (in either orientation), or the best available smaller source. It avoids
// downloading the highest resolution when Instagram already offers 720p.
func SelectSharedVideo(versions []responses.VideoVersions) (best responses.VideoVersions, found bool) {
	bestScale := 0.0
	for _, v := range versions {
		if v.URL == "" {
			continue
		}
		scale := 0.0
		if v.Width > 0 && v.Height > 0 {
			scale = math.Max(float64(min(v.Width, v.Height))/720, float64(max(v.Width, v.Height))/1280)
		}
		if !found || (scale > 0 && bestScale == 0) ||
			(bestScale < 1 && scale > bestScale) ||
			(bestScale >= 1 && scale >= 1 && scale < bestScale) {
			best, bestScale, found = v, scale, true
		}
	}
	return
}

func SharedVideoCompressionAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// CompressSharedVideo always transcodes shared Instagram videos. It never
// returns the original on failure. Sources and output stay on private disk,
// one conversion runs at a time, and closing the returned reader removes both
// temporary files. CHATLOOM_MEDIA_TMPDIR (or TMPDIR) must point to disk storage.
func CompressSharedVideo(ctx context.Context, source io.Reader, sourceSize, maxSize int64) (int64, io.ReadCloser, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return 0, nil, ErrSharedVideoCompressionUnavailable
	}
	if maxSize <= 0 {
		maxSize = SharedVideoMaxSourceSize
	}
	maxSourceSize := min(maxSize, SharedVideoMaxSourceSize)
	maxOutputSize := min(maxSize, SharedVideoMaxSize)
	if sourceSize == 0 || sourceSize > maxSourceSize {
		return 0, nil, ErrTooLargeFile
	}
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case sharedVideoGate <- struct{}{}:
		defer func() { <-sharedVideoGate }()
	default:
		return 0, nil, ErrSharedVideoCompressionBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	stopClosingSource := context.AfterFunc(ctx, func() {
		if closer, ok := source.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	defer stopClosingSource()
	dir, err := os.MkdirTemp(os.Getenv("CHATLOOM_MEDIA_TMPDIR"), "chatloom-shared-video-")
	if err != nil {
		return 0, nil, fmt.Errorf("%w: temporary storage", ErrSharedVideoCompressionFailed)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	inputPath := filepath.Join(dir, "input.mp4")
	input, err := os.OpenFile(inputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: source file", ErrSharedVideoCompressionFailed)
	}
	copyLimit := maxSourceSize + 1
	if sourceSize > 0 {
		copyLimit = sourceSize + 1
	}
	n, copyErr := io.Copy(input, io.LimitReader(source, copyLimit))
	closeErr := input.Close()
	if n > maxSourceSize {
		return 0, nil, ErrTooLargeFile
	}
	if copyErr != nil || closeErr != nil || n == 0 || (sourceSize > 0 && n != sourceSize) {
		return 0, nil, fmt.Errorf("%w: incomplete source", ErrSharedVideoCompressionFailed)
	}
	outputPath := filepath.Join(dir, "compressed.mp4")
	cmd := exec.CommandContext(ctx, ffmpegPath, sharedVideoArgs(inputPath, outputPath, maxOutputSize)...)
	// Do not expose source metadata or FFmpeg diagnostics in logs or responses.
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return 0, nil, fmt.Errorf("%w: %w", ErrSharedVideoCompressionFailed, ctx.Err())
		}
		return 0, nil, ErrSharedVideoCompressionFailed
	}
	output, err := os.Open(outputPath)
	if err != nil {
		return 0, nil, ErrSharedVideoCompressionFailed
	}
	stat, err := output.Stat()
	// FFmpeg's -fs may stop encoding early: reject that output instead of serving
	// a truncated reel. It can overshoot the threshold by a packet or two.
	if err != nil || stat.Size() < 1 || stat.Size() >= maxOutputSize {
		_ = output.Close()
		return 0, nil, ErrTooLargeFile
	}
	_ = os.Remove(inputPath)
	keep = true
	return stat.Size(), &sharedVideoReader{File: output, dir: dir}, nil
}

func sharedVideoArgs(inputPath, outputPath string, maxSize int64) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-threads", "2", "-filter_threads", "1", "-protocol_whitelist", "file,pipe", "-f", "mov", "-i", inputPath,
		"-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1",
		"-vf", "scale=w='min(iw,if(gte(iw,ih),1280,720))':h='min(ih,if(gte(iw,ih),720,1280))':force_original_aspect_ratio=decrease:force_divisible_by=2,setsar=1",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-maxrate", "1800k", "-bufsize", "3600k",
		"-pix_fmt", "yuv420p", "-fpsmax", "30", "-threads", "2",
		"-c:a", "aac", "-b:a", "96k", "-ac", "2", "-movflags", "+faststart",
		"-fs", strconv.FormatInt(maxSize, 10), outputPath,
	}
}

type sharedVideoReader struct {
	*os.File
	dir string
}

func (r *sharedVideoReader) Close() error {
	err := r.File.Close()
	_ = os.RemoveAll(r.dir)
	return err
}
