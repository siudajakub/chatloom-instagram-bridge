package mediadl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
)

func TestSelectSharedVideo(t *testing.T) {
	v := func(url string, w, h int) responses.VideoVersions {
		return responses.VideoVersions{URL: url, Width: w, Height: h}
	}
	for _, tc := range []struct {
		name     string
		versions []responses.VideoVersions
		want     string
	}{
		{"portrait 720p", []responses.VideoVersions{v("4k", 2160, 3840), v("720p", 720, 1280), v("1080p", 1080, 1920)}, "720p"},
		{"landscape 720p", []responses.VideoVersions{v("1080p", 1920, 1080), v("720p", 1280, 720)}, "720p"},
		{"smallest adequate", []responses.VideoVersions{v("4k", 2160, 3840), v("1080p", 1080, 1920), v("tiny", 240, 426)}, "1080p"},
		{"best smaller", []responses.VideoVersions{v("tiny", 240, 426), v("480p", 480, 854)}, "480p"},
		{"square", []responses.VideoVersions{v("1080p", 1080, 1080), v("720p", 720, 720)}, "720p"},
		{"skip missing URL", []responses.VideoVersions{v("", 720, 1280), v("480p", 480, 854)}, "480p"},
		{"prefer known dimensions", []responses.VideoVersions{v("unknown", 0, 0), v("720p", 720, 1280)}, "720p"},
		{"unknown dimensions fallback", []responses.VideoVersions{v("unknown", 0, 0)}, "unknown"},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, found := SelectSharedVideo(tc.versions)
			if got.URL != tc.want || found != (tc.want != "") {
				t.Fatalf("got %q, found %v; want %q", got.URL, found, tc.want)
			}
		})
	}
}

func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CHATLOOM_MEDIA_TMPDIR", dir)
	return dir
}

func TestSharedVideoUnavailableDoesNotReturnOriginal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, reader, err := CompressSharedVideo(context.Background(), strings.NewReader("original"), 8, 1024)
	if !errors.Is(err, ErrSharedVideoCompressionUnavailable) || reader != nil {
		t.Fatalf("got reader %v, error %v", reader, err)
	}
}

func TestSharedVideoRejectsLargeOrIncompleteSourceAndCleansUp(t *testing.T) {
	dir := fakeFFmpeg(t)
	for _, tc := range []struct {
		name string
		data string
		size int64
		max  int64
		want error
	}{
		{"declared too large", "source", 6, 5, ErrTooLargeFile},
		{"source may exceed output ceiling", "short", SharedVideoMaxSize + 1, SharedVideoMaxSourceSize, ErrSharedVideoCompressionFailed},
		{"configured source ceiling", "source", SharedVideoMaxSize + 1, SharedVideoMaxSize, ErrTooLargeFile},
		{"hard source ceiling", "source", SharedVideoMaxSourceSize + 1, SharedVideoMaxSourceSize * 2, ErrTooLargeFile},
		{"unknown too large", "source", -1, 5, ErrTooLargeFile},
		{"incomplete", "short", 10, 1024, ErrSharedVideoCompressionFailed},
		{"longer than declared", "longer", 3, 1024, ErrSharedVideoCompressionFailed},
		{"encoder failure", "invalid video", 13, 1024, ErrSharedVideoCompressionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, reader, err := CompressSharedVideo(context.Background(), strings.NewReader(tc.data), tc.size, tc.max)
			if !errors.Is(err, tc.want) || reader != nil {
				t.Fatalf("got reader %v, error %v; want %v", reader, err, tc.want)
			}
			files, _ := filepath.Glob(filepath.Join(dir, "chatloom-shared-video-*"))
			if len(files) != 0 {
				t.Fatal("temporary media left after failure")
			}
		})
	}
}

func TestSharedVideoLimitsConcurrency(t *testing.T) {
	fakeFFmpeg(t)
	sharedVideoGate <- struct{}{}
	defer func() { <-sharedVideoGate }()
	_, reader, err := CompressSharedVideo(context.Background(), strings.NewReader("source"), 6, 1024)
	if !errors.Is(err, ErrSharedVideoCompressionBusy) || reader != nil {
		t.Fatalf("got reader %v, error %v", reader, err)
	}
}

func TestSharedVideoCancellation(t *testing.T) {
	fakeFFmpeg(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, reader, err := CompressSharedVideo(ctx, strings.NewReader("source"), 6, 1024)
	if !errors.Is(err, context.Canceled) || reader != nil {
		t.Fatalf("got reader %v, error %v", reader, err)
	}
}

// Run explicitly with CHATLOOM_TEST_FFMPEG=1. All media is synthetic and the
// temporary directory follows TMPDIR, so no Instagram account is contacted.
func TestSharedVideoFFmpeg(t *testing.T) {
	if os.Getenv("CHATLOOM_TEST_FFMPEG") != "1" {
		t.Skip("set CHATLOOM_TEST_FFMPEG=1 for synthetic encoder verification")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("CHATLOOM_MEDIA_TMPDIR", dir)
	for _, tc := range []struct {
		name, inputSize, fps string
		audio                bool
		width, height        int
		wantFPS              string
	}{
		{"portrait high fps", "1080x1920", "60", true, 720, 1280, "30/1"},
		{"landscape", "1920x1080", "30", true, 1280, 720, "30/1"},
		{"small silent low fps", "320x240", "15", false, 320, 240, "15/1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourcePath := filepath.Join(dir, "synthetic.mp4")
			args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=" + tc.inputSize + ":rate=" + tc.fps}
			if tc.audio {
				args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000")
			}
			args = append(args, "-t", "2", "-c:v", "libx264", "-threads", "2", "-preset", "ultrafast", "-crf", "16", "-pix_fmt", "yuv420p", "-c:a", "aac", sourcePath)
			if data, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("synthetic input: %v: %s", err, data)
			}
			source, err := os.Open(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			inputStat, _ := source.Stat()
			size, reader, err := CompressSharedVideo(context.Background(), source, inputStat.Size(), SharedVideoMaxSize)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			if err != nil || int64(len(data)) != size {
				t.Fatalf("read output: %v, size %d", err, size)
			}
			if size >= inputStat.Size() {
				t.Fatalf("output %d bytes is no smaller than input %d", size, inputStat.Size())
			}
			if moov, mdat := bytes.Index(data, []byte("moov")), bytes.Index(data, []byte("mdat")); moov < 0 || mdat < 0 || moov > mdat {
				t.Fatal("MP4 is missing faststart")
			}
			outputPath := reader.(*sharedVideoReader).Name()
			probe, err := exec.Command("ffprobe", "-v", "error", "-show_streams", "-of", "json", outputPath).Output()
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Streams []struct {
					CodecName  string `json:"codec_name"`
					CodecType  string `json:"codec_type"`
					Width      int    `json:"width"`
					Height     int    `json:"height"`
					FrameRate  string `json:"r_frame_rate"`
					SampleRate string `json:"sample_rate"`
				} `json:"streams"`
			}
			if err = json.Unmarshal(probe, &result); err != nil {
				t.Fatal(err)
			}
			video, audio := false, false
			for _, stream := range result.Streams {
				if stream.CodecType == "video" {
					video = true
					if stream.CodecName != "h264" || stream.Width != tc.width || stream.Height != tc.height || stream.FrameRate != tc.wantFPS {
						t.Fatalf("unexpected video stream: %+v", stream)
					}
				} else if stream.CodecType == "audio" {
					audio = true
					if stream.CodecName != "aac" {
						t.Fatalf("unexpected audio codec %q", stream.CodecName)
					}
				}
			}
			if !video || audio != tc.audio {
				t.Fatalf("video %v audio %v; want video and audio %v", video, audio, tc.audio)
			}
			if err = reader.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(outputPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("temporary compressed video left after close")
			}
			t.Logf("synthetic %s: %d -> %d bytes (%.1f%% smaller)", tc.inputSize, inputStat.Size(), size, 100*(1-float64(size)/float64(inputStat.Size())))
		})
	}
}
