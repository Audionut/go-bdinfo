package bdinfo

import (
	"context"
	"strings"
	"testing"

	"github.com/autobrr/go-bdinfo/internal/bdrom"
	"github.com/autobrr/go-bdinfo/internal/stream"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if !opts.GenerateStreamDiagnostics {
		t.Fatalf("expected GenerateStreamDiagnostics default true")
	}
	if !opts.EnableSSIF {
		t.Fatalf("expected EnableSSIF default true")
	}
	if opts.FilterShortPlaylistsVal != 20 {
		t.Fatalf("expected FilterShortPlaylistsVal=20, got %d", opts.FilterShortPlaylistsVal)
	}
}

func TestRenderReportWithSyntheticAnalysis(t *testing.T) {
	opts := DefaultOptions()

	video := &stream.VideoStream{
		Stream: stream.Stream{
			PID:        0x1011,
			StreamType: stream.StreamTypeAVCVideo,
			BitRate:    8_000_000,
		},
		Height:        1080,
		FrameRateEnum: 24000,
		FrameRateDen:  1001,
		AspectRatio:   stream.Aspect169,
	}

	playlist := &bdrom.PlaylistFile{
		Name:         "00001.MPLS",
		Settings:     opts.toSettings("."),
		Streams:      map[uint16]stream.Info{0x1011: video},
		VideoStreams: []*stream.VideoStream{video},
		SortedStreams: []stream.Info{
			video,
		},
	}

	rom := &bdrom.BDROM{
		VolumeLabel: "TEST_DISC",
		DiscTitle:   "TEST_DISC",
		Size:        123456,
	}

	analysis := &Analysis{
		Path: "fake-disc",
		Disc: DiscInfo{
			VolumeLabel: rom.VolumeLabel,
			DiscTitle:   rom.DiscTitle,
			SizeBytes:   rom.Size,
		},
		raw: &analysisRaw{
			rom:       rom,
			playlists: []*bdrom.PlaylistFile{playlist},
			scan:      bdrom.ScanResult{},
			settings:  opts,
		},
	}

	reportText, err := RenderReport(context.Background(), analysis, nil)
	if err != nil {
		t.Fatalf("RenderReport() error = %v", err)
	}
	if !strings.Contains(reportText, "PLAYLIST: 00001.MPLS") {
		t.Fatalf("expected report to include playlist header")
	}
}

func TestRenderReportNilAnalysis(t *testing.T) {
	_, err := RenderReport(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil analysis")
	}
}
